package application

import (
	"context"
	"reflect"
	"testing"
)

type executionMutatorStub struct {
	cleared     int64
	deleted     []string
	clearCalls  int
	removeCalls int
	removedID   string
	flushCalls  int
	all         bool
	ids         []string
}

func (s *executionMutatorStub) ClearQueuedExecutions(context.Context) (int64, error) {
	s.clearCalls++
	return s.cleared, nil
}

func (s *executionMutatorStub) RemoveQueuedExecution(_ context.Context, jobID string) error {
	s.removeCalls++
	s.removedID = jobID
	return nil
}

func (s *executionMutatorStub) FlushExecutionHistory(_ context.Context, all bool, ids []string) ([]string, error) {
	s.flushCalls++
	s.all = all
	s.ids = append([]string(nil), ids...)
	return append([]string(nil), s.deleted...), nil
}

func TestExecutionCommandsAreIdempotentAndPublishChanges(t *testing.T) {
	mutator := &executionMutatorStub{cleared: 3, deleted: []string{"job-1", "job-2"}}
	receipts := newReceiptRepositoryStub()
	changes := NewChangeHub()
	commands := NewExecutionCommands(mutator, receipts, changes)

	first, err := commands.ClearQueue(t.Context(), ClearExecutionQueueRequest{IdempotencyKey: "clear-1"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := commands.ClearQueue(t.Context(), ClearExecutionQueueRequest{IdempotencyKey: "clear-1"})
	if err != nil {
		t.Fatal(err)
	}
	if first.Cleared != 3 || second != first || mutator.clearCalls != 1 {
		t.Fatalf("clear results = %#v, %#v; calls = %d", first, second, mutator.clearCalls)
	}
	removed, err := commands.RemoveQueued(t.Context(), RemoveQueuedExecutionRequest{JobExecutionID: " job-q ", IdempotencyKey: "remove-1"})
	if err != nil {
		t.Fatal(err)
	}
	replayedRemove, err := commands.RemoveQueued(t.Context(), RemoveQueuedExecutionRequest{JobExecutionID: "job-q", IdempotencyKey: "remove-1"})
	if err != nil {
		t.Fatal(err)
	}
	if !removed.Removed || replayedRemove != removed || mutator.removeCalls != 1 || mutator.removedID != "job-q" {
		t.Fatalf("remove results = %#v, %#v; calls = %d id = %q", removed, replayedRemove, mutator.removeCalls, mutator.removedID)
	}

	request := FlushExecutionHistoryRequest{JobExecutionIDs: []string{" job-1 ", "job-1", "job-2"}, IdempotencyKey: "flush-1"}
	flushed, err := commands.FlushHistory(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := commands.FlushHistory(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	if flushed.Flushed != 2 || replayed != flushed || mutator.flushCalls != 1 {
		t.Fatalf("flush results = %#v, %#v; calls = %d", flushed, replayed, mutator.flushCalls)
	}
	if mutator.all || !reflect.DeepEqual(mutator.ids, []string{"job-1", "job-2"}) {
		t.Fatalf("flush request = all %v, ids %v", mutator.all, mutator.ids)
	}
	if got := changes.Snapshot().Revision; got != 3 {
		t.Fatalf("change revision = %d, want 3", got)
	}
}

func TestExecutionCommandsValidateFlushScope(t *testing.T) {
	commands := NewExecutionCommands(&executionMutatorStub{}, nil, nil)
	tests := []FlushExecutionHistoryRequest{
		{All: true, JobExecutionIDs: []string{"job-1"}},
	}
	for _, request := range tests {
		if _, err := commands.FlushHistory(t.Context(), request); ErrorKindOf(err) != ErrorInvalidArgument {
			t.Fatalf("FlushHistory(%#v) error = %v", request, err)
		}
	}
}

func TestExecutionCommandsFlushAll(t *testing.T) {
	mutator := &executionMutatorStub{deleted: []string{"job-1"}}
	commands := NewExecutionCommands(mutator, nil, nil)
	result, err := commands.FlushHistory(t.Context(), FlushExecutionHistoryRequest{All: true})
	if err != nil {
		t.Fatal(err)
	}
	if result.Flushed != 1 || !mutator.all || len(mutator.ids) != 0 {
		t.Fatalf("result = %#v, all = %v, ids = %v", result, mutator.all, mutator.ids)
	}
}
