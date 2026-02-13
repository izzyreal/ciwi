package server

import "github.com/izzyreal/ciwi/internal/protocol"

type vaultConnectionsResponse struct {
	Connections []protocol.VaultConnection `json:"connections"`
}

type vaultConnectionResponse struct {
	Connection protocol.VaultConnection `json:"connection"`
}

type vaultConnectionDeleteResponse struct {
	Deleted bool  `json:"deleted"`
	ID      int64 `json:"id"`
}

type projectVaultSettingsResponse struct {
	Settings protocol.ProjectVaultSettings `json:"settings"`
}
