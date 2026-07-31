package pipelinechain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
)

const idPrefix = "chain-"

func NormalizePipelines(pipelines []string) []string {
	out := make([]string, len(pipelines))
	for i, pipeline := range pipelines {
		out[i] = strings.TrimSpace(pipeline)
	}
	return out
}

func ID(pipelines []string) string {
	canonical, _ := json.Marshal(NormalizePipelines(pipelines))
	sum := sha256.Sum256(canonical)
	return idPrefix + hex.EncodeToString(sum[:])
}

func DisplayName(name string, pipelines []string) string {
	if name = strings.TrimSpace(name); name != "" {
		return name
	}
	return strings.Join(NormalizePipelines(pipelines), " → ")
}
