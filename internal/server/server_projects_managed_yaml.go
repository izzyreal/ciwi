package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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

func (s *stateStore) GetManagedYAML(_ context.Context, projectID int64) (protocol.ManagedYAMLDefinition, error) {
	definition, err := s.projectStore().GetManagedYAMLProject(projectID)
	return definition, managedYAMLApplicationError(err)
}

func (s *stateStore) ValidateManagedYAML(_ context.Context, projectID int64, raw string) (protocol.ManagedYAMLDefinition, error) {
	if len(raw) > managedYAMLMaxRequestBytes {
		return protocol.ManagedYAMLDefinition{}, application.NewError(application.ErrorInvalidArgument, errManagedYAMLRequestTooLarge.Error(), errManagedYAMLRequestTooLarge)
	}
	cfg, err := parseManagedYAML(raw)
	if err != nil {
		return protocol.ManagedYAMLDefinition{}, application.NewError(application.ErrorInvalidArgument, err.Error(), err)
	}
	if err := s.projectStore().ValidateManagedYAMLName(cfg.Project.Name, projectID); err != nil {
		return protocol.ManagedYAMLDefinition{}, managedYAMLApplicationError(err)
	}
	return protocol.ManagedYAMLDefinition{ProjectID: projectID, ProjectName: cfg.Project.Name, Pipelines: len(cfg.Pipelines), PipelineChains: len(cfg.PipelineChains)}, nil
}

func (s *stateStore) SaveManagedYAML(ctx context.Context, projectID int64, revision, raw, idempotencyKey string) (protocol.ManagedYAMLDefinition, error) {
	if _, err := s.ValidateManagedYAML(ctx, projectID, raw); err != nil {
		return protocol.ManagedYAMLDefinition{}, err
	}
	cfg, err := parseManagedYAML(raw)
	if err != nil {
		return protocol.ManagedYAMLDefinition{}, application.NewError(application.ErrorInvalidArgument, err.Error(), err)
	}
	if projectID > 0 && strings.TrimSpace(revision) == "" {
		return protocol.ManagedYAMLDefinition{}, application.NewError(application.ErrorInvalidArgument, "revision is required", nil)
	}
	payload, _ := json.Marshal(struct {
		ProjectID int64  `json:"project_id"`
		Revision  string `json:"revision"`
		YAML      string `json:"yaml"`
	}{projectID, strings.TrimSpace(revision), raw})
	fingerprint := sha256.Sum256(payload)
	execute := func() (protocol.ManagedYAMLDefinition, error) {
		var definition protocol.ManagedYAMLDefinition
		if projectID > 0 {
			definition, err = s.projectStore().UpdateManagedYAMLProject(projectID, strings.TrimSpace(revision), cfg, raw)
		} else {
			definition, err = s.projectStore().CreateManagedYAMLProject(cfg, raw)
		}
		if err != nil {
			return protocol.ManagedYAMLDefinition{}, managedYAMLApplicationError(err)
		}
		s.app().changes.Publish(application.ChangeProjects)
		return definition, nil
	}
	return application.ExecuteIdempotentCommand(ctx, s.app().receipts, strings.TrimSpace(idempotencyKey), "managed_yaml_save", hex.EncodeToString(fingerprint[:]), execute)
}

func managedYAMLApplicationError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, store.ErrManagedYAMLNameConflict), errors.Is(err, store.ErrManagedYAMLRevisionConflict), errors.Is(err, store.ErrProjectIsNotManagedYAML):
		return application.NewError(application.ErrorConflict, err.Error(), err)
	case strings.Contains(err.Error(), "not found"):
		return application.NewError(application.ErrorNotFound, err.Error(), err)
	default:
		return application.WrapInternal("managed YAML", err)
	}
}

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
