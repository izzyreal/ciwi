package server

import (
	"strings"
	"sync"
	"time"
)

// agentRegistry owns all concurrent, process-local agent state. Persistence,
// job cancellation, and change publication are orchestrated by adapters around
// it; the server composition root does not own individual agent maps.
type agentRegistry struct {
	mu                sync.Mutex
	agents            map[string]agentState
	agentUpdates      map[string]string
	agentToolRefresh  map[string]bool
	agentRestarts     map[string]bool
	agentCacheWipes   map[string]bool
	agentHistoryWipes map[string]bool
	agentDeactivated  map[string]bool
	agentRollout      agentUpdateRolloutState
}

func newAgentRegistry() agentRegistry {
	return agentRegistry{
		agents:            make(map[string]agentState),
		agentUpdates:      make(map[string]string),
		agentToolRefresh:  make(map[string]bool),
		agentRestarts:     make(map[string]bool),
		agentCacheWipes:   make(map[string]bool),
		agentHistoryWipes: make(map[string]bool),
		agentDeactivated:  make(map[string]bool),
		agentRollout: agentUpdateRolloutState{
			Slots: make(map[string]int),
		},
	}
}

func (r *agentRegistry) ensureInitializedLocked() {
	if r.agents == nil {
		r.agents = make(map[string]agentState)
	}
	if r.agentUpdates == nil {
		r.agentUpdates = make(map[string]string)
	}
	if r.agentToolRefresh == nil {
		r.agentToolRefresh = make(map[string]bool)
	}
	if r.agentRestarts == nil {
		r.agentRestarts = make(map[string]bool)
	}
	if r.agentCacheWipes == nil {
		r.agentCacheWipes = make(map[string]bool)
	}
	if r.agentHistoryWipes == nil {
		r.agentHistoryWipes = make(map[string]bool)
	}
	if r.agentDeactivated == nil {
		r.agentDeactivated = make(map[string]bool)
	}
	if r.agentRollout.Slots == nil {
		r.agentRollout.Slots = make(map[string]int)
	}
}

type agentRegistrySnapshot struct {
	ID            string
	State         agentState
	PendingUpdate string
}

func (r *agentRegistry) snapshots() []agentRegistrySnapshot {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ensureInitializedLocked()
	result := make([]agentRegistrySnapshot, 0, len(r.agents))
	for id, state := range r.agents {
		result = append(result, agentRegistrySnapshot{
			ID: id, State: cloneAgentState(state), PendingUpdate: strings.TrimSpace(r.agentUpdates[id]),
		})
	}
	return result
}

func (r *agentRegistry) snapshot(agentID string) (agentRegistrySnapshot, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ensureInitializedLocked()
	state, ok := r.agents[agentID]
	if !ok {
		return agentRegistrySnapshot{}, false
	}
	return agentRegistrySnapshot{
		ID: agentID, State: cloneAgentState(state), PendingUpdate: strings.TrimSpace(r.agentUpdates[agentID]),
	}, true
}

func (r *agentRegistry) stateMap() map[string]agentState {
	snapshots := r.snapshots()
	result := make(map[string]agentState, len(snapshots))
	for _, snapshot := range snapshots {
		result[snapshot.ID] = snapshot.State
	}
	return result
}

func (r *agentRegistry) markSeen(agentID string, seen time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	state, ok := r.agents[agentID]
	if !ok {
		return
	}
	state.LastSeenUTC = seen
	r.agents[agentID] = state
}

func cloneAgentState(state agentState) agentState {
	state.Capabilities = cloneMap(state.Capabilities)
	state.RecentLog = append([]string(nil), state.RecentLog...)
	return state
}

type agentState struct {
	Hostname             string            `json:"hostname"`
	OS                   string            `json:"os"`
	Arch                 string            `json:"arch"`
	Version              string            `json:"version,omitempty"`
	Authorized           bool              `json:"authorized"`
	Deactivated          bool              `json:"deactivated,omitempty"`
	Capabilities         map[string]string `json:"capabilities"`
	LastSeenUTC          time.Time         `json:"last_seen_utc"`
	RecentLog            []string          `json:"recent_log,omitempty"`
	UpdateTarget         string            `json:"update_target,omitempty"`
	UpdateSource         string            `json:"update_source,omitempty"`
	UpdateAttempts       int               `json:"update_attempts,omitempty"`
	UpdateInProgress     bool              `json:"update_in_progress,omitempty"`
	UpdateLastRequestUTC time.Time         `json:"update_last_request_utc,omitempty"`
	UpdateNextRetryUTC   time.Time         `json:"update_next_retry_utc,omitempty"`
	UpdateLastError      string            `json:"update_last_error,omitempty"`
	UpdateLastErrorUTC   time.Time         `json:"update_last_error_utc,omitempty"`
}

type agentUpdateRolloutState struct {
	Target     string
	StartedUTC time.Time
	NextSlot   int
	Slots      map[string]int
}
