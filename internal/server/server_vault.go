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

func (s *stateStore) projectVaultHandler(w http.ResponseWriter, r *http.Request, projectID int64) {
	switch r.Method {
	case http.MethodGet:
		settings, err := s.vaultStore().GetProjectVaultSettings(projectID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, projectVaultSettingsResponse{Settings: settings})
	case http.MethodPut:
		var req protocol.UpdateProjectVaultRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON body", http.StatusBadRequest)
			return
		}
		for _, sec := range req.Secrets {
			if strings.TrimSpace(sec.Name) == "" || strings.TrimSpace(sec.Path) == "" || strings.TrimSpace(sec.Key) == "" {
				http.Error(w, "each secret requires name, path and key", http.StatusBadRequest)
				return
			}
		}
		settings, err := s.vaultStore().UpdateProjectVaultSettings(projectID, req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, projectVaultSettingsResponse{Settings: settings})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *stateStore) projectVaultTestHandler(w http.ResponseWriter, r *http.Request, projectID int64) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req protocol.TestVaultConnectionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	settings, err := s.vaultStore().GetProjectVaultSettings(projectID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	if settings.VaultConnectionID <= 0 {
		if strings.TrimSpace(settings.VaultConnectionName) == "" {
			writeJSON(w, http.StatusOK, protocol.TestProjectVaultResponse{OK: false, Message: "project has no vault connection configured"})
			return
		}
	}
	var conn protocol.VaultConnection
	if settings.VaultConnectionID > 0 {
		conn, err = s.vaultStore().GetVaultConnectionByID(settings.VaultConnectionID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
	} else {
		conn, err = s.vaultStore().GetVaultConnectionByName(settings.VaultConnectionName)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
	}
	if _, err := s.getVaultToken(r.Context(), conn, req.SecretIDOverride); err != nil {
		writeJSON(w, http.StatusOK, protocol.TestProjectVaultResponse{OK: false, Message: err.Error()})
		return
	}
	details := map[string]string{}
	ok := true
	for _, sp := range settings.Secrets {
		_, err := s.readVaultSecret(r.Context(), conn, sp)
		if err != nil {
			ok = false
			details[sp.Name] = err.Error()
		} else {
			details[sp.Name] = "ok"
		}
	}
	msg := "vault access test ok"
	if !ok {
		msg = "one or more secrets failed"
	}
	writeJSON(w, http.StatusOK, protocol.TestProjectVaultResponse{OK: ok, Message: msg, Details: details})
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
