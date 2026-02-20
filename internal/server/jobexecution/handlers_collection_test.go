package jobexecution

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/izzyreal/ciwi/internal/protocol"
)

func TestHandleCollectionSummaryView(t *testing.T) {
	store := &stubStore{}
	store.listJobExecutionsFn = func() ([]protocol.JobExecution, error) {
		return []protocol.JobExecution{
			{ID: "q-1", Status: protocol.JobExecutionStatusQueued, Metadata: map[string]string{"pipeline_run_id": "run-a", "project_id": "p1", "pipeline_id": "build"}},
			{ID: "q-2", Status: protocol.JobExecutionStatusRunning, Metadata: map[string]string{"pipeline_run_id": "run-a", "project_id": "p1", "pipeline_id": "build"}},
			{ID: "h-1", Status: protocol.JobExecutionStatusFailed},
			{ID: "h-2", Status: protocol.JobExecutionStatusSucceeded},
		}, nil
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs?view=summary&max=3", nil)
	HandleCollection(rec, req, HandlerDeps{Store: store})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var got SummaryViewResponse
	mustDecodeCollectionJSON(t, rec, &got)
	if got.View != "summary" || got.Max != 3 {
		t.Fatalf("unexpected summary metadata: %+v", got)
	}
	if got.Total != 3 || got.QueuedCount != 2 || got.HistoryCount != 1 {
		t.Fatalf("unexpected summary counts: %+v", got)
	}
	if len(got.QueuedGroups) != 1 || got.QueuedGroups[0].RunID != "run-a|p1|build" {
		t.Fatalf("unexpected queued groups: %+v", got.QueuedGroups)
	}
	if len(got.HistoryGroups) != 1 || got.HistoryGroups[0].Key != "job:h-1" {
		t.Fatalf("unexpected history groups: %+v", got.HistoryGroups)
	}
}

func TestHandleCollectionPagedViewQueued(t *testing.T) {
	store := &stubStore{}
	store.listJobExecutionsFn = func() ([]protocol.JobExecution, error) {
		return []protocol.JobExecution{
			{ID: "q-1", Status: protocol.JobExecutionStatusQueued},
			{ID: "q-2", Status: protocol.JobExecutionStatusLeased},
			{ID: "q-3", Status: protocol.JobExecutionStatusRunning},
			{ID: "h-1", Status: protocol.JobExecutionStatusSucceeded},
		}, nil
	}
	deps := HandlerDeps{Store: store}
	deps.AttachTestSummaries = func(jobs []protocol.JobExecution) {
		for i := range jobs {
			jobs[i].TestSummary = &protocol.JobExecutionTestSummary{Total: 1, Passed: 1}
		}
	}
	deps.AttachUnmetRequirements = func(jobs []protocol.JobExecution) {
		for i := range jobs {
			jobs[i].UnmetRequirements = []string{"none"}
		}
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs?view=queued&offset=1&limit=1", nil)
	HandleCollection(rec, req, deps)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var got PagedViewResponse
	mustDecodeCollectionJSON(t, rec, &got)
	if got.View != "queued" || got.Total != 3 || got.Offset != 1 || got.Limit != 1 {
		t.Fatalf("unexpected page metadata: %+v", got)
	}
	if len(got.JobExecutions) != 1 || got.JobExecutions[0].ID != "q-2" {
		t.Fatalf("unexpected page jobs: %+v", got.JobExecutions)
	}
	if got.JobExecutions[0].TestSummary == nil || len(got.JobExecutions[0].UnmetRequirements) != 1 {
		t.Fatalf("expected attachers to enrich page jobs: %+v", got.JobExecutions[0])
	}
}

func TestHandleCollectionDefaultListAndMethodNotAllowed(t *testing.T) {
	store := &stubStore{}
	store.listJobExecutionsFn = func() ([]protocol.JobExecution, error) {
		return []protocol.JobExecution{{ID: "job-1", Status: protocol.JobExecutionStatusQueued}}, nil
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs", nil)
	HandleCollection(rec, req, HandlerDeps{Store: store})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var got ListViewResponse
	mustDecodeCollectionJSON(t, rec, &got)
	if len(got.JobExecutions) != 1 || got.JobExecutions[0].ID != "job-1" {
		t.Fatalf("unexpected list payload: %+v", got)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/jobs", nil)
	HandleCollection(rec, req, HandlerDeps{Store: store})
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestHandleCollectionStoreUnavailableAndStoreError(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs", nil)
	HandleCollection(rec, req, HandlerDeps{})
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 with nil store, got %d", rec.Code)
	}

	store := &stubStore{}
	store.listJobExecutionsFn = func() ([]protocol.JobExecution, error) {
		return nil, protocolError("boom")
	}
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/v1/jobs", nil)
	HandleCollection(rec, req, HandlerDeps{Store: store})
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 on store error, got %d", rec.Code)
	}
}

func TestHandleClearQueue(t *testing.T) {
	t.Run("nil store", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/jobs/clear-queue", nil)
		HandleClearQueue(rec, req, HandlerDeps{})
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d", rec.Code)
		}
	})

	t.Run("method not allowed", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs/clear-queue", nil)
		HandleClearQueue(rec, req, HandlerDeps{Store: &stubStore{}})
		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("expected 405, got %d", rec.Code)
		}
	})

	t.Run("store error", func(t *testing.T) {
		store := &stubStore{}
		store.clearQueuedJobExecutionsFn = func() (int64, error) { return 0, protocolError("nope") }
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/jobs/clear-queue", nil)
		HandleClearQueue(rec, req, HandlerDeps{Store: store})
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d", rec.Code)
		}
	})

	t.Run("success", func(t *testing.T) {
		store := &stubStore{}
		store.clearQueuedJobExecutionsFn = func() (int64, error) { return 7, nil }
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/jobs/clear-queue", nil)
		HandleClearQueue(rec, req, HandlerDeps{Store: store})
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var got ClearQueueViewResponse
		mustDecodeCollectionJSON(t, rec, &got)
		if got.Cleared != 7 {
			t.Fatalf("unexpected cleared count: %+v", got)
		}
	})
}

func TestHandleFlushHistory(t *testing.T) {
	t.Run("nil store", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/jobs/flush-history", nil)
		HandleFlushHistory(rec, req, HandlerDeps{})
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d", rec.Code)
		}
	})

	t.Run("method not allowed", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs/flush-history", nil)
		HandleFlushHistory(rec, req, HandlerDeps{Store: &stubStore{}})
		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("expected 405, got %d", rec.Code)
		}
	})

	t.Run("store error", func(t *testing.T) {
		store := &stubStore{}
		store.flushJobExecutionHistoryFn = func() (int64, error) { return 0, protocolError("nope") }
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/jobs/flush-history", nil)
		HandleFlushHistory(rec, req, HandlerDeps{Store: store})
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d", rec.Code)
		}
	})

	t.Run("success", func(t *testing.T) {
		store := &stubStore{}
		store.flushJobExecutionHistoryFn = func() (int64, error) { return 11, nil }
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/jobs/flush-history", nil)
		HandleFlushHistory(rec, req, HandlerDeps{Store: store})
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var got FlushHistoryViewResponse
		mustDecodeCollectionJSON(t, rec, &got)
		if got.Flushed != 11 {
			t.Fatalf("unexpected flushed count: %+v", got)
		}
	})
}

type protocolError string

func (e protocolError) Error() string { return string(e) }

func mustDecodeCollectionJSON(t *testing.T, rec *httptest.ResponseRecorder, out any) {
	t.Helper()
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("unexpected content-type %q", ct)
	}
	if err := json.Unmarshal(rec.Body.Bytes(), out); err != nil {
		t.Fatalf("decode json: %v", err)
	}
}

func TestHandleCollectionHistoryPaginationClampsLimits(t *testing.T) {
	store := &stubStore{}
	store.listJobExecutionsFn = func() ([]protocol.JobExecution, error) {
		return []protocol.JobExecution{
			{ID: "h-1", Status: protocol.JobExecutionStatusSucceeded, CreatedUTC: time.Now().UTC()},
			{ID: "h-2", Status: protocol.JobExecutionStatusFailed, CreatedUTC: time.Now().UTC()},
		}, nil
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs?view=history&offset=-5&limit=1000", nil)
	HandleCollection(rec, req, HandlerDeps{Store: store})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var got PagedViewResponse
	mustDecodeCollectionJSON(t, rec, &got)
	if got.Offset != 0 || got.Limit != 200 {
		t.Fatalf("expected clamped offset/limit, got offset=%d limit=%d", got.Offset, got.Limit)
	}
}
