//go:build darwin || linux || windows

package gio

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type nativePreferences struct {
	Theme          string          `json:"theme"`
	Disclosures    map[string]bool `json:"disclosures,omitempty"`
	ConnectionMode string          `json:"connection_mode,omitempty"`
	ServerEndpoint string          `json:"server_endpoint,omitempty"`
}

const (
	connectionModeDiscover = "discover"
	connectionModeExplicit = "explicit"
)

func (p nativePreferences) normalizedConnection() (string, string) {
	mode := p.ConnectionMode
	if mode != connectionModeExplicit {
		mode = connectionModeDiscover
	}
	return mode, p.ServerEndpoint
}

type nativeConnectionSettings struct {
	Mode     string
	Endpoint string
	Address  string
}

func nativeConnectionSettingsForLaunch(preferences nativePreferences, addressOverride string) nativeConnectionSettings {
	mode, endpoint := preferences.normalizedConnection()
	settings := nativeConnectionSettings{Mode: mode, Endpoint: endpoint}
	if addressOverride = strings.TrimSpace(addressOverride); addressOverride != "" {
		settings.Mode = connectionModeExplicit
		settings.Endpoint = addressOverride
		settings.Address = addressOverride
	} else if mode == connectionModeExplicit {
		settings.Address = strings.TrimSpace(endpoint)
	}
	return settings
}

var nativePreferencesMu sync.Mutex

func nativePreferencesPath() (string, error) {
	directory, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locate native client preferences: %w", err)
	}
	return filepath.Join(directory, "ciwi", "native-ui.json"), nil
}

func loadNativePreferences(path string) (nativePreferences, error) {
	payload, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nativePreferences{}, nil
	}
	if err != nil {
		return nativePreferences{}, fmt.Errorf("read native client preferences: %w", err)
	}
	var preferences nativePreferences
	if err := json.Unmarshal(payload, &preferences); err != nil {
		return nativePreferences{}, fmt.Errorf("decode native client preferences: %w", err)
	}
	return preferences, nil
}

func saveNativePreferences(path string, preferences nativePreferences) error {
	payload, err := json.MarshalIndent(preferences, "", "  ")
	if err != nil {
		return fmt.Errorf("encode native client preferences: %w", err)
	}
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create native client preferences directory: %w", err)
	}
	temporary, err := os.CreateTemp(directory, "native-ui-*.json")
	if err != nil {
		return fmt.Errorf("create native client preferences file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer func() { _ = os.Remove(temporaryPath) }()
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("protect native client preferences file: %w", err)
	}
	if _, err := temporary.Write(payload); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write native client preferences: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close native client preferences: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("replace native client preferences: %w", err)
	}
	return nil
}

func updateNativePreferences(path string, update func(*nativePreferences)) error {
	nativePreferencesMu.Lock()
	defer nativePreferencesMu.Unlock()
	preferences, err := loadNativePreferences(path)
	if err != nil {
		return err
	}
	update(&preferences)
	return saveNativePreferences(path, preferences)
}
