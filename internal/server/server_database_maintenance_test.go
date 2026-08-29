package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/izzyreal/ciwi/internal/application"
	"github.com/izzyreal/ciwi/internal/protocol"
)

func TestVacuumDatabaseAPIReportsResultAndReplaysReceipt(t *testing.T) {
	ts, _ := newTestHTTPServerWithState(t)
	request, err := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/server/database/vacuum", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Idempotency-Key", "vacuum-api-1")
	response, err := ts.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", response.StatusCode)
	}
	var first application.DatabaseVacuumResult
	if err := json.NewDecoder(response.Body).Decode(&first); err != nil {
		t.Fatal(err)
	}
	if first.BeforeBytes <= 0 || first.AfterBytes <= 0 || !strings.HasPrefix(first.Message, "Database vacuumed:") {
		t.Fatalf("result = %+v", first)
	}

	replay, err := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/server/database/vacuum", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	replay.Header.Set("Idempotency-Key", "vacuum-api-1")
	replayResponse, err := ts.Client().Do(replay)
	if err != nil {
		t.Fatal(err)
	}
	defer replayResponse.Body.Close()
	var second application.DatabaseVacuumResult
	if err := json.NewDecoder(replayResponse.Body).Decode(&second); err != nil {
		t.Fatal(err)
	}
	if second != first {
		t.Fatalf("replayed result = %+v, want %+v", second, first)
	}
}

func TestVacuumDatabaseAPIRejectsActiveExecution(t *testing.T) {
	ts, state := newTestHTTPServerWithState(t)
	if _, err := state.db.CreateJobExecution(protocol.CreateJobExecutionRequest{Script: "echo queued"}); err != nil {
		t.Fatal(err)
	}
	response := mustJSONRequest(t, ts.Client(), http.MethodPost, ts.URL+"/api/v1/server/database/vacuum", map[string]any{})
	defer response.Body.Close()
	if response.StatusCode != http.StatusPreconditionFailed {
		t.Fatalf("status = %d", response.StatusCode)
	}
}
