package application

import (
	"context"
	"errors"
	"testing"
)

type pipelineRunnerStub struct {
	calls  int
	result RunPipelineResult
	err    error
}

func (s *pipelineRunnerStub) RunPipeline(context.Context, RunPipelineRequest) (RunPipelineResult, error) {
	s.calls++
	return s.result, s.err
}

type receiptRepositoryStub struct {
	receipts map[string]CommandReceipt
}

func newReceiptRepositoryStub() *receiptRepositoryStub {
	return &receiptRepositoryStub{receipts: map[string]CommandReceipt{}}
}

func (s *receiptRepositoryStub) Claim(_ context.Context, key, operation, fingerprint string) (CommandReceipt, bool, error) {
	if receipt, ok := s.receipts[key]; ok {
		return receipt, false, nil
	}
	receipt := CommandReceipt{Key: key, Operation: operation, Fingerprint: fingerprint, Status: "pending"}
	s.receipts[key] = receipt
	return receipt, true, nil
}

func (s *receiptRepositoryStub) Complete(_ context.Context, key string, result []byte) error {
	receipt := s.receipts[key]
	receipt.Status = "completed"
	receipt.Result = append([]byte(nil), result...)
	s.receipts[key] = receipt
	return nil
}

func (s *receiptRepositoryStub) Fail(_ context.Context, key string, result []byte) error {
	receipt := s.receipts[key]
	receipt.Status = "failed"
	receipt.Result = append([]byte(nil), result...)
	s.receipts[key] = receipt
	return nil
}

func TestPipelineCommandsDeduplicatesCompletedCommand(t *testing.T) {
	runner := &pipelineRunnerStub{result: RunPipelineResult{
		ProjectName: "ciwi", PipelineID: "build", Enqueued: 1, JobExecutionIDs: []string{"job-1"},
	}}
	receipts := newReceiptRepositoryStub()
	hub := NewChangeHub()
	commands := NewPipelineCommands(runner, receipts, hub)
	request := RunPipelineRequest{PipelineDBID: 7, DryRun: true, IdempotencyKey: "command-1"}

	first, err := commands.RunPipeline(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	second, err := commands.RunPipeline(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if runner.calls != 1 {
		t.Fatalf("runner calls=%d want=1", runner.calls)
	}
	if first.JobExecutionIDs[0] != second.JobExecutionIDs[0] {
		t.Fatalf("receipt result mismatch: first=%+v second=%+v", first, second)
	}
	if got := hub.Snapshot().Revision; got != 1 {
		t.Fatalf("change revision=%d want=1", got)
	}
}

func TestPipelineCommandsRejectsReusedKeyForDifferentRequest(t *testing.T) {
	runner := &pipelineRunnerStub{result: RunPipelineResult{PipelineID: "build"}}
	commands := NewPipelineCommands(runner, newReceiptRepositoryStub(), nil)
	if _, err := commands.RunPipeline(context.Background(), RunPipelineRequest{PipelineDBID: 1, IdempotencyKey: "same"}); err != nil {
		t.Fatal(err)
	}
	_, err := commands.RunPipeline(context.Background(), RunPipelineRequest{PipelineDBID: 2, IdempotencyKey: "same"})
	if ErrorKindOf(err) != ErrorConflict {
		t.Fatalf("error=%v kind=%q want conflict", err, ErrorKindOf(err))
	}
}

func TestPipelineCommandsRejectsOversizedIdempotencyKey(t *testing.T) {
	runner := &pipelineRunnerStub{}
	commands := NewPipelineCommands(runner, newReceiptRepositoryStub(), nil)
	_, err := commands.RunPipeline(context.Background(), RunPipelineRequest{PipelineDBID: 1, IdempotencyKey: string(make([]byte, 201))})
	if ErrorKindOf(err) != ErrorInvalidArgument || runner.calls != 0 {
		t.Fatalf("error=%v runner calls=%d", err, runner.calls)
	}
}

func TestPipelineCommandsReplaysFailureWithoutRepeatingCommand(t *testing.T) {
	runner := &pipelineRunnerStub{err: errors.New("failed")}
	receipts := newReceiptRepositoryStub()
	commands := NewPipelineCommands(runner, receipts, nil)
	request := RunPipelineRequest{PipelineDBID: 1, IdempotencyKey: "retry"}
	_, err := commands.RunPipeline(context.Background(), request)
	if err == nil {
		t.Fatal("expected error")
	}
	_, err = commands.RunPipeline(context.Background(), request)
	if err == nil || runner.calls != 1 {
		t.Fatalf("replayed error=%v runner calls=%d", err, runner.calls)
	}
	if receipt := receipts.receipts["retry"]; receipt.Status != "failed" {
		t.Fatalf("failed command receipt = %+v", receipt)
	}
}
