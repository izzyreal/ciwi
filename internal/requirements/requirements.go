package requirements

import (
	"fmt"
	"strings"

	"golang.org/x/mod/semver"
)

type AgentSnapshot struct {
	ID           string
	OS           string
	Arch         string
	Capabilities map[string]string
}

func DiagnoseUnmetRequirements(required map[string]string, agents []AgentSnapshot) []string {
	if len(required) == 0 {
		return nil
	}
	if len(agents) == 0 {
		return []string{"no agents connected"}
	}

	reasons := []string{}
	for key, value := range required {
		if strings.HasPrefix(key, "requires.tool.") {
			tool := strings.TrimPrefix(key, "requires.tool.")
			constraint := strings.TrimSpace(value)
			seenTool := false
			satisfied := false
			for _, agent := range agents {
				agentValue := strings.TrimSpace(agent.Capabilities["tool."+tool])
				if agentValue != "" {
					seenTool = true
				}
				if ToolConstraintMatch(agentValue, constraint) {
					satisfied = true
					break
				}
			}
			if !satisfied {
				if !seenTool {
					reasons = append(reasons, "missing tool "+tool)
				} else if constraint == "" || constraint == "*" {
					reasons = append(reasons, "tool "+tool+" unavailable")
				} else {
					reasons = append(reasons, "tool "+tool+" does not satisfy "+constraint)
				}
			}
			continue
		}

		if key == "shell" {
			requiredShell := strings.TrimSpace(value)
			ok := false
			for _, agent := range agents {
				if ShellCapabilityMatch(mergeCapabilities(agent), requiredShell) {
					ok = true
					break
				}
			}
			if !ok {
				reasons = append(reasons, fmt.Sprintf("no agent with %s=%s", key, requiredShell))
			}
			continue
		}

		if key == "agent_id" {
			requiredAgentID := strings.TrimSpace(value)
			ok := false
			for _, agent := range agents {
				if strings.TrimSpace(agent.ID) == requiredAgentID {
					ok = true
					break
				}
			}
			if !ok {
				reasons = append(reasons, fmt.Sprintf("no agent with %s=%s", key, requiredAgentID))
			}
			continue
		}

		requiredValue := strings.TrimSpace(value)
		ok := false
		for _, agent := range agents {
			caps := mergeCapabilities(agent)
			if strings.TrimSpace(caps[key]) == requiredValue {
				ok = true
				break
			}
		}
		if !ok {
			reasons = append(reasons, fmt.Sprintf("no agent with %s=%s", key, requiredValue))
		}
	}

	return reasons
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
	merged := map[string]string{
		"os":   strings.TrimSpace(agent.OS),
		"arch": strings.TrimSpace(agent.Arch),
	}
	for key, value := range agent.Capabilities {
		merged[key] = value
	}
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
