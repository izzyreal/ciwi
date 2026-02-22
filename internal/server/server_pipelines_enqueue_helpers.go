package server

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/izzyreal/ciwi/internal/config"
	"github.com/izzyreal/ciwi/internal/protocol"
)

func normalizePipelineJobNeeds(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, need := range in {
		need = strings.TrimSpace(need)
		if need == "" {
			continue
		}
		if _, exists := seen[need]; exists {
			continue
		}
		seen[need] = struct{}{}
		out = append(out, need)
	}
	return out
}

func cloneProtocolJobCaches(in []protocol.JobCacheSpec) []protocol.JobCacheSpec {
	if len(in) == 0 {
		return nil
	}
	out := make([]protocol.JobCacheSpec, 0, len(in))
	for _, c := range in {
		out = append(out, protocol.JobCacheSpec{
			ID:  c.ID,
			Env: c.Env,
		})
	}
	return out
}

func resolveDependencyArtifactJobID(dependsOn []string, depArtifactJobIDs map[string]string, jobID string, vars map[string]string) string {
	if len(dependsOn) == 0 || len(depArtifactJobIDs) == 0 {
		return ""
	}
	candidates := []string{
		strings.TrimSpace(vars["name"]),
		strings.TrimSpace(vars["build_target"]),
		strings.TrimSpace(jobID),
	}
	if strings.HasPrefix(jobID, "release-") {
		candidates = append(candidates, strings.TrimSpace(strings.TrimPrefix(jobID, "release-")))
	}
	for _, depID := range dependsOn {
		depID = strings.TrimSpace(depID)
		if depID == "" {
			continue
		}
		for _, c := range candidates {
			c = strings.TrimSpace(c)
			if c == "" {
				continue
			}
			if v := strings.TrimSpace(depArtifactJobIDs[depID+":"+c]); v != "" {
				return v
			}
		}
	}
	return ""
}

func resolveDependencyArtifactJobIDs(dependsOn []string, depArtifactJobIDsAll map[string][]string, preferred string) []string {
	if len(dependsOn) == 0 || len(depArtifactJobIDsAll) == 0 {
		if strings.TrimSpace(preferred) == "" {
			return nil
		}
		return []string{strings.TrimSpace(preferred)}
	}
	out := make([]string, 0)
	seen := map[string]struct{}{}
	if p := strings.TrimSpace(preferred); p != "" {
		out = append(out, p)
		seen[p] = struct{}{}
	}
	for _, depID := range dependsOn {
		depID = strings.TrimSpace(depID)
		if depID == "" {
			continue
		}
		for _, id := range depArtifactJobIDsAll[depID] {
			id = strings.TrimSpace(id)
			if id == "" {
				continue
			}
			if _, ok := seen[id]; ok {
				continue
			}
			out = append(out, id)
			seen[id] = struct{}{}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func cloneJobStepPlan(in []protocol.JobStepPlanItem) []protocol.JobStepPlanItem {
	if len(in) == 0 {
		return nil
	}
	out := make([]protocol.JobStepPlanItem, 0, len(in))
	for _, step := range in {
		var secrets []protocol.ProjectSecretSpec
		if len(step.VaultSecrets) > 0 {
			secrets = append([]protocol.ProjectSecretSpec(nil), step.VaultSecrets...)
		}
		out = append(out, protocol.JobStepPlanItem{
			Index:           step.Index,
			Total:           step.Total,
			Name:            step.Name,
			Script:          step.Script,
			Kind:            step.Kind,
			Env:             cloneMap(step.Env),
			VaultConnection: step.VaultConnection,
			VaultSecrets:    secrets,
			TestName:        step.TestName,
			TestFormat:      step.TestFormat,
			TestReport:      step.TestReport,
			CoverageFormat:  step.CoverageFormat,
			CoverageReport:  step.CoverageReport,
		})
	}
	return out
}

func buildAutoBumpNextVersion(versionRaw, mode string) string {
	parts := strings.Split(strings.TrimSpace(versionRaw), ".")
	if len(parts) != 3 {
		return ""
	}
	major, err1 := strconv.Atoi(parts[0])
	minor, err2 := strconv.Atoi(parts[1])
	patch, err3 := strconv.Atoi(parts[2])
	if err1 != nil || err2 != nil || err3 != nil {
		return ""
	}
	switch strings.TrimSpace(mode) {
	case "patch":
		patch++
	case "minor":
		minor++
		patch = 0
	case "major":
		major++
		minor = 0
		patch = 0
	default:
		return ""
	}
	return fmt.Sprintf("%d.%d.%d", major, minor, patch)
}

func deriveAutoBumpBranch(sourceRef string) string {
	ref := strings.TrimSpace(sourceRef)
	if ref == "" {
		return ""
	}
	switch {
	case strings.HasPrefix(ref, "refs/heads/"):
		return strings.TrimSpace(strings.TrimPrefix(ref, "refs/heads/"))
	case strings.HasPrefix(ref, "refs/"):
		return ""
	}
	if len(ref) >= 7 && len(ref) <= 40 {
		isHex := true
		for _, r := range ref {
			if (r < '0' || r > '9') && (r < 'a' || r > 'f') && (r < 'A' || r > 'F') {
				isHex = false
				break
			}
		}
		if isHex {
			return ""
		}
	}
	return ref
}

func describePipelineStep(step config.PipelineJobStep, idx int, jobID string) string {
	if step.Test != nil {
		name := strings.TrimSpace(step.Test.Name)
		if name == "" {
			name = fmt.Sprintf("%s-test-%d", jobID, idx+1)
		}
		return "test " + name
	}
	return fmt.Sprintf("step %d", idx+1)
}

func describeSkippedPipelineStepLiteral(step config.PipelineJobStep, idx int, jobID string) string {
	if strings.TrimSpace(step.Run) != "" {
		return strings.TrimSpace(step.Run)
	}
	if step.Test != nil {
		command := strings.TrimSpace(step.Test.Command)
		if command != "" {
			return command
		}
	}
	return describePipelineStep(step, idx, jobID)
}

func cloneJobCachesFromPersisted(in []config.PipelineJobCacheSpec) []protocol.JobCacheSpec {
	if len(in) == 0 {
		return nil
	}
	out := make([]protocol.JobCacheSpec, 0, len(in))
	for _, c := range in {
		out = append(out, protocol.JobCacheSpec{
			ID:  c.ID,
			Env: c.Env,
		})
	}
	return out
}
