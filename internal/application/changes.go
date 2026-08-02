package application

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
)

type ChangeTopic string

const (
	ChangeServer           ChangeTopic = "server"
	ChangeProjects         ChangeTopic = "projects"
	ChangeAgents           ChangeTopic = "agents"
	ChangeQueue            ChangeTopic = "queue"
	ChangeHistory          ChangeTopic = "history"
	ChangeUpdates          ChangeTopic = "updates"
	ChangeVault            ChangeTopic = "vault"
	ChangeAgentEligibility ChangeTopic = "agent-eligibility"
)

type Change struct {
	InstanceID string
	Revision   uint64
	Topics     []ChangeTopic
	OccurredAt time.Time
	Resync     bool
}

// ChangeHub broadcasts coalescible invalidations. Publishers never block on a
// slow consumer: a full subscriber receives a resync marker instead.
type ChangeHub struct {
	mu          sync.Mutex
	instanceID  string
	revision    uint64
	nextID      uint64
	subscribers map[uint64]chan Change
	now         func() time.Time
}

func NewChangeHub() *ChangeHub {
	return &ChangeHub{
		instanceID:  uuid.NewString(),
		subscribers: map[uint64]chan Change{},
		now:         func() time.Time { return time.Now().UTC() },
	}
}

func (h *ChangeHub) Snapshot() Change {
	h.mu.Lock()
	defer h.mu.Unlock()
	return Change{InstanceID: h.instanceID, Revision: h.revision, OccurredAt: h.now()}
}

func (h *ChangeHub) Publish(topics ...ChangeTopic) Change {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.revision++
	change := Change{
		InstanceID: h.instanceID,
		Revision:   h.revision,
		Topics:     uniqueTopics(topics),
		OccurredAt: h.now(),
	}
	for _, subscriber := range h.subscribers {
		select {
		case subscriber <- change:
		default:
			select {
			case <-subscriber:
			default:
			}
			resync := change
			resync.Topics = nil
			resync.Resync = true
			select {
			case subscriber <- resync:
			default:
			}
		}
	}
	return change
}

func (h *ChangeHub) Watch(ctx context.Context) <-chan Change {
	output := make(chan Change, 1)
	if h == nil {
		close(output)
		return output
	}
	h.mu.Lock()
	id := h.nextID
	h.nextID++
	h.subscribers[id] = output
	initial := Change{InstanceID: h.instanceID, Revision: h.revision, OccurredAt: h.now(), Resync: true}
	output <- initial
	h.mu.Unlock()

	go func() {
		<-ctx.Done()
		h.mu.Lock()
		if subscriber, ok := h.subscribers[id]; ok {
			delete(h.subscribers, id)
			close(subscriber)
		}
		h.mu.Unlock()
	}()
	return output
}

func uniqueTopics(input []ChangeTopic) []ChangeTopic {
	seen := map[ChangeTopic]struct{}{}
	out := make([]ChangeTopic, 0, len(input))
	for _, topic := range input {
		if topic == "" {
			continue
		}
		if _, exists := seen[topic]; exists {
			continue
		}
		seen[topic] = struct{}{}
		out = append(out, topic)
	}
	return out
}
