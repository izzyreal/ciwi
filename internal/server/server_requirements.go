package server

import (
	"fmt"
	"strings"

	"golang.org/x/mod/semver"
)

func diagnoseUnmetRequirements(required map[string]string, agents map[string]agentState) []string {
	if len(required) == 0 {
		return nil
	}
	if len(agents) == 0 {
		return []string{"no agents connected"}
	}
	reasons := []string{}
	for k, v := range required {
		if strings.HasPrefix(k, "requires.tool.") {
			tool := strings.TrimPrefix(k, "requires.tool.")
			constraint := strings.TrimSpace(v)
			seenTool := false
			satisfied := false
			for _, a := range agents {
				av := strings.TrimSpace(a.Capabilities["tool."+tool])
				if av != "" {
					seenTool = true
				}
				if toolConstraintMatch(av, constraint) {
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
		if k == "shell" {
			need := strings.TrimSpace(v)
			ok := false
			for _, a := range agents {
				caps := mergeCapabilities(a, nil)
				if shellCapabilityMatch(caps, need) {
					ok = true
					break
				}
			}
			if !ok {
				reasons = append(reasons, fmt.Sprintf("no agent with %s=%s", k, need))
			}
			continue
		}
		need := strings.TrimSpace(v)
		ok := false
		for _, a := range agents {
			caps := mergeCapabilities(a, nil)
			if strings.TrimSpace(caps[k]) == need {
				ok = true
				break
			}
		}
		if !ok {
			reasons = append(reasons, fmt.Sprintf("no agent with %s=%s", k, need))
		}
	}
	return reasons
}

func shellCapabilityMatch(agentCapabilities map[string]string, requiredValue string) bool {
	required := strings.ToLower(strings.TrimSpace(requiredValue))
	if required == "" {
		return true
	}
	for _, s := range strings.Split(agentCapabilities["shells"], ",") {
		if strings.EqualFold(strings.TrimSpace(s), required) {
			return true
		}
	}
	return false
}

func toolConstraintMatch(agentValue, constraint string) bool {
	agentValue = strings.TrimSpace(agentValue)
	constraint = strings.TrimSpace(constraint)
	if agentValue == "" {
		return false
	}
	if constraint == "" || constraint == "*" {
		return true
	}
	op := ""
	val := constraint
	for _, candidate := range []string{">=", "<=", ">", "<", "==", "="} {
		if strings.HasPrefix(constraint, candidate) {
			op = candidate
			val = strings.TrimSpace(strings.TrimPrefix(constraint, candidate))
			break
		}
	}
	if val == "" {
		return true
	}
	if op == "" {
		return agentValue == val
	}
	av, aok := normalizeSemver(agentValue)
	vv, vok := normalizeSemver(val)
	if !aok || !vok {
		switch op {
		case "=", "==":
			return agentValue == val
		default:
			return false
		}
	}
	cmp := semver.Compare(av, vv)
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
