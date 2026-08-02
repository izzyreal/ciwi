package application

import (
	"context"
	"testing"
)

type projectMutatorStub struct{ request ProjectActionRequest }

func (s *projectMutatorStub) ExecuteProjectAction(_ context.Context, request ProjectActionRequest) (ProjectActionResult, error) {
	s.request = request
	return ProjectActionResult{ProjectID: request.ProjectID, Message: request.Action + " complete"}, nil
}

func (s *projectMutatorStub) ImportProject(_ context.Context, request ImportProjectRequest) (ImportProjectResult, error) {
	return ImportProjectResult{ProjectName: "ciwi", RepoURL: request.RepoURL, ConfigFile: request.ConfigFile}, nil
}

func TestProjectCommandsValidateExecuteAndPublish(t *testing.T) {
	mutator := &projectMutatorStub{}
	changes := NewChangeHub()
	commands := NewProjectCommands(mutator, nil, changes)
	watchCtx, cancel := context.WithCancel(t.Context())
	defer cancel()
	events := changes.Watch(watchCtx)
	<-events
	result, err := commands.Execute(t.Context(), ProjectActionRequest{ProjectID: 7, Action: "RELOAD"})
	if err != nil || result.ProjectID != 7 || mutator.request.Action != ProjectActionReload {
		t.Fatalf("result=%+v request=%+v err=%v", result, mutator.request, err)
	}
	if change := <-events; len(change.Topics) != 1 || change.Topics[0] != ChangeProjects {
		t.Fatalf("change=%+v", change)
	}
	if _, err := commands.Execute(t.Context(), ProjectActionRequest{ProjectID: 0, Action: ProjectActionDelete}); ErrorKindOf(err) != ErrorInvalidArgument {
		t.Fatalf("invalid project error=%v", err)
	}
}

func TestProjectImportDefaultsConfigAndPublishes(t *testing.T) {
	changes := NewChangeHub()
	commands := NewProjectCommands(&projectMutatorStub{}, nil, changes)
	watchCtx, cancel := context.WithCancel(t.Context())
	defer cancel()
	events := changes.Watch(watchCtx)
	<-events
	result, err := commands.Import(t.Context(), ImportProjectRequest{RepoURL: " https://example.test/repo.git "})
	if err != nil || result.ConfigFile != "ciwi-project.yaml" {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if change := <-events; len(change.Topics) != 1 || change.Topics[0] != ChangeProjects {
		t.Fatalf("change=%+v", change)
	}
}
