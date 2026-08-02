package application

import (
	"context"
	"testing"
)

type executionControllerStub struct {
	cancelCalls int
	rerunCalls  int
}

func (s *executionControllerStub) CancelExecution(_ context.Context, jobID string) (CancelExecutionResult, error) {
	s.cancelCalls++
	return CancelExecutionResult{JobExecutionID: jobID, Status: "failed"}, nil
}

func (s *executionControllerStub) RerunExecution(_ context.Context, jobID string) (RerunExecutionResult, error) {
	s.rerunCalls++
	return RerunExecutionResult{OriginalJobExecutionID: jobID, JobExecutionID: "rerun-1", Status: "queued"}, nil
}

func TestExecutionControlCommandsAreIdempotent(t *testing.T) {
	controller := &executionControllerStub{}
	hub := NewChangeHub()
	commands := NewExecutionControlCommands(controller, newReceiptRepositoryStub(), hub)
	request := ExecutionControlRequest{JobExecutionID: "job-1", IdempotencyKey: "cancel-1"}
	first, err := commands.Cancel(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	second, err := commands.Cancel(t.Context(), request)
	if err != nil || second != first || controller.cancelCalls != 1 {
		t.Fatalf("cancel results = %#v, %#v; calls = %d; err = %v", first, second, controller.cancelCalls, err)
	}
	rerunRequest := ExecutionControlRequest{JobExecutionID: "job-1", IdempotencyKey: "rerun-1"}
	rerun, err := commands.Rerun(t.Context(), rerunRequest)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := commands.Rerun(t.Context(), rerunRequest)
	if err != nil || replayed != rerun || controller.rerunCalls != 1 {
		t.Fatalf("rerun results = %#v, %#v; calls = %d; err = %v", rerun, replayed, controller.rerunCalls, err)
	}
	if got := hub.Snapshot().Revision; got != 2 {
		t.Fatalf("change revision = %d, want 2", got)
	}
}

func TestExecutionControlCommandsValidateJobID(t *testing.T) {
	commands := NewExecutionControlCommands(&executionControllerStub{}, nil, nil)
	if _, err := commands.Cancel(t.Context(), ExecutionControlRequest{}); ErrorKindOf(err) != ErrorInvalidArgument {
		t.Fatalf("Cancel() error = %v", err)
	}
}
