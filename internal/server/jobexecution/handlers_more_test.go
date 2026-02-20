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

func TestHandleByIDRootBranchesAndRouting(t *testing.T) {
	t.Run("store unavailable", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs/job-1", nil)
		HandleByID(rec, req, HandlerDeps{})
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d", rec.Code)
		}
	})

	t.Run("invalid and unknown paths", func(t *testing.T) {
		store := &stubStore{}

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs/", nil)
		HandleByID(rec, req, HandlerDeps{Store: store})
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404 for invalid path, got %d", rec.Code)
		}

		rec = httptest.NewRecorder()
		req = httptest.NewRequest(http.MethodGet, "/api/v1/jobs/job-1/unknown", nil)
		HandleByID(rec, req, HandlerDeps{Store: store})
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404 for unknown resource, got %d", rec.Code)
		}
	})

	t.Run("root get and hook attachments", func(t *testing.T) {
		store := &stubStore{
			getJobExecutionFn: func(id string) (protocol.JobExecution, error) {
				if id != "job-1" {
					t.Fatalf("unexpected job id: %q", id)
				}
				return protocol.JobExecution{ID: id, Status: protocol.JobExecutionStatusRunning}, nil
			},
		}
		attachTest := false
		attachReq := false
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs/job-1", nil)
		HandleByID(rec, req, HandlerDeps{
			Store: store,
			AttachTestSummary: func(job *protocol.JobExecution) {
				attachTest = true
				job.Output = "with-summary"
			},
			AttachUnmetRequirementsToExecution: func(job *protocol.JobExecution) {
				attachReq = true
				job.UnmetRequirements = []string{"tool:cmake:missing"}
			},
		})
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if !attachTest || !attachReq {
			t.Fatalf("expected both attach hooks to run")
		}
		if !strings.Contains(rec.Body.String(), "with-summary") || !strings.Contains(rec.Body.String(), "\"unmet_requirements\"") {
			t.Fatalf("expected enriched payload, got %s", rec.Body.String())
		}
	})

	t.Run("root get not found", func(t *testing.T) {
		store := &stubStore{
			getJobExecutionFn: func(string) (protocol.JobExecution, error) {
				return protocol.JobExecution{}, errors.New("missing")
			},
		}
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs/job-1", nil)
		HandleByID(rec, req, HandlerDeps{Store: store})
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", rec.Code)
		}
	})

	t.Run("root delete branches", func(t *testing.T) {
		store := &stubStore{}

		store.deleteQueuedJobExecutionFn = func(id string) error { return errors.New("job not found") }
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodDelete, "/api/v1/jobs/job-1", nil)
		HandleByID(rec, req, HandlerDeps{Store: store})
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", rec.Code)
		}

		store.deleteQueuedJobExecutionFn = func(id string) error { return errors.New("already running") }
		rec = httptest.NewRecorder()
		req = httptest.NewRequest(http.MethodDelete, "/api/v1/jobs/job-1", nil)
		HandleByID(rec, req, HandlerDeps{Store: store})
		if rec.Code != http.StatusConflict {
			t.Fatalf("expected 409, got %d", rec.Code)
		}

		store.deleteQueuedJobExecutionFn = func(id string) error { return nil }
		rec = httptest.NewRecorder()
		req = httptest.NewRequest(http.MethodDelete, "/api/v1/jobs/job-1", nil)
		HandleByID(rec, req, HandlerDeps{Store: store})
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "\"deleted\":true") {
			t.Fatalf("expected delete payload, got %s", rec.Body.String())
		}
	})

	t.Run("root method not allowed", func(t *testing.T) {
		store := &stubStore{}
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPatch, "/api/v1/jobs/job-1", nil)
		HandleByID(rec, req, HandlerDeps{Store: store})
		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("expected 405, got %d", rec.Code)
		}
	})
}

func TestHandleByIDArtifactsDownloadAllErrorFromStore(t *testing.T) {
	store := &stubStore{
		listJobExecutionArtifactsFn: func(id string) ([]protocol.JobExecutionArtifact, error) {
			return nil, errors.New("db unavailable")
		},
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs/job-1/artifacts/download-all", nil)
	HandleByID(rec, req, HandlerDeps{Store: store, ArtifactsDir: t.TempDir()})
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}

func TestNowUTCUsesDepsClockOrFallback(t *testing.T) {
	fixed := time.Date(2026, time.February, 20, 12, 0, 0, 0, time.FixedZone("PDT", -7*3600))
	got := nowUTC(HandlerDeps{
		Now: func() time.Time { return fixed },
	})
	if !got.Equal(fixed.UTC()) {
		t.Fatalf("expected fixed UTC time, got %s want %s", got, fixed.UTC())
	}

	before := time.Now().UTC()
	got = nowUTC(HandlerDeps{
		Now: func() time.Time { return time.Time{} },
	})
	after := time.Now().UTC()
	if got.Before(before.Add(-1*time.Second)) || got.After(after.Add(1*time.Second)) {
		t.Fatalf("expected fallback to current UTC time, got %s (range %s..%s)", got, before, after)
	}
}
