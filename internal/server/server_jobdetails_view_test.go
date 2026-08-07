package server

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/izzyreal/ciwi/internal/domain"
	"github.com/izzyreal/ciwi/internal/protocol"
	"github.com/izzyreal/ciwi/internal/server/jobexecution"
)

func TestJobDetailsViewUsesApplicationPresentationShape(t *testing.T) {
	server, state := newTestHTTPServerWithState(t)
	defer server.Close()
	job, err := state.db.CreateJobExecution(protocol.CreateJobExecutionRequest{
		Script: "go test ./...", Metadata: map[string]string{
			"project": "ciwi", "project_id": "42", "pipeline_id": "build", "pipeline_job_id": "unit-tests",
		},
		RequiredCapabilities: map[string]string{"os": "windows", "requires.tool.wix": ">=6.0.0"},
		StepPlan:             []protocol.JobStepPlanItem{{Index: 1, Total: 1, Name: "Run tests", YAMLLiteral: "run: go test ./...", Script: "go test ./..."}},
	})
	if err != nil {
		t.Fatal(err)
	}
	response := mustJSONRequest(t, server.Client(), http.MethodGet, server.URL+"/api/v1/views/jobs/"+job.ID, nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d: %s", response.StatusCode, readBody(t, response))
	}
	defer response.Body.Close()
	var view jobDetailsViewResponse
	if err := json.NewDecoder(response.Body).Decode(&view); err != nil {
		t.Fatal(err)
	}
	if view.ID != job.ID || view.ProjectID != 42 || view.Title != "ciwi / build / unit-tests" || view.Context == "" || view.Status != "queued" {
		t.Fatalf("view = %+v", view)
	}
	if view.SchedulingDiagnosis.Summary != "No agents are registered" {
		t.Fatalf("scheduling diagnosis = %+v", view.SchedulingDiagnosis)
	}
	if len(view.Timeline) < 3 || view.Timeline[2].Title != "Job step 1/1: Run tests" {
		t.Fatalf("timeline = %+v", view.Timeline)
	}
	if len(view.OutputGroups) != len(view.Timeline) || view.OutputGroups[2].YAMLLiteral == "" || view.OutputGroups[2].ExpandedCommand != "go test ./..." {
		t.Fatalf("output groups = %+v", view.OutputGroups)
	}
	if view.Progress.State != domain.ProgressIndeterminate {
		t.Fatalf("job progress = %+v", view.Progress)
	}
	if len(view.JobProperties) == 0 || view.CacheStatisticsEmpty == "" {
		t.Fatalf("job detail rows = properties %+v cache empty %q", view.JobProperties, view.CacheStatisticsEmpty)
	}
	if view.HostToolRequirements.Summary == "" && view.HostToolRequirements.EmptyLabel == "" {
		t.Fatalf("host tool requirements = %+v", view.HostToolRequirements)
	}
	if view.ContainerToolRequirements.EmptyLabel == "" {
		t.Fatalf("container tool requirements = %+v", view.ContainerToolRequirements)
	}
	if view.Artifacts.EmptyLabel == "" || view.TestReport.EmptyLabel == "" || view.CoverageReport.EmptyLabel == "" {
		t.Fatalf("reports = artifacts %+v tests %+v coverage %+v", view.Artifacts, view.TestReport, view.CoverageReport)
	}
	if !view.RunContext.Available || len(view.RunContext.Pipelines) != 1 {
		t.Fatalf("run context = %+v", view.RunContext)
	}
	if err := state.db.AppendJobExecutionEvents(job.ID, []protocol.JobExecutionEvent{{
		Type: protocol.JobExecutionEventTypeStepOutput, Step: &protocol.JobStepPlanItem{Index: 1, Total: 1, Name: "Run tests"}, Output: "\x1b[32mok\x1b[0m\n",
	}}); err != nil {
		t.Fatal(err)
	}
	outputResponse := mustJSONRequest(t, server.Client(), http.MethodGet, server.URL+"/api/v1/views/jobs/"+job.ID+"/output?after_event_id=0", nil)
	if outputResponse.StatusCode != http.StatusOK {
		t.Fatalf("output status = %d: %s", outputResponse.StatusCode, readBody(t, outputResponse))
	}
	defer outputResponse.Body.Close()
	var output jobOutputViewResponse
	if err := json.NewDecoder(outputResponse.Body).Decode(&output); err != nil {
		t.Fatal(err)
	}
	if output.NextEventID <= 0 || output.Terminal || len(output.Events) != 1 || output.Events[0].Text != "ok\n" {
		t.Fatalf("output = %+v", output)
	}
}

func TestJobDetailsViewIncludesStoredReportsAndSyntheticReportArtifacts(t *testing.T) {
	server, state := newTestHTTPServerWithState(t)
	defer server.Close()
	job, err := state.db.CreateJobExecution(protocol.CreateJobExecutionRequest{
		Script: "go test ./...",
		Metadata: map[string]string{
			"project": "ciwi", "pipeline_id": "build", "pipeline_job_id": "unit-tests",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	report := protocol.JobExecutionTestReport{
		Total: 2, Passed: 1, Failed: 1,
		Coverage: &protocol.CoverageReport{
			Format: "go-coverprofile", TotalStatements: 10, CoveredStatements: 6, Percent: 60,
		},
	}
	if err := state.db.SaveJobExecutionTestReport(job.ID, report); err != nil {
		t.Fatal(err)
	}
	if err := jobexecution.PersistTestReportArtifact(state.artifactsDir, job.ID, report); err != nil {
		t.Fatal(err)
	}
	if err := jobexecution.PersistCoverageReportArtifact(state.artifactsDir, job.ID, report); err != nil {
		t.Fatal(err)
	}

	response := mustJSONRequest(t, server.Client(), http.MethodGet, server.URL+"/api/v1/views/jobs/"+job.ID, nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d: %s", response.StatusCode, readBody(t, response))
	}
	defer response.Body.Close()
	var view jobDetailsViewResponse
	if err := json.NewDecoder(response.Body).Decode(&view); err != nil {
		t.Fatal(err)
	}
	if len(view.Artifacts.Rows) != 2 || view.Artifacts.Rows[0].Label != "coverage-report.json" || view.Artifacts.Rows[1].Label != "test-report.json" {
		t.Fatalf("artifacts = %+v", view.Artifacts)
	}
	if view.TestReport.Summary != "2 total · 1 passed · 1 failed · 0 skipped" {
		t.Fatalf("test report = %+v", view.TestReport)
	}
	if view.CoverageReport.Summary != "60.00% overall · 6/10 statements · 0 file(s) · go-coverprofile" {
		t.Fatalf("coverage report = %+v", view.CoverageReport)
	}
}
