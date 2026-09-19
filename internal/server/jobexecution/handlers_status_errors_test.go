package jobexecution

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/izzyreal/ciwi/internal/protocol"
)

func TestStatusPersistenceErrorClassification(t *testing.T) {
	for _, tt := range []struct {
		name, message string
		code          int
	}{
		{"database failure", "update job status: database is locked", 500},
		{"missing job", "job not found", 404},
		{"ownership conflict", "job is leased by another agent", 409},
	} {
		t.Run(tt.name, func(t *testing.T) {
			store := &stubStore{
				getJobExecutionFn: func(id string) (protocol.JobExecution, error) { return protocol.JobExecution{ID: id}, nil },
				updateJobExecutionStatusFn: func(string, protocol.JobExecutionStatusUpdateRequest) (protocol.JobExecution, error) {
					return protocol.JobExecution{}, errors.New(tt.message)
				},
			}
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/jobs/job/status", strings.NewReader(`{"agent_id":"agent","status":"running"}`))
			handleJobStatus(rec, req, HandlerDeps{Store: store}, "job")
			if rec.Code != tt.code {
				t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
			}
		})
	}
}
