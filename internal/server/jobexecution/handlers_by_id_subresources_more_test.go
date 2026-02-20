package jobexecution

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/izzyreal/ciwi/internal/protocol"
)

func TestHandleByIDSubresourcesAdditionalBranches(t *testing.T) {
	t.Run("cancel method not allowed", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs/job-1/cancel", nil)
		HandleByID(rec, req, HandlerDeps{Store: &stubStore{}, ArtifactsDir: t.TempDir()})
		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("expected 405, got %d", rec.Code)
		}
	})

	t.Run("cancel job not found", func(t *testing.T) {
		store := &stubStore{
			getJobExecutionFn: func(id string) (protocol.JobExecution, error) {
				return protocol.JobExecution{}, errors.New("missing")
			},
		}
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/jobs/job-1/cancel", nil)
		HandleByID(rec, req, HandlerDeps{Store: store, ArtifactsDir: t.TempDir()})
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", rec.Code)
		}
	})

	t.Run("rerun rejects not started job", func(t *testing.T) {
		store := &stubStore{
			getJobExecutionFn: func(id string) (protocol.JobExecution, error) {
				return protocol.JobExecution{ID: id}, nil
			},
		}
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/jobs/job-1/rerun", nil)
		HandleByID(rec, req, HandlerDeps{Store: store, ArtifactsDir: t.TempDir()})
		if rec.Code != http.StatusConflict {
			t.Fatalf("expected 409, got %d", rec.Code)
		}
	})

	t.Run("status validation and conflict branches", func(t *testing.T) {
		store := &stubStore{}

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/jobs/job-1/status", strings.NewReader("{bad"))
		HandleByID(rec, req, HandlerDeps{Store: store, ArtifactsDir: t.TempDir()})
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 invalid json, got %d", rec.Code)
		}

		rec = httptest.NewRecorder()
		req = httptest.NewRequest(http.MethodPost, "/api/v1/jobs/job-1/status", strings.NewReader(`{"status":"running"}`))
		HandleByID(rec, req, HandlerDeps{Store: store, ArtifactsDir: t.TempDir()})
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 missing agent_id, got %d", rec.Code)
		}

		rec = httptest.NewRecorder()
		req = httptest.NewRequest(http.MethodPost, "/api/v1/jobs/job-1/status", strings.NewReader(`{"agent_id":"a1","status":"queued"}`))
		HandleByID(rec, req, HandlerDeps{Store: store, ArtifactsDir: t.TempDir()})
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 invalid status, got %d", rec.Code)
		}

		store.updateJobExecutionStatusFn = func(id string, req protocol.JobExecutionStatusUpdateRequest) (protocol.JobExecution, error) {
			return protocol.JobExecution{}, errors.New("job is leased by another agent")
		}
		rec = httptest.NewRecorder()
		req = httptest.NewRequest(http.MethodPost, "/api/v1/jobs/job-1/status", strings.NewReader(`{"agent_id":"a1","status":"running"}`))
		HandleByID(rec, req, HandlerDeps{Store: store, ArtifactsDir: t.TempDir()})
		if rec.Code != http.StatusConflict {
			t.Fatalf("expected 409 for another agent, got %d", rec.Code)
		}

		store.updateJobExecutionStatusFn = func(id string, req protocol.JobExecutionStatusUpdateRequest) (protocol.JobExecution, error) {
			return protocol.JobExecution{}, errors.New("db down")
		}
		rec = httptest.NewRecorder()
		req = httptest.NewRequest(http.MethodPost, "/api/v1/jobs/job-1/status", strings.NewReader(`{"agent_id":"a1","status":"running","timestamp_utc":"`+time.Now().UTC().Format(time.RFC3339Nano)+`"}`))
		HandleByID(rec, req, HandlerDeps{Store: store, ArtifactsDir: t.TempDir()})
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for generic update error, got %d", rec.Code)
		}
	})

	t.Run("artifacts post validation and lease branches", func(t *testing.T) {
		store := &stubStore{}

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/jobs/job-1/artifacts", strings.NewReader("{bad"))
		HandleByID(rec, req, HandlerDeps{Store: store, ArtifactsDir: t.TempDir()})
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 invalid json, got %d", rec.Code)
		}

		rec = httptest.NewRecorder()
		req = httptest.NewRequest(http.MethodPost, "/api/v1/jobs/job-1/artifacts", strings.NewReader(`{"artifacts":[]}`))
		HandleByID(rec, req, HandlerDeps{Store: store, ArtifactsDir: t.TempDir()})
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 missing agent_id, got %d", rec.Code)
		}

		store.getJobExecutionFn = func(id string) (protocol.JobExecution, error) {
			return protocol.JobExecution{}, errors.New("missing")
		}
		rec = httptest.NewRecorder()
		req = httptest.NewRequest(http.MethodPost, "/api/v1/jobs/job-1/artifacts", strings.NewReader(`{"agent_id":"a1","artifacts":[]}`))
		HandleByID(rec, req, HandlerDeps{Store: store, ArtifactsDir: t.TempDir()})
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404 job not found, got %d", rec.Code)
		}

		store.getJobExecutionFn = func(id string) (protocol.JobExecution, error) {
			return protocol.JobExecution{ID: id, LeasedByAgentID: "other"}, nil
		}
		rec = httptest.NewRecorder()
		req = httptest.NewRequest(http.MethodPost, "/api/v1/jobs/job-1/artifacts", strings.NewReader(`{"agent_id":"a1","artifacts":[]}`))
		HandleByID(rec, req, HandlerDeps{Store: store, ArtifactsDir: t.TempDir()})
		if rec.Code != http.StatusConflict {
			t.Fatalf("expected 409 lease conflict, got %d", rec.Code)
		}
	})

	t.Run("tests endpoint additional branches", func(t *testing.T) {
		store := &stubStore{}

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/api/v1/jobs/job-1/tests", nil)
		HandleByID(rec, req, HandlerDeps{Store: store, ArtifactsDir: t.TempDir()})
		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("expected 405 tests method, got %d", rec.Code)
		}

		store.getJobExecutionTestReportFn = func(id string) (protocol.JobExecutionTestReport, bool, error) {
			return protocol.JobExecutionTestReport{}, false, nil
		}
		rec = httptest.NewRecorder()
		req = httptest.NewRequest(http.MethodGet, "/api/v1/jobs/job-1/tests", nil)
		HandleByID(rec, req, HandlerDeps{Store: store, ArtifactsDir: t.TempDir()})
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 tests get missing, got %d", rec.Code)
		}

		rec = httptest.NewRecorder()
		req = httptest.NewRequest(http.MethodPost, "/api/v1/jobs/job-1/tests", strings.NewReader(`{"report":{"total":1}}`))
		HandleByID(rec, req, HandlerDeps{Store: store, ArtifactsDir: t.TempDir()})
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 missing agent_id, got %d", rec.Code)
		}

		store.getJobExecutionFn = func(id string) (protocol.JobExecution, error) {
			return protocol.JobExecution{}, errors.New("missing")
		}
		rec = httptest.NewRecorder()
		req = httptest.NewRequest(http.MethodPost, "/api/v1/jobs/job-1/tests", strings.NewReader(`{"agent_id":"a1","report":{"total":1}}`))
		HandleByID(rec, req, HandlerDeps{Store: store, ArtifactsDir: t.TempDir()})
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404 tests job missing, got %d", rec.Code)
		}

		store.getJobExecutionFn = func(id string) (protocol.JobExecution, error) {
			return protocol.JobExecution{ID: id, LeasedByAgentID: "other"}, nil
		}
		rec = httptest.NewRecorder()
		req = httptest.NewRequest(http.MethodPost, "/api/v1/jobs/job-1/tests", strings.NewReader(`{"agent_id":"a1","report":{"total":1}}`))
		HandleByID(rec, req, HandlerDeps{Store: store, ArtifactsDir: t.TempDir()})
		if rec.Code != http.StatusConflict {
			t.Fatalf("expected 409 tests lease conflict, got %d", rec.Code)
		}

		store.getJobExecutionFn = func(id string) (protocol.JobExecution, error) {
			return protocol.JobExecution{ID: id, LeasedByAgentID: "a1"}, nil
		}
		store.saveJobExecutionTestReportFn = func(id string, report protocol.JobExecutionTestReport) error {
			return errors.New("save failed")
		}
		rec = httptest.NewRecorder()
		req = httptest.NewRequest(http.MethodPost, "/api/v1/jobs/job-1/tests", strings.NewReader(`{"agent_id":"a1","report":{"total":1}}`))
		HandleByID(rec, req, HandlerDeps{Store: store, ArtifactsDir: t.TempDir()})
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500 tests save failure, got %d", rec.Code)
		}
	})
}
