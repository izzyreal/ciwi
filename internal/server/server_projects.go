package server

import (
	"context"
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
)

func (s *stateStore) loadConfigHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req protocol.LoadConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	if req.ConfigPath == "" {
		req.ConfigPath = "ciwi.yaml"
	}

	fullPath, err := resolveConfigPath(req.ConfigPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	cfg, err := config.Load(fullPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.db.LoadConfig(cfg, fullPath, "", "", filepath.Base(fullPath)); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, protocol.LoadConfigResponse{ProjectName: cfg.Project.Name, ConfigPath: fullPath, Pipelines: len(cfg.Pipelines)})
}

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

	cfgContent, err := fetchConfigFileFromRepo(importCtx, tmpDir, req.RepoURL, req.RepoRef, req.ConfigFile)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	resp, err := s.persistImportedProject(req, cfgContent)
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
		detail, err := s.db.GetProjectDetail(projectID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"project": detail})
		return
	}

	if len(parts) != 2 {
		http.NotFound(w, r)
		return
	}
	switch parts[1] {
	case "reload":
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		project, err := s.db.GetProjectByID(projectID)
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

		cfgContent, err := fetchConfigFileFromRepo(reloadCtx, tmpDir, project.RepoURL, project.RepoRef, configFile)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		resp, err := s.persistImportedProject(protocol.ImportProjectRequest{
			RepoURL:    project.RepoURL,
			RepoRef:    project.RepoRef,
			ConfigFile: configFile,
		}, cfgContent)
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

func fetchConfigFileFromRepo(ctx context.Context, tmpDir, repoURL, repoRef, configFile string) (string, error) {
	if out, err := runCmd(ctx, "", "git", "init", "-q", tmpDir); err != nil {
		return "", fmt.Errorf("git init failed: %v\n%s", err, out)
	}
	if out, err := runCmd(ctx, "", "git", "-C", tmpDir, "remote", "add", "origin", repoURL); err != nil {
		return "", fmt.Errorf("git remote add failed: %v\n%s", err, out)
	}

	ref := strings.TrimSpace(repoRef)
	if ref == "" {
		ref = "HEAD"
	}

	if out, err := runCmd(ctx, "", "git", "-C", tmpDir, "fetch", "-q", "--depth", "1", "origin", ref); err != nil {
		return "", fmt.Errorf("git fetch failed: %v\n%s", err, out)
	}

	out, err := runCmd(ctx, "", "git", "-C", tmpDir, "show", "FETCH_HEAD:"+configFile)
	if err != nil {
		return "", fmt.Errorf("repo is not a valid ciwi project: missing root file %q", configFile)
	}

	return out, nil
}

func (s *stateStore) listProjectsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	projects, err := s.db.ListProjects()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"projects": projects})
}

func (s *stateStore) persistImportedProject(req protocol.ImportProjectRequest, cfgContent string) (protocol.ImportProjectResponse, error) {
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
	if err := s.db.LoadConfig(cfg, configPath, req.RepoURL, req.RepoRef, req.ConfigFile); err != nil {
		return protocol.ImportProjectResponse{}, err
	}

	return protocol.ImportProjectResponse{
		ProjectName: cfg.Project.Name,
		RepoURL:     req.RepoURL,
		RepoRef:     req.RepoRef,
		ConfigFile:  req.ConfigFile,
		Pipelines:   len(cfg.Pipelines),
	}, nil
}
