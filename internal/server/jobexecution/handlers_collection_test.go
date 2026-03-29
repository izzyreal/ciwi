package jobexecution

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/izzyreal/ciwi/internal/protocol"
)

func TestHandleCollectionPagedViewHistory(t *testing.T) {
	store := &stubStore{}
	store.listJobExecutionsFn = func() ([]protocol.JobExecution, error) {
		return []protocol.JobExecution{
			{ID: "q-1", Status: protocol.JobExecutionStatusQueued},
			{ID: "q-2", Status: protocol.JobExecutionStatusLeased},
			{ID: "q-3", Status: protocol.JobExecutionStatusRunning},
			{ID: "h-1", Status: protocol.JobExecutionStatusSucceeded},
			{ID: "h-2", Status: protocol.JobExecutionStatusFailed},
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
	req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs?view=history&offset=1&limit=1", nil)
	HandleCollection(rec, req, deps)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var got PagedViewResponse
	mustDecodeCollectionJSON(t, rec, &got)
	if got.View != "history" || got.Total != 2 || got.Offset != 1 || got.Limit != 1 {
		t.Fatalf("unexpected page metadata: %+v", got)
	}
	if len(got.JobExecutions) != 1 || got.JobExecutions[0].ID != "h-2" {
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
