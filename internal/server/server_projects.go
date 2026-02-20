package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/izzyreal/ciwi/internal/config"
	"github.com/izzyreal/ciwi/internal/protocol"
	serverproject "github.com/izzyreal/ciwi/internal/server/project"
)

var fetchProjectConfigAndIcon = serverproject.FetchConfigAndIconFromRepo

func (s *stateStore) importProjectHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req protocol.ImportProjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.RepoURL) == "" {
		http.Error(w, "repo_url is required", http.StatusBadRequest)
		return
	}
	if req.ConfigFile == "" {
		req.ConfigFile = "ciwi-project.yaml"
	}
	configFile := filepath.Clean(req.ConfigFile)
	if configFile == "." || configFile == "" || filepath.Base(configFile) != configFile {
		http.Error(w, "config_file must point to a root-level file", http.StatusBadRequest)
		return
	}
	req.ConfigFile = configFile
	if _, err := exec.LookPath("git"); err != nil {
		http.Error(w, "git not found on server", http.StatusInternalServerError)
		return
	}

	tmpDir, err := os.MkdirTemp("", "ciwi-import-*")
	if err != nil {
		http.Error(w, fmt.Sprintf("create temp dir: %v", err), http.StatusInternalServerError)
		return
	}
	defer os.RemoveAll(tmpDir)

	importCtx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()

	fetchRes, err := fetchProjectConfigAndIcon(importCtx, tmpDir, req.RepoURL, req.RepoRef, req.ConfigFile)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	resp, err := s.persistImportedProject(req, fetchRes.ConfigContent, fetchRes.SourceCommit, fetchRes.IconContentType, fetchRes.IconContentBytes)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusCreated, resp)
}

func (s *stateStore) projectByIDHandler(w http.ResponseWriter, r *http.Request) {
	rel := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/projects/"), "/")
	if rel == "" {
		http.NotFound(w, r)
		return
	}
	parts := strings.Split(rel, "/")
	projectID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || projectID <= 0 {
		http.Error(w, "invalid project id", http.StatusBadRequest)
		return
	}

	if len(parts) == 1 {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		detail, err := s.projectStore().GetProjectDetail(projectID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, projectDetailViewResponse{Project: detail})
		return
	}

	if len(parts) != 2 {
		http.NotFound(w, r)
		return
	}
	switch parts[1] {
	case "icon":
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		icon, ok := s.getProjectIcon(projectID)
		if !ok {
			http.NotFound(w, r)
			return
		}
		sum := sha256.Sum256(icon.Data)
		etag := `"` + hex.EncodeToString(sum[:]) + `"`
		if matchesETag(r.Header.Get("If-None-Match"), etag) {
			w.Header().Set("ETag", etag)
			w.Header().Set("Cache-Control", "public, max-age=300")
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("Content-Type", icon.ContentType)
		w.Header().Set("ETag", etag)
		w.Header().Set("Cache-Control", "public, max-age=300")
		_, _ = w.Write(icon.Data)
		return
	case "reload":
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		project, err := s.projectStore().GetProjectByID(projectID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		if strings.TrimSpace(project.RepoURL) == "" {
			http.Error(w, "project has no repo_url configured", http.StatusBadRequest)
			return
		}
		configFile := project.ConfigFile
		if configFile == "" {
			configFile = "ciwi-project.yaml"
		}

		tmpDir, err := os.MkdirTemp("", "ciwi-reload-*")
		if err != nil {
			http.Error(w, fmt.Sprintf("create temp dir: %v", err), http.StatusInternalServerError)
			return
		}
		defer os.RemoveAll(tmpDir)

		reloadCtx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
		defer cancel()

		fetchRes, err := fetchProjectConfigAndIcon(reloadCtx, tmpDir, project.RepoURL, project.RepoRef, configFile)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		resp, err := s.persistImportedProject(protocol.ImportProjectRequest{
			RepoURL:    project.RepoURL,
			RepoRef:    project.RepoRef,
			ConfigFile: configFile,
		}, fetchRes.ConfigContent, fetchRes.SourceCommit, fetchRes.IconContentType, fetchRes.IconContentBytes)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		writeJSON(w, http.StatusOK, resp)
	case "vault":
		s.projectVaultHandler(w, r, projectID)
	case "vault-test":
		s.projectVaultTestHandler(w, r, projectID)
	default:
		http.NotFound(w, r)
	}
}

func matchesETag(ifNoneMatch, target string) bool {
	target = strings.TrimSpace(target)
	if target == "" {
		return false
	}
	raw := strings.TrimSpace(ifNoneMatch)
	if raw == "" {
		return false
	}
	if raw == "*" {
		return true
	}
	for _, part := range strings.Split(raw, ",") {
		candidate := strings.TrimSpace(part)
		if candidate == target {
			return true
		}
	}
	return false
}

func (s *stateStore) reloadProjectFromRepo(ctx context.Context, project protocol.ProjectSummary) error {
	if strings.TrimSpace(project.RepoURL) == "" {
		return fmt.Errorf("project has no repo_url configured")
	}
	configFile := strings.TrimSpace(project.ConfigFile)
	if configFile == "" {
		configFile = "ciwi-project.yaml"
	}

	tmpDir, err := os.MkdirTemp("", "ciwi-reload-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	reloadCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	fetchRes, err := fetchProjectConfigAndIcon(reloadCtx, tmpDir, project.RepoURL, project.RepoRef, configFile)
	if err != nil {
		return err
	}
	_, err = s.persistImportedProject(protocol.ImportProjectRequest{
		RepoURL:    project.RepoURL,
		RepoRef:    project.RepoRef,
		ConfigFile: configFile,
	}, fetchRes.ConfigContent, fetchRes.SourceCommit, fetchRes.IconContentType, fetchRes.IconContentBytes)
	return err
}

func (s *stateStore) listProjectsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	projects, err := s.projectStore().ListProjects()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, projectListViewResponse{Projects: projects})
}

func (s *stateStore) persistImportedProject(req protocol.ImportProjectRequest, cfgContent, loadedCommit, iconContentType string, iconContent []byte) (protocol.ImportProjectResponse, error) {
	cfg, err := config.Parse([]byte(cfgContent), req.ConfigFile)
	if err != nil {
		return protocol.ImportProjectResponse{}, err
	}

	for i := range cfg.Pipelines {
		if strings.TrimSpace(cfg.Pipelines[i].Source.Repo) == "" {
			cfg.Pipelines[i].Source.Repo = req.RepoURL
		}
		if strings.TrimSpace(cfg.Pipelines[i].Source.Ref) == "" {
			cfg.Pipelines[i].Source.Ref = req.RepoRef
		}
	}

	configPath := fmt.Sprintf("%s:%s", req.RepoURL, req.ConfigFile)
	if req.RepoRef != "" {
		configPath = fmt.Sprintf("%s@%s:%s", req.RepoURL, req.RepoRef, req.ConfigFile)
	}
	if err := s.projectStore().LoadConfig(cfg, configPath, req.RepoURL, req.RepoRef, req.ConfigFile); err != nil {
		return protocol.ImportProjectResponse{}, err
	}
	if project, err := s.projectStore().GetProjectByName(cfg.Project.Name); err == nil {
		if commitErr := s.projectStore().SetProjectLoadedCommit(project.ID, strings.TrimSpace(loadedCommit)); commitErr != nil {
			return protocol.ImportProjectResponse{}, commitErr
		}
		s.setProjectIcon(project.ID, iconContentType, iconContent)
	}

	return protocol.ImportProjectResponse{
		ProjectName: cfg.Project.Name,
		RepoURL:     req.RepoURL,
		RepoRef:     req.RepoRef,
		ConfigFile:  req.ConfigFile,
		Pipelines:   len(cfg.Pipelines),
	}, nil
}
