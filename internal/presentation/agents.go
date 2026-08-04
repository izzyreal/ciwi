package presentation

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/izzyreal/ciwi/internal/application"
	"github.com/izzyreal/ciwi/internal/domain"
)

type AgentView struct {
	ID                string `json:"id"`
	Hostname          string `json:"hostname"`
	Platform          string `json:"platform"`
	Version           string `json:"version"`
	Status            string `json:"status"`
	StatusLabel       string `json:"status_label"`
	Authorization     string `json:"authorization"`
	Activation        string `json:"activation"`
	Authorized        bool   `json:"authorized"`
	Deactivated       bool   `json:"deactivated"`
	JobInProgress     bool   `json:"job_in_progress"`
	CapabilitiesLabel string `json:"capabilities_label"`
	RunMode           string `json:"run_mode"`
	LastSeen          string `json:"last_seen"`
	RecentLog         string `json:"recent_log"`
	UpdateLabel       string `json:"update_label"`
	CanUpdate         bool   `json:"can_update"`
	CanContact        bool   `json:"can_contact"`
}

type AgentsView struct {
	Summary string      `json:"summary"`
	Agents  []AgentView `json:"agents"`
}

type AgentDetailsView struct {
	Agent AgentView `json:"agent"`
}

type AgentsQueries struct {
	agents *application.AgentQueries
	now    func() time.Time
}

func NewAgentsQueries(agents *application.AgentQueries) *AgentsQueries {
	return &AgentsQueries{agents: agents, now: func() time.Time { return time.Now().UTC() }}
}

func (q *AgentsQueries) GetAgentsView(ctx context.Context) (AgentsView, error) {
	agents, err := q.agents.ListAgents(ctx)
	if err != nil {
		return AgentsView{}, err
	}
	sort.Slice(agents, func(i, j int) bool { return agents[i].ID < agents[j].ID })
	now := q.now()
	views := make([]AgentView, 0, len(agents))
	online := 0
	for _, agent := range agents {
		view := agentToView(agent, now)
		if view.Status == "online" {
			online++
		}
		views = append(views, view)
	}
	return AgentsView{Summary: agentSummary(online, len(views)), Agents: views}, nil
}

func (q *AgentsQueries) GetAgentDetailsView(ctx context.Context, agentID string) (AgentDetailsView, error) {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return AgentDetailsView{}, application.NewError(application.ErrorInvalidArgument, "agent id is required", nil)
	}
	agents, err := q.agents.ListAgents(ctx)
	if err != nil {
		return AgentDetailsView{}, err
	}
	for _, agent := range agents {
		if agent.ID == agentID {
			return AgentDetailsView{Agent: agentToView(agent, q.now())}, nil
		}
	}
	return AgentDetailsView{}, application.NewError(application.ErrorNotFound, "agent not found", nil)
}

func agentToView(agent domain.Agent, now time.Time) AgentView {
	status := agentStatus(agent.LastSeenUTC, now)
	capabilities := make([]string, 0, len(agent.Capabilities))
	for key, value := range agent.Capabilities {
		capabilities = append(capabilities, key+"="+value)
	}
	sort.Strings(capabilities)
	updateLabel := "No pending update"
	if agent.UpdateRequested {
		updateLabel = "Update requested → " + agent.UpdateTarget
		if agent.JobInProgress {
			updateLabel += " (agent busy)"
		} else if agent.UpdateInProgress {
			updateLabel = "Update → " + agent.UpdateTarget + " in progress"
		} else if !agent.UpdateNextRetry.IsZero() && agent.UpdateAttempts > 0 {
			updateLabel = "Update retry pending"
			if agent.UpdateLastError != "" {
				updateLabel += ": " + agent.UpdateLastError
			}
		}
	}
	runMode := "Manual"
	if strings.EqualFold(strings.TrimSpace(agent.Capabilities["run_mode"]), "service") {
		runMode = "Service"
	}
	return AgentView{
		ID: agent.ID, Hostname: agent.Hostname, Platform: strings.Trim(strings.TrimSpace(agent.OS)+"/"+strings.TrimSpace(agent.Arch), "/"),
		Version: agent.Version, Status: status, StatusLabel: strings.ToUpper(status[:1]) + status[1:],
		Authorization: boolLabel(agent.Authorized, "Authorized", "Unauthorized"), Activation: boolLabel(!agent.Deactivated, "Active", "Deactivated"),
		Authorized: agent.Authorized, Deactivated: agent.Deactivated, JobInProgress: agent.JobInProgress,
		CapabilitiesLabel: strings.Join(capabilities, ", "), RunMode: runMode, LastSeen: formatAgentTime(agent.LastSeenUTC),
		RecentLog: strings.Join(agent.RecentLog, "\n"), UpdateLabel: updateLabel,
		CanUpdate:  !agent.UpdateInProgress && (agent.UpdateRequested || agent.NeedsUpdate) && status != "offline",
		CanContact: status != "offline",
	}
}

func agentStatus(lastSeen, now time.Time) string {
	if lastSeen.IsZero() {
		return "offline"
	}
	age := now.Sub(lastSeen)
	if age <= 20*time.Second {
		return "online"
	}
	if age <= time.Minute {
		return "stale"
	}
	return "offline"
}

func boolLabel(value bool, yes, no string) string {
	if value {
		return yes
	}
	return no
}

func formatAgentTime(value time.Time) string {
	if value.IsZero() {
		return "Never"
	}
	return value.Local().Format("Mon 02 Jan, 15:04:05")
}

func agentSummary(online, total int) string {
	if total == 1 {
		return boolLabel(online == 1, "1/1 online", "0/1 online")
	}
	return strconv.Itoa(online) + "/" + strconv.Itoa(total) + " online"
}
