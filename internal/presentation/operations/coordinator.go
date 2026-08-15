// Package operations coordinates asynchronous user intent without depending on
// a renderer, transport, or persistence implementation.
package operations

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

type Class string

const (
	ClassQuery    Class = "query"
	ClassMutation Class = "mutation"
)

type State string

const (
	StateQueued               State = "queued"
	StateWaitingForConnection State = "waiting-for-connection"
	StateRunning              State = "running"
	StateVerifying            State = "verifying"
	StateSucceeded            State = "succeeded"
	StateFailed               State = "failed"
	StateCancelled            State = "cancelled"
	StateOutcomeUnknown       State = "outcome-unknown"
)

func (s State) Terminal() bool {
	return s == StateSucceeded || s == StateFailed || s == StateCancelled || s == StateOutcomeUnknown
}

type Definition struct {
	Command     string
	Class       Class
	Scope       string
	Pending     string
	Persistence string
}

type Request struct {
	Definition Definition
	Arguments  map[string]string
}

type Operation struct {
	ID             string            `json:"id"`
	IdempotencyKey string            `json:"idempotencyKey"`
	Command        string            `json:"command"`
	Arguments      map[string]string `json:"arguments,omitempty"`
	Fingerprint    string            `json:"fingerprint"`
	Scope          string            `json:"scope"`
	Class          Class             `json:"class"`
	Persistence    string            `json:"persistence,omitempty"`
	PendingLabel   string            `json:"pendingLabel,omitempty"`
	State          State             `json:"state"`
	Message        string            `json:"message,omitempty"`
	CreatedAt      time.Time         `json:"createdAt"`
	UpdatedAt      time.Time         `json:"updatedAt"`
	// Value is an adapter-owned outcome. It is deliberately excluded from the
	// durable representation so the coordinator remains renderer- and
	// transport-neutral.
	Value any `json:"-"`
}

type Result struct {
	State   State
	Message string
	Err     error
	Value   any
}

type Executor interface {
	Execute(context.Context, Operation) Result
}

type Journal interface {
	Put(Operation) error
	Delete(string) error
}

type Disposition string

const (
	DispositionAccepted  Disposition = "accepted"
	DispositionDuplicate Disposition = "duplicate"
	DispositionConflict  Disposition = "conflict"
)

type Submission struct {
	Disposition Disposition
	Operation   Operation
	Conflict    *Operation
}

type Coordinator struct {
	ctx      context.Context
	cancel   context.CancelFunc
	executor Executor
	journal  Journal
	workers  chan struct{}
	now      func() time.Time
	newID    func() string

	mu            sync.RWMutex
	operations    map[string]*Operation
	byFingerprint map[string]string
	byScope       map[string]string
	cancels       map[string]context.CancelFunc
	changed       chan struct{}
}

func New(parent context.Context, workerLimit int, executor Executor, journal Journal) *Coordinator {
	if workerLimit < 1 {
		workerLimit = 1
	}
	ctx, cancel := context.WithCancel(parent)
	return &Coordinator{
		ctx: ctx, cancel: cancel, executor: executor, journal: journal,
		workers: make(chan struct{}, workerLimit), now: time.Now, newID: uuid.NewString,
		operations: map[string]*Operation{}, byFingerprint: map[string]string{},
		byScope: map[string]string{}, cancels: map[string]context.CancelFunc{}, changed: make(chan struct{}, 1),
	}
}

func (c *Coordinator) Close() { c.cancel() }

// Changed is edge-triggered. Consumers must call Snapshot after receiving it;
// the snapshot, not the notification channel, is the source of truth.
func (c *Coordinator) Changed() <-chan struct{} { return c.changed }

func (c *Coordinator) Submit(request Request) (Submission, error) {
	definition := request.Definition
	if strings.TrimSpace(definition.Command) == "" {
		return Submission{}, errors.New("operation command is required")
	}
	if definition.Class != ClassQuery && definition.Class != ClassMutation {
		return Submission{}, errors.New("only query and mutation operations may be coordinated")
	}
	arguments := cloneArguments(request.Arguments)
	fingerprint, err := Fingerprint(definition.Command, arguments)
	if err != nil {
		return Submission{}, err
	}
	now := c.now().UTC()

	c.mu.Lock()
	if id := c.byFingerprint[fingerprint]; id != "" {
		if existing := c.operations[id]; existing != nil && !existing.State.Terminal() {
			copy := cloneOperation(*existing)
			c.mu.Unlock()
			return Submission{Disposition: DispositionDuplicate, Operation: copy}, nil
		}
	}
	if id := c.byScope[definition.Scope]; id != "" {
		if existing := c.operations[id]; existing != nil && !existing.State.Terminal() {
			if definition.Class == ClassQuery && existing.Class == ClassQuery {
				if cancel := c.cancels[id]; cancel != nil {
					cancel()
				}
			} else {
				copy := cloneOperation(*existing)
				c.mu.Unlock()
				return Submission{Disposition: DispositionConflict, Conflict: &copy}, nil
			}
		}
	}
	operation := Operation{
		ID: c.newID(), IdempotencyKey: c.newID(), Command: definition.Command,
		Arguments: arguments, Fingerprint: fingerprint, Scope: definition.Scope,
		Class: definition.Class, Persistence: definition.Persistence,
		PendingLabel: definition.Pending, State: StateQueued, CreatedAt: now, UpdatedAt: now,
	}
	if operation.Scope == "" {
		operation.Scope = definition.Command
	}
	if operation.Class == ClassMutation && c.journal != nil {
		if err := c.journal.Put(operation); err != nil {
			c.mu.Unlock()
			return Submission{}, err
		}
	}
	c.operations[operation.ID] = &operation
	c.byFingerprint[operation.Fingerprint] = operation.ID
	c.byScope[operation.Scope] = operation.ID
	opCtx, cancel := context.WithCancel(c.ctx)
	c.cancels[operation.ID] = cancel
	copy := cloneOperation(operation)
	c.mu.Unlock()
	c.notify()
	go c.execute(opCtx, operation.ID)
	return Submission{Disposition: DispositionAccepted, Operation: copy}, nil
}

// Restore resumes a previously journaled safe mutation with its original
// operation and idempotency identities. Callers must first verify that the
// journal belongs to the connected server installation.
func (c *Coordinator) Restore(operation Operation) (Submission, error) {
	if operation.Class != ClassMutation || strings.TrimSpace(operation.ID) == "" || strings.TrimSpace(operation.IdempotencyKey) == "" {
		return Submission{}, errors.New("restored mutation requires operation and idempotency identities")
	}
	if strings.TrimSpace(operation.Command) == "" || strings.TrimSpace(operation.Fingerprint) == "" {
		return Submission{}, errors.New("restored mutation is incomplete")
	}
	c.mu.Lock()
	if existing := c.operations[operation.ID]; existing != nil {
		copy := cloneOperation(*existing)
		c.mu.Unlock()
		return Submission{Disposition: DispositionDuplicate, Operation: copy}, nil
	}
	if id := c.byFingerprint[operation.Fingerprint]; id != "" {
		if existing := c.operations[id]; existing != nil && !existing.State.Terminal() {
			copy := cloneOperation(*existing)
			c.mu.Unlock()
			return Submission{Disposition: DispositionDuplicate, Operation: copy}, nil
		}
	}
	if id := c.byScope[operation.Scope]; id != "" {
		if existing := c.operations[id]; existing != nil && !existing.State.Terminal() {
			copy := cloneOperation(*existing)
			c.mu.Unlock()
			return Submission{Disposition: DispositionConflict, Conflict: &copy}, nil
		}
	}
	operation.Arguments = cloneArguments(operation.Arguments)
	operation.State = StateQueued
	operation.Message = ""
	operation.UpdatedAt = c.now().UTC()
	if operation.CreatedAt.IsZero() {
		operation.CreatedAt = operation.UpdatedAt
	}
	if c.journal != nil {
		if err := c.journal.Put(operation); err != nil {
			c.mu.Unlock()
			return Submission{}, err
		}
	}
	c.operations[operation.ID] = &operation
	c.byFingerprint[operation.Fingerprint] = operation.ID
	c.byScope[operation.Scope] = operation.ID
	opCtx, cancel := context.WithCancel(c.ctx)
	c.cancels[operation.ID] = cancel
	copy := cloneOperation(operation)
	c.mu.Unlock()
	c.notify()
	go c.execute(opCtx, operation.ID)
	return Submission{Disposition: DispositionAccepted, Operation: copy}, nil
}

func (c *Coordinator) execute(ctx context.Context, id string) {
	select {
	case c.workers <- struct{}{}:
	case <-ctx.Done():
		c.finish(id, Result{State: StateCancelled, Err: ctx.Err()})
		return
	}
	defer func() { <-c.workers }()
	c.setState(id, StateRunning, "")
	c.mu.RLock()
	operation := cloneOperation(*c.operations[id])
	c.mu.RUnlock()
	result := Result{State: StateSucceeded}
	if c.executor != nil {
		result = c.executor.Execute(ctx, operation)
	}
	if result.State == "" {
		if result.Err != nil {
			result.State = StateFailed
		} else {
			result.State = StateSucceeded
		}
	}
	c.finish(id, result)
}

func (c *Coordinator) setState(id string, state State, message string) {
	c.mu.Lock()
	if operation := c.operations[id]; operation != nil && !operation.State.Terminal() {
		operation.State = state
		operation.Message = message
		operation.UpdatedAt = c.now().UTC()
		if operation.Class == ClassMutation && c.journal != nil {
			_ = c.journal.Put(cloneOperation(*operation))
		}
	}
	c.mu.Unlock()
	c.notify()
}

func (c *Coordinator) finish(id string, result Result) {
	c.mu.Lock()
	operation := c.operations[id]
	if operation == nil || operation.State.Terminal() {
		c.mu.Unlock()
		return
	}
	operation.State = result.State
	operation.Message = result.Message
	if operation.Message == "" && result.Err != nil {
		operation.Message = result.Err.Error()
	}
	operation.UpdatedAt = c.now().UTC()
	operation.Value = result.Value
	delete(c.byFingerprint, operation.Fingerprint)
	if c.byScope[operation.Scope] == id {
		delete(c.byScope, operation.Scope)
	}
	delete(c.cancels, id)
	if operation.Class == ClassMutation && c.journal != nil {
		switch operation.State {
		case StateSucceeded, StateFailed, StateCancelled:
			_ = c.journal.Delete(id)
		case StateOutcomeUnknown:
			_ = c.journal.Put(cloneOperation(*operation))
		}
	}
	c.mu.Unlock()
	c.notify()
}

// CancelActive asks every active operation to stop. Executors distinguish
// queries, which are safely cancelled, from mutations whose server outcome may
// require receipt reconciliation.
func (c *Coordinator) CancelActive() {
	c.mu.RLock()
	cancels := make([]context.CancelFunc, 0)
	for id, operation := range c.operations {
		if !operation.State.Terminal() {
			if cancel := c.cancels[id]; cancel != nil {
				cancels = append(cancels, cancel)
			}
		}
	}
	c.mu.RUnlock()
	for _, cancel := range cancels {
		cancel()
	}
}

func (c *Coordinator) Snapshot() []Operation {
	c.mu.RLock()
	result := make([]Operation, 0, len(c.operations))
	for _, operation := range c.operations {
		result = append(result, cloneOperation(*operation))
	}
	c.mu.RUnlock()
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.Before(result[j].CreatedAt) })
	return result
}

func (c *Coordinator) ActiveFor(command string, arguments map[string]string) (Operation, bool) {
	fingerprint, err := Fingerprint(command, arguments)
	if err != nil {
		return Operation{}, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	id := c.byFingerprint[fingerprint]
	operation := c.operations[id]
	if operation == nil || operation.State.Terminal() {
		return Operation{}, false
	}
	return cloneOperation(*operation), true
}

// Forget removes a terminal operation after its outcome has been consumed by
// an adapter. Active operations are never removed.
func (c *Coordinator) Forget(id string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	operation := c.operations[id]
	if operation == nil || !operation.State.Terminal() {
		return false
	}
	delete(c.operations, id)
	return true
}

func (c *Coordinator) notify() {
	select {
	case c.changed <- struct{}{}:
	default:
	}
}

func Fingerprint(command string, arguments map[string]string) (string, error) {
	payload, err := json.Marshal(struct {
		Command   string            `json:"command"`
		Arguments map[string]string `json:"arguments"`
	}{Command: command, Arguments: arguments})
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:]), nil
}

func cloneArguments(arguments map[string]string) map[string]string {
	result := make(map[string]string, len(arguments))
	for key, value := range arguments {
		result[key] = value
	}
	return result
}

func cloneOperation(operation Operation) Operation {
	operation.Arguments = cloneArguments(operation.Arguments)
	return operation
}
