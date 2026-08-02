package application

import (
	"context"
	"testing"
)

type updateBackendStub struct {
	status   ServerUpdateStatus
	check    ServerUpdateCheckResult
	versions ServerUpdateVersions
	action   ServerUpdateActionRequest
}

func (s *updateBackendStub) GetServerUpdateStatus(context.Context) (ServerUpdateStatus, error) {
	return s.status, nil
}
func (s *updateBackendStub) CheckForServerUpdates(context.Context) (ServerUpdateCheckResult, error) {
	return s.check, nil
}
func (s *updateBackendStub) ListServerUpdateVersions(context.Context) (ServerUpdateVersions, error) {
	return s.versions, nil
}
func (s *updateBackendStub) ExecuteServerUpdateAction(_ context.Context, request ServerUpdateActionRequest) (ServerUpdateActionResult, error) {
	s.action = request
	return ServerUpdateActionResult{Message: "accepted"}, nil
}

func TestServerUpdateOperationsValidateAndPublish(t *testing.T) {
	backend := &updateBackendStub{}
	hub := NewChangeHub()
	operations := NewServerUpdateOperations(backend, hub)
	before := hub.Snapshot().Revision

	if _, err := operations.Execute(context.Background(), ServerUpdateActionRequest{Action: "rollback"}); ErrorKindOf(err) != ErrorInvalidArgument {
		t.Fatalf("rollback error = %v", err)
	}
	if _, err := operations.Execute(context.Background(), ServerUpdateActionRequest{Action: " ROLLBACK ", TargetVersion: " v0.1.9 "}); err != nil {
		t.Fatal(err)
	}
	if backend.action.Action != ServerUpdateActionRollback || backend.action.TargetVersion != "v0.1.9" {
		t.Fatalf("normalized action = %+v", backend.action)
	}
	if got := hub.Snapshot().Revision; got != before+1 {
		t.Fatalf("revision = %d, want %d", got, before+1)
	}
	if _, err := operations.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := hub.Snapshot().Revision; got != before+2 {
		t.Fatalf("revision after check = %d, want %d", got, before+2)
	}
}
