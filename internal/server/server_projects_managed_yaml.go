package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/izzyreal/ciwi/internal/application"
	"github.com/izzyreal/ciwi/internal/config"
	"github.com/izzyreal/ciwi/internal/protocol"
	"github.com/izzyreal/ciwi/internal/store"
)

const managedYAMLMaxRequestBytes = 2 << 20

var errManagedYAMLRequestTooLarge = errors.New("managed YAML request exceeds 2 MiB")

func (s *stateStore) validateManagedYAMLHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req protocol.ValidateManagedYAMLRequest
	if err := decodeManagedYAMLRequest(w, r, &req); err != nil {
		writeManagedYAMLRequestError(w, err)
		return
	}
	cfg, err := parseManagedYAML(req.YAML)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.projectStore().ValidateManagedYAMLName(cfg.Project.Name, req.ProjectID); err != nil {
		writeManagedYAMLError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, protocol.ManagedYAMLDefinition{
		ProjectID:      req.ProjectID,
		ProjectName:    cfg.Project.Name,
		Pipelines:      len(cfg.Pipelines),
		PipelineChains: len(cfg.PipelineChains),
	})
}

func (s *stateStore) createManagedYAMLHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req protocol.ManagedYAMLRequest
	if err := decodeManagedYAMLRequest(w, r, &req); err != nil {
		writeManagedYAMLRequestError(w, err)
		return
	}
	cfg, err := parseManagedYAML(req.YAML)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	definition, err := s.projectStore().CreateManagedYAMLProject(cfg, req.YAML)
	if err != nil {
		writeManagedYAMLError(w, err)
		return
	}
	definition.YAML = ""
	s.app().changes.Publish(application.ChangeProjects)
	writeJSON(w, http.StatusCreated, definition)
}

func (s *stateStore) managedYAMLProjectHandler(w http.ResponseWriter, r *http.Request, projectID int64) {
	switch r.Method {
	case http.MethodGet:
		definition, err := s.projectStore().GetManagedYAMLProject(projectID)
		if err != nil {
			writeManagedYAMLError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, definition)
	case http.MethodPut:
		var req protocol.UpdateManagedYAMLRequest
		if err := decodeManagedYAMLRequest(w, r, &req); err != nil {
			writeManagedYAMLRequestError(w, err)
			return
		}
		if strings.TrimSpace(req.Revision) == "" {
			http.Error(w, "revision is required", http.StatusBadRequest)
			return
		}
		cfg, err := parseManagedYAML(req.YAML)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		definition, err := s.projectStore().UpdateManagedYAMLProject(projectID, strings.TrimSpace(req.Revision), cfg, req.YAML)
		if err != nil {
			writeManagedYAMLError(w, err)
			return
		}
		definition.YAML = ""
		s.app().changes.Publish(application.ChangeProjects)
		writeJSON(w, http.StatusOK, definition)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func decodeManagedYAMLRequest(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, managedYAMLMaxRequestBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			return errManagedYAMLRequestTooLarge
		}
		return fmt.Errorf("invalid JSON body: %w", err)
	}
	return nil
}

func writeManagedYAMLRequestError(w http.ResponseWriter, err error) {
	if errors.Is(err, errManagedYAMLRequestTooLarge) {
		http.Error(w, err.Error(), http.StatusRequestEntityTooLarge)
		return
	}
	http.Error(w, err.Error(), http.StatusBadRequest)
}

func parseManagedYAML(raw string) (config.File, error) {
	if strings.TrimSpace(raw) == "" {
		return config.File{}, fmt.Errorf("yaml is required")
	}
	cfg, err := config.Parse([]byte(raw), "managed YAML")
	if err != nil {
		return config.File{}, err
	}
	cfg.Project.Name = strings.TrimSpace(cfg.Project.Name)
	return cfg, nil
}

func writeManagedYAMLError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, store.ErrManagedYAMLNameConflict), errors.Is(err, store.ErrManagedYAMLRevisionConflict), errors.Is(err, store.ErrProjectIsNotManagedYAML):
		status = http.StatusConflict
	case strings.Contains(strings.ToLower(err.Error()), "not found"):
		status = http.StatusNotFound
	}
	http.Error(w, err.Error(), status)
}
