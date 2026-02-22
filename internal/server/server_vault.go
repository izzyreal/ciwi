package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/izzyreal/ciwi/internal/protocol"
)

func (s *stateStore) vaultConnectionsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		items, err := s.vaultStore().ListVaultConnections()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, vaultConnectionsResponse{Connections: items})
	case http.MethodPost:
		var req protocol.UpsertVaultConnectionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON body", http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.URL) == "" || strings.TrimSpace(req.RoleID) == "" || strings.TrimSpace(req.SecretIDEnv) == "" {
			http.Error(w, "name, url, role_id and secret_id_env are required", http.StatusBadRequest)
			return
		}
		item, err := s.vaultStore().UpsertVaultConnection(req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusCreated, vaultConnectionResponse{Connection: item})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *stateStore) vaultConnectionByIDHandler(w http.ResponseWriter, r *http.Request) {
	rel := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/vault/connections/"), "/")
	if rel == "" {
		http.NotFound(w, r)
		return
	}
	parts := strings.Split(rel, "/")
	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "invalid vault connection id", http.StatusBadRequest)
		return
	}
	if len(parts) == 1 {
		if r.Method != http.MethodDelete {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if err := s.vaultStore().DeleteVaultConnection(id); err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, vaultConnectionDeleteResponse{Deleted: true, ID: id})
		return
	}
	if len(parts) != 2 || parts[1] != "test" || r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	var req protocol.TestVaultConnectionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	conn, err := s.vaultStore().GetVaultConnectionByID(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	token, err := s.getVaultToken(r.Context(), conn, req.SecretIDOverride)
	if err != nil {
		writeJSON(w, http.StatusOK, protocol.TestVaultConnectionResponse{OK: false, Message: err.Error()})
		return
	}
	if req.TestSecret != nil {
		if _, err := s.readVaultSecret(r.Context(), conn, *req.TestSecret); err != nil {
			writeJSON(w, http.StatusOK, protocol.TestVaultConnectionResponse{OK: false, Message: err.Error()})
			return
		}
	}
	writeJSON(w, http.StatusOK, protocol.TestVaultConnectionResponse{OK: true, Message: "vault auth ok, token=" + token[:minInt(8, len(token))] + "..."})
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
