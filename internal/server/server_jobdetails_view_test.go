package server

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/izzyreal/ciwi/internal/protocol"
)

func TestJobDetailsViewUsesApplicationPresentationShape(t *testing.T) {
	server, state := newTestHTTPServerWithState(t)
	defer server.Close()
	job, err := state.db.CreateJobExecution(protocol.CreateJobExecutionRequest{
		Script: "go test ./...", Metadata: map[string]string{
			"project": "ciwi", "pipeline_id": "build", "pipeline_job_id": "unit-tests",
		},
		StepPlan: []protocol.JobStepPlanItem{{Index: 1, Total: 1, Name: "Run tests", Script: "go test ./..."}},
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
	if view.ID != job.ID || view.Title != "Job: unit-tests" || view.Context == "" || view.Status != "queued" {
		t.Fatalf("view = %+v", view)
	}
	if len(view.Timeline) < 3 || view.Timeline[2].Title != "Job step 3/3: Run tests" {
		t.Fatalf("timeline = %+v", view.Timeline)
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
	if output.NextEventID <= 0 || output.Terminal || len(output.Lines) != 1 || output.Lines[0].Text != "ok\n" {
		t.Fatalf("output = %+v", output)
	}
}
