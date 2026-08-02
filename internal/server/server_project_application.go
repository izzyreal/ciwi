package server

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/izzyreal/ciwi/internal/application"
	"github.com/izzyreal/ciwi/internal/protocol"
)

type projectMutatorAdapter struct{ state *stateStore }

func (a projectMutatorAdapter) ExecuteProjectAction(ctx context.Context, request application.ProjectActionRequest) (application.ProjectActionResult, error) {
	if err := ctx.Err(); err != nil {
		return application.ProjectActionResult{}, err
	}
	s := a.state
	switch request.Action {
	case application.ProjectActionReload:
		project, err := s.projectStore().GetProjectByID(request.ProjectID)
		if err != nil {
			return application.ProjectActionResult{}, projectActionError("find project", err)
		}
		if project.SourceKind == protocol.ProjectSourceManagedYAML {
			return application.ProjectActionResult{}, application.NewError(application.ErrorConflict, "managed YAML projects cannot be reloaded from VCS", nil)
		}
		if err := s.reloadProjectFromRepo(ctx, project); err != nil {
			return application.ProjectActionResult{}, application.NewError(application.ErrorInvalidArgument, err.Error(), err)
		}
		return application.ProjectActionResult{ProjectID: request.ProjectID, Message: "project reloaded"}, nil
	case application.ProjectActionDelete:
		if err := s.projectStore().DeleteProjectByID(request.ProjectID); err != nil {
			return application.ProjectActionResult{}, projectActionError("delete project", err)
		}
		s.setProjectIcon(request.ProjectID, "", nil)
		return application.ProjectActionResult{ProjectID: request.ProjectID, Message: "project deleted"}, nil
	default:
		return application.ProjectActionResult{}, application.NewError(application.ErrorInvalidArgument, "unsupported project action", nil)
	}
}

func (a projectMutatorAdapter) ImportProject(ctx context.Context, request application.ImportProjectRequest) (application.ImportProjectResult, error) {
	if _, err := exec.LookPath("git"); err != nil {
		return application.ImportProjectResult{}, application.WrapInternal("find git", err)
	}
	tmpDir, err := os.MkdirTemp("", "ciwi-import-*")
	if err != nil {
		return application.ImportProjectResult{}, application.WrapInternal("create import workspace", err)
	}
	defer os.RemoveAll(tmpDir)
	importCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	fetch, err := fetchProjectConfigAndIcon(importCtx, tmpDir, request.RepoURL, request.RepoRef, request.ConfigFile)
	if err != nil {
		return application.ImportProjectResult{}, application.NewError(application.ErrorInvalidArgument, err.Error(), err)
	}
	result, err := a.state.persistImportedProject(protocol.ImportProjectRequest{
		RepoURL: request.RepoURL, RepoRef: request.RepoRef, ConfigFile: request.ConfigFile,
	}, fetch.ConfigContent, fetch.SourceCommit, fetch.ResolvedRef, fetch.IconContentType, fetch.IconContentBytes)
	if err != nil {
		return application.ImportProjectResult{}, application.NewError(application.ErrorInvalidArgument, err.Error(), err)
	}
	return application.ImportProjectResult{
		ProjectName: result.ProjectName, RepoURL: result.RepoURL, RepoRef: result.RepoRef,
		ConfigFile: result.ConfigFile, Pipelines: result.Pipelines,
	}, nil
}

func projectActionError(operation string, err error) error {
	if strings.Contains(strings.ToLower(err.Error()), "not found") {
		return application.NewError(application.ErrorNotFound, "project not found", err)
	}
	return application.WrapInternal(operation, err)
}
