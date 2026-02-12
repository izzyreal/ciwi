package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/izzyreal/ciwi/internal/store"
)

func (s *stateStore) streamVersionResolve(w http.ResponseWriter, p store.PersistedPipeline) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	send := func(step, status, message string, extra map[string]any) {
		payload := map[string]any{
			"step":    step,
			"status":  status,
			"message": message,
		}
		for k, v := range extra {
			payload[k] = v
		}
		b, _ := json.Marshal(payload)
		_, _ = fmt.Fprintf(w, "data: %s\n\n", string(b))
		flusher.Flush()
	}

	send("start", "running", fmt.Sprintf("resolving version for pipeline %q", p.PipelineID), nil)
	depCtx, depErr := s.checkPipelineDependenciesWithReporter(p, func(step, status, message string) {
		send(step, status, message, nil)
	})
	if depErr != nil {
		send("done", "error", depErr.Error(), nil)
		return
	}
	runCtx, runErr := resolvePipelineRunContextWithReporter(p, depCtx, func(step, status, message string) {
		send(step, status, message, nil)
	})
	if runErr != nil {
		send("done", "error", runErr.Error(), nil)
		return
	}
	send("done", "ok", "version resolution completed", map[string]any{
		"pipeline_version":     strings.TrimSpace(runCtx.Version),
		"pipeline_version_raw": strings.TrimSpace(runCtx.VersionRaw),
		"source_ref_resolved":  strings.TrimSpace(runCtx.SourceRefResolved),
		"version_file":         strings.TrimSpace(runCtx.VersionFile),
		"tag_prefix":           strings.TrimSpace(runCtx.TagPrefix),
		"auto_bump":            strings.TrimSpace(runCtx.AutoBump),
	})
}
