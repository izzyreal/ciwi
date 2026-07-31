package server

import (
	"net/http"
	"testing"
	"time"

	"github.com/izzyreal/ciwi/internal/protocol"
)

func TestJobExecutionGraphContextAPI(t *testing.T) {
	ts, state := newTestHTTPServerWithState(t)
	defer ts.Close()
	created, err := state.db.CreateJobExecution(protocol.CreateJobExecutionRequest{
		Script: "echo ok", RequiredCapabilities: map[string]string{}, TimeoutSeconds: 30,
		Metadata: map[string]string{"project": "demo", "pipeline_id": "build", "pipeline_run_id": "run-1", "pipeline_job_id": "compile"},
	})
	if err != nil {
		t.Fatalf("create graph API job: %v", err)
	}
	resp := mustJSONRequest(t, ts.Client(), http.MethodGet, ts.URL+"/api/v1/jobs/"+created.ID+"/graph-context", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("graph context status=%d body=%s", resp.StatusCode, readBody(t, resp))
	}
	var context protocol.JobExecutionGraphContext
	decodeJSONBody(t, resp, &context)
	if context.Scope != "pipeline" || context.CurrentExecutionID != created.ID || len(context.Pipelines) != 1 {
		t.Fatalf("unexpected graph API response: %+v", context)
	}
}

func TestBuildJobExecutionGraphContextBuildsChainDAGAndLatestAttempts(t *testing.T) {
	base := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	job := func(id, pipeline, runID, logicalID, status string, offset int, extra map[string]string) protocol.JobExecution {
		metadata := map[string]string{
			"project": "demo", "project_id": "7", "pipeline_id": pipeline,
			"pipeline_run_id": runID, "pipeline_job_id": logicalID,
			"chain_run_id": "chain-run", "pipeline_chain_name": "Build and release",
		}
		for key, value := range extra {
			metadata[key] = value
		}
		return protocol.JobExecution{ID: id, Status: status, Metadata: metadata, CreatedUTC: base.Add(time.Duration(offset) * time.Second)}
	}
	originalLinux := job("linux-1", "build", "build-run", "compile", "failed", 1, map[string]string{"pipeline_chain_index": "0", "matrix_name": "linux"})
	runLinux := job("linux-2", "build", "build-run", "compile", "succeeded", 2, map[string]string{
		"pipeline_chain_index": "0", "matrix_name": "linux",
		protocol.JobMetadataAttemptRootJobID: "linux-1", protocol.JobMetadataRerunOfJobID: "linux-1",
	})
	windows := job("windows-1", "build", "build-run", "compile", "succeeded", 3, map[string]string{"pipeline_chain_index": "0", "matrix_name": "windows"})
	publish := job("publish-1", "release", "release-run", "publish", "queued", 4, map[string]string{
		"pipeline_chain_index": "1", "chain_depends_on_pipelines": "build", "chain_blocked": "1", "needs_job_ids": "prepare",
	})

	context := buildJobExecutionGraphContext(originalLinux, []protocol.JobExecution{publish, windows, runLinux, originalLinux}, map[string]int64{"build": 11, "release": 12})
	if context.Scope != "chain" || context.CurrentPipelineID != "build" || context.CurrentPipelineChain != "Build and release" {
		t.Fatalf("unexpected context header: %+v", context)
	}
	if len(context.Pipelines) != 2 || context.Pipelines[0].PipelineID != "build" || context.Pipelines[1].PipelineID != "release" {
		t.Fatalf("unexpected pipeline order: %+v", context.Pipelines)
	}
	if context.Pipelines[0].PipelineDBID != 11 || context.Pipelines[1].PipelineDBID != 12 {
		t.Fatalf("missing current definition IDs: %+v", context.Pipelines)
	}
	if len(context.Pipelines[1].DependsOn) != 1 || context.Pipelines[1].DependsOn[0] != "build" || context.Pipelines[1].Status != "waiting" {
		t.Fatalf("unexpected release graph node: %+v", context.Pipelines[1])
	}
	compile := context.Pipelines[0].Jobs[0]
	if compile.PipelineJobID != "compile" || compile.Status != "succeeded" || len(compile.Executions) != 3 {
		t.Fatalf("unexpected compile aggregation: %+v", compile)
	}
	latest := map[string]bool{}
	for _, execution := range compile.Executions {
		latest[execution.ID] = execution.LatestAttempt
	}
	if latest["linux-1"] || !latest["linux-2"] || !latest["windows-1"] {
		t.Fatalf("unexpected latest attempt flags: %+v", latest)
	}
}

func TestBuildJobExecutionGraphContextFallsBackToIsolatedJob(t *testing.T) {
	job := protocol.JobExecution{
		ID: "adhoc-1", Status: "running", CreatedUTC: time.Now().UTC(),
		Metadata: map[string]string{"pipeline_job_id": "adhoc"},
	}
	context := buildJobExecutionGraphContext(job, []protocol.JobExecution{job}, nil)
	if context.Scope != "job" || len(context.Pipelines) != 1 || len(context.Pipelines[0].Jobs) != 1 {
		t.Fatalf("unexpected isolated context: %+v", context)
	}
	if context.Pipelines[0].Jobs[0].Status != "running" {
		t.Fatalf("unexpected isolated status: %+v", context.Pipelines[0].Jobs[0])
	}
}

func TestAggregateJobGraphStatusesPrioritizesActiveThenFailure(t *testing.T) {
	if got := aggregateJobGraphStatuses([]string{"failed", "running"}); got != "running" {
		t.Fatalf("active status should win, got %q", got)
	}
	if got := aggregateJobGraphStatuses([]string{"waiting", "failed"}); got != "failed" {
		t.Fatalf("failure should win over waiting, got %q", got)
	}
}
