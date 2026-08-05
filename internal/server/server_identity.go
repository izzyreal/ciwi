package server

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/izzyreal/ciwi/internal/store"
)

const serverInstallationIDStateKey = "server_installation_id"

func ensureServerInstallationID(db *store.Store) (string, error) {
	if value, ok, err := db.GetAppState(serverInstallationIDStateKey); err != nil {
		return "", fmt.Errorf("load server installation identity: %w", err)
	} else if ok && strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value), nil
	}
	value := uuid.NewString()
	if err := db.SetAppState(serverInstallationIDStateKey, value); err != nil {
		return "", fmt.Errorf("persist server installation identity: %w", err)
	}
	return value, nil
}
