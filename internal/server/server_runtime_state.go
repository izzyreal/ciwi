package server

import (
	"net/http"
	"os/exec"
	"sort"
	"strings"
	"time"
)

const (
	agentOnlineMaxAge = 20 * time.Second
	agentStaleMaxAge  = 60 * time.Second
)

type runtimeStateResponse struct {
	Mode             string    `json:"mode"`
	Reasons          []string  `json:"reasons,omitempty"`
	OnlineAgents     int       `json:"online_agents"`
	StaleAgents      int       `json:"stale_agents"`
	OfflineAgents    int       `json:"offline_agents"`
	HasAnyAgents     bool      `json:"has_any_agents"`
	LastAgentSeenUTC time.Time `json:"last_agent_seen_utc,omitempty"`
	GitAvailable     bool      `json:"git_available"`
	ServerVersion    string    `json:"server_version,omitempty"`
}

func (s *stateStore) runtimeStateHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, s.computeRuntimeState(time.Now().UTC()))
}

func (s *stateStore) computeRuntimeState(now time.Time) runtimeStateResponse {
	s.mu.Lock()
	agents := make(map[string]agentState, len(s.agents))
	for id, a := range s.agents {
		agents[id] = a
	}
	s.mu.Unlock()

	var (
		online, stale, offline int
		lastSeen               time.Time
	)
	for _, a := range agents {
		ts := a.LastSeenUTC
		if ts.After(lastSeen) {
			lastSeen = ts
		}
		status := classifyAgentFreshness(ts, now)
		switch status {
		case "online":
			online++
		case "stale":
			stale++
		default:
			offline++
		}
	}

	reasons := make([]string, 0)
	mode := "normal"
	if len(agents) > 0 && online == 0 {
		mode = "degraded_offline"
		reasons = append(reasons, "no online agents")
	}
	if !serverGitAvailable() {
		mode = "degraded_offline"
		reasons = append(reasons, "git unavailable on server")
	}
	sort.Strings(reasons)
	return runtimeStateResponse{
		Mode:             mode,
		Reasons:          reasons,
		OnlineAgents:     online,
		StaleAgents:      stale,
		OfflineAgents:    offline,
		HasAnyAgents:     len(agents) > 0,
		LastAgentSeenUTC: lastSeen,
		GitAvailable:     serverGitAvailable(),
		ServerVersion:    currentVersion(),
	}
}

func classifyAgentFreshness(lastSeen, now time.Time) string {
	if lastSeen.IsZero() {
		return "offline"
	}
	age := now.Sub(lastSeen)
	if age <= agentOnlineMaxAge {
		return "online"
	}
	if age <= agentStaleMaxAge {
		return "stale"
	}
	return "offline"
}

func serverGitAvailable() bool {
	_, err := execLookPath("git")
	return err == nil
}

var execLookPath = func(file string) (string, error) {
	return execLookPathImpl(file)
}

var execLookPathImpl = func(file string) (string, error) {
	return exec.LookPath(strings.TrimSpace(file))
}
