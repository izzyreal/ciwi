package jobexecution

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/izzyreal/ciwi/internal/application"
	"github.com/izzyreal/ciwi/internal/protocol"
)

type executionControlsStub struct {
	cancel func(context.Context, application.ExecutionControlRequest) (application.CancelExecutionResult, error)
	rerun  func(context.Context, application.ExecutionControlRequest) (application.RerunExecutionResult, error)
}

func (s executionControlsStub) Cancel(ctx context.Context, request application.ExecutionControlRequest) (application.CancelExecutionResult, error) {
	return s.cancel(ctx, request)
}

func (s executionControlsStub) Rerun(ctx context.Context, request application.ExecutionControlRequest) (application.RerunExecutionResult, error) {
	return s.rerun(ctx, request)
}

type executionCommandsStub struct {
	remove func(context.Context, application.RemoveQueuedExecutionRequest) (application.RemoveQueuedExecutionResult, error)
}

func (executionCommandsStub) ClearQueue(context.Context, application.ClearExecutionQueueRequest) (application.ClearExecutionQueueResult, error) {
	panic("unexpected ClearQueue call")
}

func (executionCommandsStub) FlushHistory(context.Context, application.FlushExecutionHistoryRequest) (application.FlushExecutionHistoryResult, error) {
	panic("unexpected FlushHistory call")
}

func (s executionCommandsStub) RemoveQueued(ctx context.Context, request application.RemoveQueuedExecutionRequest) (application.RemoveQueuedExecutionResult, error) {
	return s.remove(ctx, request)
}

func TestHTTPExecutionControlsUseReceiptBackedApplicationCommands(t *testing.T) {
	store := &stubStore{getJobExecutionFn: func(id string) (protocol.JobExecution, error) {
		return protocol.JobExecution{ID: id, Status: protocol.JobExecutionStatusFailed}, nil
	}}
	controls := executionControlsStub{
		cancel: func(_ context.Context, request application.ExecutionControlRequest) (application.CancelExecutionResult, error) {
			if request.JobExecutionID != "job-1" || request.IdempotencyKey != "cancel-key" {
				t.Fatalf("unexpected cancel request: %+v", request)
			}
			return application.CancelExecutionResult{JobExecutionID: "job-1", Status: "failed"}, nil
		},
		rerun: func(_ context.Context, request application.ExecutionControlRequest) (application.RerunExecutionResult, error) {
			if request.JobExecutionID != "job-1" || request.IdempotencyKey != "rerun-key" {
				t.Fatalf("unexpected rerun request: %+v", request)
			}
			return application.RerunExecutionResult{OriginalJobExecutionID: "job-1", JobExecutionID: "job-2", Status: "queued"}, nil
		},
	}

	cancelRecorder := httptest.NewRecorder()
	cancelRequest := httptest.NewRequest(http.MethodPost, "/api/v1/jobs/job-1/cancel", nil)
	cancelRequest.Header.Set("Idempotency-Key", "cancel-key")
	HandleByID(cancelRecorder, cancelRequest, HandlerDeps{Store: store, ExecutionControls: controls})
	if cancelRecorder.Code != http.StatusOK {
		t.Fatalf("cancel status = %d: %s", cancelRecorder.Code, cancelRecorder.Body.String())
	}

	rerunRecorder := httptest.NewRecorder()
	rerunRequest := httptest.NewRequest(http.MethodPost, "/api/v1/jobs/job-1/rerun", nil)
	rerunRequest.Header.Set("Idempotency-Key", "rerun-key")
	HandleByID(rerunRecorder, rerunRequest, HandlerDeps{Store: store, ExecutionControls: controls})
	if rerunRecorder.Code != http.StatusCreated {
		t.Fatalf("rerun status = %d: %s", rerunRecorder.Code, rerunRecorder.Body.String())
	}
}

func TestHTTPQueuedRemovalUsesReceiptBackedApplicationCommand(t *testing.T) {
	commands := executionCommandsStub{remove: func(_ context.Context, request application.RemoveQueuedExecutionRequest) (application.RemoveQueuedExecutionResult, error) {
		if request.JobExecutionID != "job-1" || request.IdempotencyKey != "remove-key" {
			t.Fatalf("unexpected remove request: %+v", request)
		}
		return application.RemoveQueuedExecutionResult{JobExecutionID: "job-1", Removed: true}, nil
	}}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodDelete, "/api/v1/jobs/job-1", nil)
	request.Header.Set("Idempotency-Key", "remove-key")
	HandleByID(recorder, request, HandlerDeps{Store: &stubStore{}, ExecutionCommands: commands})
	if recorder.Code != http.StatusOK {
		t.Fatalf("remove status = %d: %s", recorder.Code, recorder.Body.String())
	}
}
