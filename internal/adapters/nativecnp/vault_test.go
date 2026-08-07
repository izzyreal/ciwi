package nativecnp

import (
	"context"
	"testing"

	"github.com/izzyreal/ciwi/internal/protocol"
	cnpv1 "github.com/izzyreal/ciwi/pkg/cnp/v1"
)

type vaultServiceStub struct{}

func (vaultServiceStub) ListVaultConnections(context.Context) ([]protocol.VaultConnection, error) {
	return []protocol.VaultConnection{{ID: 7, Name: "home-vault", URL: "https://vault.example", AppRoleMount: "approle"}}, nil
}
func (vaultServiceStub) UpsertVaultConnection(_ context.Context, request protocol.UpsertVaultConnectionRequest) (protocol.VaultConnection, error) {
	return protocol.VaultConnection{ID: 8, Name: request.Name, URL: request.URL}, nil
}
func (vaultServiceStub) TestVaultConnection(context.Context, int64, string) (protocol.TestVaultConnectionResponse, error) {
	return protocol.TestVaultConnectionResponse{OK: true, Message: "vault auth ok"}, nil
}
func (vaultServiceStub) DeleteVaultConnection(context.Context, int64) error { return nil }

func TestVaultOperationsCrossNativeProtocol(t *testing.T) {
	handler := &Handler{services: Services{Vault: vaultServiceStub{}}}
	execute := func(request *cnpv1.Request) *cnpv1.Response {
		t.Helper()
		request.Metadata = &cnpv1.RequestMetadata{RequestId: "vault-test", IdempotencyKey: "key"}
		return handler.execute(context.Background(), request)
	}
	listed := execute(&cnpv1.Request{Operation: &cnpv1.Request_ListVaultConnections{ListVaultConnections: &cnpv1.Empty{}}}).GetVaultConnectionList()
	if listed == nil || len(listed.Connections) != 1 || listed.Connections[0].Name != "home-vault" {
		t.Fatalf("list result = %#v", listed)
	}
	saved := execute(&cnpv1.Request{Operation: &cnpv1.Request_UpsertVaultConnection{UpsertVaultConnection: &cnpv1.UpsertVaultConnectionRequest{Name: "release", Url: "https://vault.example"}}}).GetVaultConnection()
	if saved == nil || saved.Id != 8 || saved.Name != "release" {
		t.Fatalf("save result = %#v", saved)
	}
	tested := execute(&cnpv1.Request{Operation: &cnpv1.Request_TestVaultConnection{TestVaultConnection: &cnpv1.TestVaultConnectionRequest{Id: 7}}}).GetTestVaultConnection()
	if tested == nil || !tested.Ok {
		t.Fatalf("test result = %#v", tested)
	}
	deleted := execute(&cnpv1.Request{Operation: &cnpv1.Request_DeleteVaultConnection{DeleteVaultConnection: &cnpv1.VaultConnectionIDRequest{Id: 7}}}).GetDeleteVaultConnection()
	if deleted == nil || !deleted.Deleted || deleted.Id != 7 {
		t.Fatalf("delete result = %#v", deleted)
	}
}
