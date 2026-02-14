package server

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

	"github.com/izzyreal/ciwi/internal/protocol"
)

const updateReloadProjectsPendingKey = "update_reload_projects_pending"

func (s *stateStore) maybeRunPostUpdateProjectReload(ctx context.Context) {
	if s == nil || s.db == nil {
		return
	}
	v, ok, err := s.db.GetAppState(updateReloadProjectsPendingKey)
	if err != nil {
		slog.Error("read post-update project reload flag failed", "error", err)
		return
	}
	if !ok || strings.TrimSpace(v) != "1" {
		return
	}
	go s.runPostUpdateProjectReload(ctx)
}

func (s *stateStore) runPostUpdateProjectReload(ctx context.Context) {
	projects, err := s.projectStore().ListProjects()
	if err != nil {
		slog.Error("post-update project reload list failed", "error", err)
		return
	}
	var candidates []protocol.ProjectSummary
	for _, p := range projects {
		if !isRootRepoProject(p) {
			continue
		}
		candidates = append(candidates, p)
	}
	if len(candidates) == 0 {
		_ = s.updateStateStore().SetAppState(updateReloadProjectsPendingKey, "0")
		return
	}
	var failures []string
	for _, p := range candidates {
		if err := s.reloadProjectFromRepo(ctx, p); err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", p.Name, err))
			slog.Error("post-update project reload failed", "project", p.Name, "error", err)
			continue
		}
		slog.Info("post-update project reloaded", "project", p.Name)
	}
	if len(failures) == 0 {
		_ = s.updateStateStore().SetAppState(updateReloadProjectsPendingKey, "0")
		_ = s.persistUpdateStatus(map[string]string{
			"update_message": "post-update project reload complete",
		})
		return
	}
	_ = s.persistUpdateStatus(map[string]string{
		"update_message": "post-update project reload incomplete; retry on next restart",
	})
}

func isRootRepoProject(p protocol.ProjectSummary) bool {
	if strings.TrimSpace(p.RepoURL) == "" {
		return false
	}
	configFile := strings.TrimSpace(p.ConfigFile)
	if configFile == "" {
		configFile = "ciwi-project.yaml"
	}
	base := filepath.Base(configFile)
	return base == configFile && base != "." && base != ""
}
