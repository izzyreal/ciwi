package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/izzyreal/ciwi/internal/domain"
	"github.com/izzyreal/ciwi/internal/protocol"
)

func TestFrontPageViewIncludesExecutionCardSummaries(t *testing.T) {
	server, state := newTestHTTPServerWithState(t)
	defer server.Close()
	_, err := state.db.CreateJobExecution(protocol.CreateJobExecutionRequest{
		Script: "true",
		Metadata: map[string]string{
			"project": "ciwi", "pipeline_id": "build", "pipeline_run_id": "run-1",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	state.frontPageViewHandler(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/views/front-page", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
	}
	var response frontPageViewResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.QueuedExecutions) != 1 || response.QueuedExecutions[0].Summary.InProgress != 1 {
		t.Fatalf("queued executions = %+v", response.QueuedExecutions)
	}
}

func TestFrontPageProjectResponseKeepsEmptyContractFields(t *testing.T) {
	response := frontPageProjectsToResponse([]domain.Project{{
		ID: 1, Name: "ad-hoc", Pipelines: []domain.Pipeline{{ID: 2, PipelineID: "build"}},
	}})
	payload, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range [][]byte{
		[]byte(`"repo_url":""`), []byte(`"repo_ref":""`), []byte(`"config_file":""`),
		[]byte(`"depends_on":[]`), []byte(`"pipeline_chains":[]`),
	} {
		if !bytes.Contains(payload, field) {
			t.Errorf("response omits stable contract field %s: %s", field, payload)
		}
	}
}
