package requirements

import (
	"fmt"
	"sort"
	"strings"

	"github.com/izzyreal/ciwi/internal/domain"
	"github.com/izzyreal/ciwi/internal/protocol"
	"golang.org/x/mod/semver"
)

type AgentSnapshot struct {
	ID           string
	OS           string
	Arch         string
	Capabilities map[string]string
	Freshness    string
	Authorized   bool
	Deactivated  bool
	Updating     bool
	Busy         bool
}

const (
	DiagnosisReady        = domain.SchedulingReady
	DiagnosisWaiting      = domain.SchedulingWaiting
	DiagnosisIncompatible = domain.SchedulingIncompatible
)

type MatchIssue = domain.SchedulingMatchIssue
type AgentAssessment = domain.SchedulingAgentAssessment
type SchedulingDiagnosis = domain.SchedulingDiagnosis

type MatchResult struct {
	Matches bool
	Issues  []MatchIssue
}

func DiagnoseQueuedJob(job protocol.JobExecution, agents []AgentSnapshot) *SchedulingDiagnosis {
	if reason := protocol.JobSchedulingBlockedReason(job); reason != "" {
		if protocol.IsPendingJobExecutionStatus(job.Status) {
			return &SchedulingDiagnosis{State: DiagnosisWaiting, Summary: reason}
		}
		return nil
	}
	if !protocol.IsQueuedJobExecutionStatus(job.Status) {
		return nil
	}
	if protocol.IsJobWaitingForPrerequisites(job) {
		return nil
	}
	diagnosis := DiagnoseScheduling(job.RequiredCapabilities, agents)
	return &diagnosis
}

// MatchAgent is the canonical scheduler capability matcher. All requirements
// must be satisfied by this one agent; capabilities from different agents are
// never combined.
func MatchAgent(required map[string]string, agent AgentSnapshot) MatchResult {
	return MatchCapabilities(required, mergeCapabilities(agent))
}

func MatchCapabilities(required, observed map[string]string) MatchResult {
	issues := make([]MatchIssue, 0)
	keys := make([]string, 0, len(required))
	for key := range required {
		if !strings.HasPrefix(key, "requires.container.tool.") {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	for _, key := range keys {
		expected := strings.TrimSpace(required[key])
		actualKey := key
		matched := false
		code := "capability_mismatch"
		switch {
		case strings.HasPrefix(key, "requires.tool."):
			tool := strings.TrimPrefix(key, "requires.tool.")
			actualKey = "tool." + tool
			matched = ToolConstraintMatch(strings.TrimSpace(observed[actualKey]), expected)
			code = "tool_mismatch"
		case key == "shell":
			matched = ShellCapabilityMatch(observed, expected)
			code = "shell_mismatch"
		default:
			matched = strings.TrimSpace(observed[actualKey]) == expected
		}
		if matched {
			continue
		}
		actual := strings.TrimSpace(observed[actualKey])
		issues = append(issues, MatchIssue{
			Code: code, Key: key, Expected: expected, Actual: actual,
			Message: mismatchMessage(key, expected, actual),
		})
	}
	return MatchResult{Matches: len(issues) == 0, Issues: issues}
}

func DiagnoseScheduling(required map[string]string, agents []AgentSnapshot) SchedulingDiagnosis {
	diagnosis := SchedulingDiagnosis{Requirements: RequirementLabels(required)}
	if len(agents) == 0 {
		diagnosis.State = DiagnosisIncompatible
		diagnosis.Summary = "No agents are registered"
		return diagnosis
	}

	sortedAgents := append([]AgentSnapshot(nil), agents...)
	sort.Slice(sortedAgents, func(i, j int) bool { return sortedAgents[i].ID < sortedAgents[j].ID })
	matching := 0
	available := 0
	for _, agent := range sortedAgents {
		match := MatchAgent(required, agent)
		availabilityIssues := agentAvailabilityIssues(agent)
		assessment := AgentAssessment{
			AgentID: strings.TrimSpace(agent.ID), CapabilityMatch: match.Matches,
			Available:        match.Matches && len(availabilityIssues) == 0,
			CapabilityIssues: match.Issues, AvailabilityIssues: availabilityIssues,
		}
		if match.Matches {
			matching++
			if assessment.Available {
				available++
			}
		}
		diagnosis.Agents = append(diagnosis.Agents, assessment)
	}
	sort.SliceStable(diagnosis.Agents, func(i, j int) bool {
		left, right := diagnosis.Agents[i], diagnosis.Agents[j]
		if left.CapabilityMatch != right.CapabilityMatch {
			return left.CapabilityMatch
		}
		if len(left.CapabilityIssues) != len(right.CapabilityIssues) {
			return len(left.CapabilityIssues) < len(right.CapabilityIssues)
		}
		return left.AgentID < right.AgentID
	})

	switch {
	case available > 0:
		diagnosis.State = DiagnosisReady
		diagnosis.Summary = "Eligible agent available; awaiting lease"
	case matching == 1:
		diagnosis.State = DiagnosisWaiting
		for _, agent := range diagnosis.Agents {
			if agent.CapabilityMatch {
				diagnosis.Summary = "Matching agent " + agent.AgentID + " is " + strings.Join(agent.AvailabilityIssues, ", ")
				break
			}
		}
	case matching > 1:
		diagnosis.State = DiagnosisWaiting
		diagnosis.Summary = fmt.Sprintf("%d matching agents are currently unavailable", matching)
	default:
		diagnosis.State = DiagnosisIncompatible
		diagnosis.Summary = "No agent matches all requirements"
		if len(diagnosis.Requirements) > 0 {
			diagnosis.Summary += ": " + strings.Join(diagnosis.Requirements, ", ")
		}
	}
	return diagnosis
}

func RequirementLabels(required map[string]string) []string {
	labels := make([]string, 0, len(required))
	for key, value := range required {
		value = strings.TrimSpace(value)
		switch {
		case strings.HasPrefix(key, "requires.container.tool."):
			continue
		case strings.HasPrefix(key, "requires.tool."):
			tool := strings.TrimPrefix(key, "requires.tool.")
			if value == "" || value == "*" {
				labels = append(labels, tool)
			} else {
				labels = append(labels, tool+" "+value)
			}
		case key == "shell":
			labels = append(labels, value+" shell")
		case key == "agent_id":
			labels = append(labels, "agent "+value)
		case key == "os", key == "arch":
			labels = append(labels, value)
		default:
			labels = append(labels, key+"="+value)
		}
	}
	sort.Strings(labels)
	return labels
}

func agentAvailabilityIssues(agent AgentSnapshot) []string {
	issues := make([]string, 0, 5)
	switch strings.ToLower(strings.TrimSpace(agent.Freshness)) {
	case "online":
	case "stale":
		issues = append(issues, "stale")
	default:
		issues = append(issues, "offline")
	}
	if !agent.Authorized {
		issues = append(issues, "unauthorized")
	}
	if agent.Deactivated {
		issues = append(issues, "deactivated")
	}
	if agent.Updating {
		issues = append(issues, "updating")
	}
	if agent.Busy {
		issues = append(issues, "busy")
	}
	return issues
}

func mismatchMessage(key, expected, actual string) string {
	actualLabel := actual
	if actualLabel == "" {
		actualLabel = "missing"
	}
	if strings.HasPrefix(key, "requires.tool.") {
		return fmt.Sprintf("tool %s expected %s, got %s", strings.TrimPrefix(key, "requires.tool."), expected, actualLabel)
	}
	return fmt.Sprintf("%s expected %s, got %s", key, expected, actualLabel)
}

func ShellCapabilityMatch(agentCapabilities map[string]string, requiredValue string) bool {
	required := strings.ToLower(strings.TrimSpace(requiredValue))
	if required == "" {
		return true
	}
	for _, shell := range strings.Split(agentCapabilities["shells"], ",") {
		if strings.EqualFold(strings.TrimSpace(shell), required) {
			return true
		}
	}
	return false
}

func ToolConstraintMatch(agentValue, constraint string) bool {
	agentValue = strings.TrimSpace(agentValue)
	constraint = strings.TrimSpace(constraint)
	if agentValue == "" {
		return false
	}
	if constraint == "" || constraint == "*" {
		return true
	}

	op := ""
	value := constraint
	for _, candidate := range []string{">=", "<=", ">", "<", "==", "="} {
		if strings.HasPrefix(constraint, candidate) {
			op = candidate
			value = strings.TrimSpace(strings.TrimPrefix(constraint, candidate))
			break
		}
	}
	if value == "" {
		return true
	}
	if op == "" {
		return agentValue == value
	}

	agentSemver, agentOK := normalizeSemver(agentValue)
	constraintSemver, constraintOK := normalizeSemver(value)
	if !agentOK || !constraintOK {
		switch op {
		case "=", "==":
			return agentValue == value
		default:
			return false
		}
	}

	cmp := semver.Compare(agentSemver, constraintSemver)
	switch op {
	case ">":
		return cmp > 0
	case ">=":
		return cmp >= 0
	case "<":
		return cmp < 0
	case "<=":
		return cmp <= 0
	case "=", "==":
		return cmp == 0
	default:
		return false
	}
}

func mergeCapabilities(agent AgentSnapshot) map[string]string {
	merged := map[string]string{}
	for key, value := range agent.Capabilities {
		merged[key] = value
	}
	merged["agent_id"] = strings.TrimSpace(agent.ID)
	merged["os"] = strings.TrimSpace(agent.OS)
	merged["arch"] = strings.TrimSpace(agent.Arch)
	return merged
}

func normalizeSemver(v string) (string, bool) {
	v = strings.TrimSpace(v)
	if v == "" {
		return "", false
	}
	if !strings.HasPrefix(v, "v") {
		v = "v" + v
	}
	if !semver.IsValid(v) {
		return "", false
	}
	return v, true
}
