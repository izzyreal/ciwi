package server

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"time"
)

func (s *stateStore) warmProjectIconsOnStartup(ctx context.Context) {
	if s == nil || s.db == nil {
		return
	}
	projects, err := s.projectStore().ListProjects()
	if err != nil {
		slog.Error("startup icon warmup list projects failed", "error", err)
		return
	}
	for _, p := range projects {
		if !isRootRepoProject(p) {
			continue
		}
		configFile := strings.TrimSpace(p.ConfigFile)
		if configFile == "" {
			configFile = "ciwi-project.yaml"
		}

		tmpDir, err := os.MkdirTemp("", "ciwi-iconwarm-*")
		if err != nil {
			slog.Error("startup icon warmup temp dir failed", "project", p.Name, "error", err)
			continue
		}

		func() {
			defer os.RemoveAll(tmpDir)
			fetchCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
			defer cancel()

			fetchRes, err := fetchProjectConfigAndIcon(fetchCtx, tmpDir, p.RepoURL, p.RepoRef, configFile)
			if err != nil {
				slog.Warn("startup icon warmup fetch failed", "project", p.Name, "error", err)
				return
			}
			if len(fetchRes.IconContentBytes) == 0 || strings.TrimSpace(fetchRes.IconContentType) == "" {
				return
			}
			s.setProjectIcon(p.ID, fetchRes.IconContentType, fetchRes.IconContentBytes)
		}()
	}
}
