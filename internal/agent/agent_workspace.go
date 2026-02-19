package agent

import (
	"crypto/sha1"
	"encoding/hex"
	"path/filepath"
	"sort"
	"strings"

	"github.com/izzyreal/ciwi/internal/protocol"
)

func workspaceDirForJob(workDir string, job protocol.JobExecution) string {
	meta := job.Metadata
	projectID := strings.TrimSpace(meta["project_id"])
	if projectID == "" {
		projectID = "project-unknown"
	}
	projectName := strings.TrimSpace(meta["project"])
	if projectName == "" {
		projectName = "project-unknown"
	}
	pipelineJobID := strings.TrimSpace(meta["pipeline_job_id"])
	if pipelineJobID == "" {
		pipelineJobID = strings.TrimSpace(job.ID)
	}
	if pipelineJobID == "" {
		pipelineJobID = "job-unknown"
	}
	matrixIdentity := workspaceMatrixIdentity(meta)
	envFingerprint := workspaceEnvFingerprint(job.RequiredCapabilities)

	parts := []string{
		sanitizeWorkspacePathPart(projectID),
		sanitizeWorkspacePathPart(projectName),
		sanitizeWorkspacePathPart(pipelineJobID),
	}
	if strings.TrimSpace(matrixIdentity) != "" {
		parts = append(parts, sanitizeWorkspacePathPart(matrixIdentity))
	}
	prefix := strings.Join(parts, "_")
	if len(prefix) > 120 {
		prefix = prefix[:120]
	}
	name := prefix + "_" + envFingerprint
	return filepath.Join(workDir, "workspaces", name)
}

func workspaceMatrixIdentity(meta map[string]string) string {
	if name := strings.TrimSpace(meta["matrix_name"]); name != "" {
		return name
	}
	if idx := strings.TrimSpace(meta["matrix_index"]); idx != "" {
		return "idx-" + idx
	}
	return ""
}

func workspaceEnvFingerprint(required map[string]string) string {
	if len(required) == 0 {
		return "env-default"
	}
	keys := []string{"arch", "executor", "os", "shell"}
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		val := strings.TrimSpace(required[key])
		if val == "" {
			continue
		}
		parts = append(parts, key+"="+val)
	}
	// If none of the canonical keys are present, fall back to full deterministic map
	// so two incompatible capability sets don't collapse into the same workspace key.
	if len(parts) == 0 {
		allKeys := make([]string, 0, len(required))
		for key := range required {
			allKeys = append(allKeys, key)
		}
		sort.Strings(allKeys)
		for _, key := range allKeys {
			val := strings.TrimSpace(required[key])
			if val == "" {
				continue
			}
			parts = append(parts, key+"="+val)
		}
	}
	if len(parts) == 0 {
		return "env-default"
	}
	sum := sha1.Sum([]byte(strings.Join(parts, "|")))
	return "env-" + hex.EncodeToString(sum[:8])
}

func sanitizeWorkspacePathPart(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return "x"
	}
	var b strings.Builder
	b.Grow(len(v))
	lastUnderscore := false
	for _, r := range v {
		isAllowed := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' || r == '-' || r == '_'
		if isAllowed {
			b.WriteRune(r)
			lastUnderscore = false
			continue
		}
		if !lastUnderscore {
			b.WriteByte('_')
			lastUnderscore = true
		}
	}
	out := strings.Trim(b.String(), "._-")
	if out == "" {
		return "x"
	}
	return out
}
