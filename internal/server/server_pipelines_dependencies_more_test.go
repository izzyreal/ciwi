package server

import (
	"strings"
	"testing"
	"time"

	"github.com/izzyreal/ciwi/internal/protocol"
	"github.com/izzyreal/ciwi/internal/store"
)

func TestPipelineDependencyHelpers(t *testing.T) {
	if dependencyRunIsSuccessful(nil) {
		t.Fatalf("expected empty statuses to be unsuccessful")
	}
	if !dependencyRunIsSuccessful([]string{protocol.JobExecutionStatusSucceeded}) {
		t.Fatalf("expected succeeded statuses to be successful")
	}
	if dependencyRunIsSuccessful([]string{protocol.JobExecutionStatusSucceeded, protocol.JobExecutionStatusFailed}) {
		t.Fatalf("expected mixed statuses to be unsuccessful")
	}

	if !dependencyRunVersionMatches(map[string]string{"pipeline_version_raw": "1.2.3"}, "1.2.3", "") {
		t.Fatalf("expected raw version match")
	}
	if dependencyRunVersionMatches(map[string]string{"pipeline_version": "v1.2.2"}, "", "v1.2.3") {
		t.Fatalf("expected tagged version mismatch")
	}
	if !dependencyRunVersionMatches(map[string]string{}, "", "") {
		t.Fatalf("expected empty version match")
	}

	needs := parseNeedsJobIDs("a, b, a, ,c")
	if len(needs) != 3 || needs[0] != "a" || needs[1] != "b" || needs[2] != "c" {
		t.Fatalf("unexpected parseNeedsJobIDs result: %v", needs)
	}
	if !needsContains(needs, "b") || needsContains(needs, "missing") {
		t.Fatalf("unexpected needsContains behavior")
	}
}

func TestVerifyDependencyRunInChain(t *testing.T) {
	now := time.Now().UTC()
	jobs := []protocol.JobExecution{
		{
			ID:         "job-1",
			Status:     protocol.JobExecutionStatusSucceeded,
			CreatedUTC: now,
			Metadata: map[string]string{
				"project":              "ciwi",
				"pipeline_id":          "build",
				"pipeline_run_id":      "run-1",
				"chain_run_id":         "chain-1",
				"pipeline_version":     "v1.2.3",
				"pipeline_version_raw": "1.2.3",
				"build_target":         "linux-amd64",
			},
			ArtifactGlobs: []string{"dist/**"},
		},
	}
	if _, found, err := verifyDependencyRunInChain(jobs, "", "ciwi", "build"); err == nil || found {
		t.Fatalf("expected missing chain run id to error")
	}
	ctx, found, err := verifyDependencyRunInChain(jobs, "chain-1", "ciwi", "build")
	if err != nil || !found {
		t.Fatalf("expected chain dependency to be found/satisfied: found=%v err=%v", found, err)
	}
	if ctx.VersionRaw != "1.2.3" || ctx.ArtifactJobIDs["linux-amd64"] != "job-1" {
		t.Fatalf("unexpected chain dependency context: %+v", ctx)
	}
	_, found, err = verifyDependencyRunInChain(jobs, "chain-missing", "ciwi", "build")
	if err != nil || found {
		t.Fatalf("expected missing chain run to return found=false without error, found=%v err=%v", found, err)
	}
}

func TestCheckPipelineDependenciesWithReporter(t *testing.T) {
	ts, s := newTestHTTPServerWithState(t)
	defer ts.Close()

	reporterMsgs := []string{}
	reporter := func(step, status, detail string) {
		reporterMsgs = append(reporterMsgs, step+":"+status+":"+detail)
	}

	p := store.PersistedPipeline{ProjectName: "ciwi", PipelineID: "release", DependsOn: []string{"build"}}
	_, err := s.checkPipelineDependenciesWithReporter(p, reporter)
	if err == nil {
		t.Fatalf("expected unsatisfied dependency when no prior runs")
	}
	if len(reporterMsgs) == 0 || !strings.Contains(strings.Join(reporterMsgs, "\n"), "dependencies:error") {
		t.Fatalf("expected reporter to include dependency error, got %v", reporterMsgs)
	}

	job, err := s.db.CreateJobExecution(protocol.CreateJobExecutionRequest{
		Script:         "echo build",
		TimeoutSeconds: 30,
		ArtifactGlobs:  []string{"dist/**"},
		Metadata: map[string]string{
			"project":              "ciwi",
			"pipeline_id":          "build",
			"pipeline_run_id":      "run-1",
			"pipeline_version":     "v1.2.3",
			"pipeline_version_raw": "1.2.3",
			"build_target":         "linux-amd64",
		},
	})
	if err != nil {
		t.Fatalf("create dependency job: %v", err)
	}
	if _, err := s.db.UpdateJobExecutionStatus(job.ID, protocol.JobExecutionStatusUpdateRequest{AgentID: "agent-1", Status: protocol.JobExecutionStatusSucceeded}); err != nil {
		t.Fatalf("mark dependency job succeeded: %v", err)
	}

	reporterMsgs = nil
	ctx, err := s.checkPipelineDependenciesWithReporter(p, reporter)
	if err != nil {
		t.Fatalf("expected dependency to be satisfied: %v", err)
	}
	if ctx.VersionRaw != "1.2.3" || ctx.ArtifactJobIDs["build:linux-amd64"] != job.ID {
		t.Fatalf("unexpected dependency context: %+v", ctx)
	}
	if len(reporterMsgs) == 0 || !strings.Contains(strings.Join(reporterMsgs, "\n"), "dependencies:ok") {
		t.Fatalf("expected reporter to include ok message, got %v", reporterMsgs)
	}
}

func TestCheckPipelineDependenciesWithReporterIgnoresCrossRepoResolvedRefInheritance(t *testing.T) {
	ts, s := newTestHTTPServerWithState(t)
	defer ts.Close()

	base := time.Now().UTC()
	createSucceeded := func(jobID, runID, pipelineID, repo, resolvedRef string) {
		t.Helper()
		job, err := s.db.CreateJobExecution(protocol.CreateJobExecutionRequest{
			Script:         "echo " + pipelineID,
			TimeoutSeconds: 30,
			Metadata: map[string]string{
				"project":                      "ciwi",
				"pipeline_id":                  pipelineID,
				"pipeline_run_id":              runID,
				"pipeline_version":             "v1.2.3",
				"pipeline_version_raw":         "1.2.3",
				"pipeline_source_repo":         repo,
				"pipeline_source_ref_resolved": resolvedRef,
				"created_hint":                 base.String(),
			},
		})
		if err != nil {
			t.Fatalf("create dependency job %s: %v", jobID, err)
		}
		if _, err := s.db.UpdateJobExecutionStatus(job.ID, protocol.JobExecutionStatusUpdateRequest{
			AgentID: "agent-1",
			Status:  protocol.JobExecutionStatusSucceeded,
		}); err != nil {
			t.Fatalf("mark dependency job %s succeeded: %v", jobID, err)
		}
	}

	createSucceeded("build-1", "run-build-1", "build-a", "https://github.com/acme/repo-a.git", "aaaaaaaa")
	createSucceeded("build-2", "run-build-2", "build-b", "https://github.com/acme/repo-b.git", "bbbbbbbb")

	p := store.PersistedPipeline{
		ProjectName: "ciwi",
		PipelineID:  "release",
		DependsOn:   []string{"build-a", "build-b"},
	}
	ctx, err := s.checkPipelineDependenciesWithReporter(p, nil)
	if err != nil {
		t.Fatalf("check dependencies: %v", err)
	}
	if ctx.SourceRefResolved != "" || ctx.SourceRepo != "" {
		t.Fatalf("expected no shared source ref inheritance across repo boundaries, got repo=%q ref=%q", ctx.SourceRepo, ctx.SourceRefResolved)
	}
}
