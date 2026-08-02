package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
)

const (
	ProjectActionReload = "reload"
	ProjectActionDelete = "delete"
)

type ProjectActionRequest struct {
	ProjectID      int64
	Action         string
	IdempotencyKey string
}

type ProjectActionResult struct {
	ProjectID int64  `json:"project_id"`
	Message   string `json:"message"`
}

type ImportProjectRequest struct {
	RepoURL        string
	RepoRef        string
	ConfigFile     string
	IdempotencyKey string
}

type ImportProjectResult struct {
	ProjectName string `json:"project_name"`
	RepoURL     string `json:"repo_url"`
	RepoRef     string `json:"repo_ref"`
	ConfigFile  string `json:"config_file"`
	Pipelines   int    `json:"pipelines"`
}

type ProjectMutator interface {
	ExecuteProjectAction(context.Context, ProjectActionRequest) (ProjectActionResult, error)
	ImportProject(context.Context, ImportProjectRequest) (ImportProjectResult, error)
}

func (c *ProjectCommands) Import(ctx context.Context, request ImportProjectRequest) (ImportProjectResult, error) {
	request.RepoURL = strings.TrimSpace(request.RepoURL)
	request.RepoRef = strings.TrimSpace(request.RepoRef)
	request.ConfigFile = strings.TrimSpace(request.ConfigFile)
	if request.ConfigFile == "" {
		request.ConfigFile = "ciwi-project.yaml"
	}
	if request.RepoURL == "" || request.ConfigFile == "." || strings.ContainsAny(request.ConfigFile, `/\\`) {
		return ImportProjectResult{}, NewError(ErrorInvalidArgument, "a repository URL and root-level config file are required", nil)
	}
	if c == nil || c.mutator == nil {
		return ImportProjectResult{}, NewError(ErrorUnavailable, "project operator unavailable", nil)
	}
	key, err := validateCommandKey(request.IdempotencyKey)
	if err != nil {
		return ImportProjectResult{}, err
	}
	execute := func() (ImportProjectResult, error) {
		result, executeErr := c.mutator.ImportProject(ctx, request)
		if executeErr == nil && c.changes != nil {
			c.changes.Publish(ChangeProjects)
		}
		return result, executeErr
	}
	if key == "" || c.receipts == nil {
		return execute()
	}
	fingerprint := request
	fingerprint.IdempotencyKey = ""
	payload, err := json.Marshal(fingerprint)
	if err != nil {
		return ImportProjectResult{}, WrapInternal("fingerprint project import", err)
	}
	sum := sha256.Sum256(payload)
	return executeIdempotentCommand(ctx, c.receipts, key, "project_import", hex.EncodeToString(sum[:]), execute)
}

type ProjectCommands struct {
	mutator  ProjectMutator
	receipts CommandReceiptRepository
	changes  *ChangeHub
}

func NewProjectCommands(mutator ProjectMutator, receipts CommandReceiptRepository, changes *ChangeHub) *ProjectCommands {
	return &ProjectCommands{mutator: mutator, receipts: receipts, changes: changes}
}

func (c *ProjectCommands) Execute(ctx context.Context, request ProjectActionRequest) (ProjectActionResult, error) {
	request.Action = strings.ToLower(strings.TrimSpace(request.Action))
	if request.ProjectID <= 0 || (request.Action != ProjectActionReload && request.Action != ProjectActionDelete) {
		return ProjectActionResult{}, NewError(ErrorInvalidArgument, "a valid project and action are required", nil)
	}
	if c == nil || c.mutator == nil {
		return ProjectActionResult{}, NewError(ErrorUnavailable, "project operator unavailable", nil)
	}
	key, err := validateCommandKey(request.IdempotencyKey)
	if err != nil {
		return ProjectActionResult{}, err
	}
	execute := func() (ProjectActionResult, error) {
		result, executeErr := c.mutator.ExecuteProjectAction(ctx, request)
		if executeErr == nil && c.changes != nil {
			c.changes.Publish(ChangeProjects)
		}
		return result, executeErr
	}
	if key == "" || c.receipts == nil {
		return execute()
	}
	fingerprint := request
	fingerprint.IdempotencyKey = ""
	payload, err := json.Marshal(fingerprint)
	if err != nil {
		return ProjectActionResult{}, WrapInternal("fingerprint project action", err)
	}
	sum := sha256.Sum256(payload)
	return executeIdempotentCommand(ctx, c.receipts, key, "project_action", hex.EncodeToString(sum[:]), execute)
}
