package operations

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type executorFunc func(context.Context, Operation) Result

func (f executorFunc) Execute(ctx context.Context, operation Operation) Result {
	return f(ctx, operation)
}

func TestCoordinatorCoalescesRapidDuplicateSubmissions(t *testing.T) {
	release := make(chan struct{})
	var calls atomic.Int32
	coordinator := New(context.Background(), 4, executorFunc(func(context.Context, Operation) Result {
		calls.Add(1)
		<-release
		return Result{State: StateSucceeded}
	}), nil)
	defer coordinator.Close()
	request := Request{Definition: Definition{Command: "run-pipeline", Class: ClassMutation, Scope: "pipeline:1"}, Arguments: map[string]string{"pipelineDbId": "1"}}
	first, err := coordinator.Submit(request)
	if err != nil || first.Disposition != DispositionAccepted {
		t.Fatalf("first submission = %#v, %v", first, err)
	}
	for index := 0; index < 100; index++ {
		submission, err := coordinator.Submit(request)
		if err != nil || submission.Disposition != DispositionDuplicate || submission.Operation.ID != first.Operation.ID {
			t.Fatalf("duplicate %d = %#v, %v", index, submission, err)
		}
	}
	close(release)
	waitForState(t, coordinator, first.Operation.ID, StateSucceeded)
	if got := calls.Load(); got != 1 {
		t.Fatalf("executor calls = %d, want 1", got)
	}
}

func TestCoordinatorRejectsSameScopeConflictAndRunsIndependentScopes(t *testing.T) {
	release := make(chan struct{})
	started := make(chan string, 2)
	coordinator := New(context.Background(), 2, executorFunc(func(_ context.Context, operation Operation) Result {
		started <- operation.Scope
		<-release
		return Result{State: StateSucceeded}
	}), nil)
	defer coordinator.Close()
	first, _ := coordinator.Submit(Request{Definition: Definition{Command: "agent-action", Class: ClassMutation, Scope: "agent:a"}, Arguments: map[string]string{"agentId": "a", "action": "restart"}})
	conflict, _ := coordinator.Submit(Request{Definition: Definition{Command: "agent-action", Class: ClassMutation, Scope: "agent:a"}, Arguments: map[string]string{"agentId": "a", "action": "deactivate"}})
	if conflict.Disposition != DispositionConflict || conflict.Conflict == nil || conflict.Conflict.ID != first.Operation.ID {
		t.Fatalf("conflict = %#v", conflict)
	}
	second, _ := coordinator.Submit(Request{Definition: Definition{Command: "agent-action", Class: ClassMutation, Scope: "agent:b"}, Arguments: map[string]string{"agentId": "b", "action": "restart"}})
	seen := map[string]bool{}
	for len(seen) < 2 {
		select {
		case scope := <-started:
			seen[scope] = true
		case <-time.After(time.Second):
			t.Fatal("independent operations did not start concurrently")
		}
	}
	close(release)
	waitForState(t, coordinator, first.Operation.ID, StateSucceeded)
	waitForState(t, coordinator, second.Operation.ID, StateSucceeded)
}

func TestCoordinatorSupersedesQueryInSameScope(t *testing.T) {
	firstCancelled := make(chan struct{})
	firstStarted := make(chan struct{})
	var calls atomic.Int32
	coordinator := New(context.Background(), 2, executorFunc(func(ctx context.Context, _ Operation) Result {
		if calls.Add(1) == 1 {
			close(firstStarted)
			<-ctx.Done()
			close(firstCancelled)
			return Result{State: StateFailed, Err: ctx.Err()}
		}
		return Result{State: StateSucceeded}
	}), nil)
	defer coordinator.Close()
	first, _ := coordinator.Submit(Request{Definition: Definition{Command: "refresh", Class: ClassQuery, Scope: "screen"}, Arguments: map[string]string{"revision": "1"}})
	select {
	case <-firstStarted:
	case <-time.After(time.Second):
		t.Fatal("first query did not start")
	}
	second, _ := coordinator.Submit(Request{Definition: Definition{Command: "refresh", Class: ClassQuery, Scope: "screen"}, Arguments: map[string]string{"revision": "2"}})
	select {
	case <-firstCancelled:
	case <-time.After(time.Second):
		t.Fatal("superseded query was not cancelled")
	}
	waitForState(t, coordinator, first.Operation.ID, StateFailed)
	waitForState(t, coordinator, second.Operation.ID, StateSucceeded)
}

type memoryJournal struct {
	mu   sync.Mutex
	puts []Operation
	dels []string
}

func (j *memoryJournal) Put(operation Operation) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.puts = append(j.puts, operation)
	return nil
}
func (j *memoryJournal) Delete(id string) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.dels = append(j.dels, id)
	return nil
}

func (j *memoryJournal) deleted(id string) bool {
	j.mu.Lock()
	defer j.mu.Unlock()
	for _, deleted := range j.dels {
		if deleted == id {
			return true
		}
	}
	return false
}

func TestCoordinatorJournalsMutationBeforeExecution(t *testing.T) {
	journal := &memoryJournal{}
	coordinator := New(context.Background(), 1, executorFunc(func(context.Context, Operation) Result {
		journal.mu.Lock()
		defer journal.mu.Unlock()
		if len(journal.puts) == 0 {
			t.Error("mutation executed before journal write")
		}
		return Result{State: StateSucceeded}
	}), journal)
	defer coordinator.Close()
	submission, err := coordinator.Submit(Request{Definition: Definition{Command: "clear-queue", Class: ClassMutation, Scope: "queue"}})
	if err != nil {
		t.Fatal(err)
	}
	waitForState(t, coordinator, submission.Operation.ID, StateSucceeded)
}

func TestCoordinatorRestorePreservesIdentitiesAndForgetsConsumedOutcome(t *testing.T) {
	journal := &memoryJournal{}
	executed := make(chan Operation, 1)
	coordinator := New(context.Background(), 1, executorFunc(func(_ context.Context, operation Operation) Result {
		executed <- operation
		return Result{State: StateSucceeded, Message: "done"}
	}), journal)
	defer coordinator.Close()
	restored := Operation{
		ID: "operation-1", IdempotencyKey: "command-1", Command: "clear-queue",
		Arguments: map[string]string{}, Fingerprint: "fingerprint-1", Scope: "execution-queue",
		Class: ClassMutation, State: StateOutcomeUnknown, CreatedAt: time.Now().Add(-time.Minute),
	}
	submission, err := coordinator.Restore(restored)
	if err != nil || submission.Disposition != DispositionAccepted {
		t.Fatalf("restore = %#v, %v", submission, err)
	}
	select {
	case operation := <-executed:
		if operation.ID != restored.ID || operation.IdempotencyKey != restored.IdempotencyKey {
			t.Fatalf("executed identities = %q, %q", operation.ID, operation.IdempotencyKey)
		}
	case <-time.After(time.Second):
		t.Fatal("restored operation did not execute")
	}
	waitForState(t, coordinator, restored.ID, StateSucceeded)
	if !coordinator.Forget(restored.ID) {
		t.Fatal("terminal operation was not forgotten")
	}
	if len(coordinator.Snapshot()) != 0 {
		t.Fatalf("snapshot after forget = %#v", coordinator.Snapshot())
	}
}

func TestCoordinatorOnlyRetainsAmbiguousMutationOutcomes(t *testing.T) {
	for _, test := range []struct {
		name       string
		state      State
		wantDelete bool
	}{
		{name: "known failure", state: StateFailed, wantDelete: true},
		{name: "unknown outcome", state: StateOutcomeUnknown, wantDelete: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			journal := &memoryJournal{}
			coordinator := New(context.Background(), 1, executorFunc(func(context.Context, Operation) Result {
				return Result{State: test.state, Message: "finished"}
			}), journal)
			defer coordinator.Close()
			submission, err := coordinator.Submit(Request{Definition: Definition{
				Command: "clear-queue", Class: ClassMutation, Scope: "queue",
			}})
			if err != nil {
				t.Fatal(err)
			}
			waitForState(t, coordinator, submission.Operation.ID, test.state)
			if got := journal.deleted(submission.Operation.ID); got != test.wantDelete {
				t.Fatalf("journal deleted = %v, want %v", got, test.wantDelete)
			}
		})
	}
}

func waitForState(t *testing.T, coordinator *Coordinator, id string, state State) {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		for _, operation := range coordinator.Snapshot() {
			if operation.ID == id && operation.State == state {
				return
			}
		}
		select {
		case <-coordinator.Changed():
		case <-deadline:
			t.Fatalf("operation %s did not reach %s: %#v", id, state, coordinator.Snapshot())
		}
	}
}
