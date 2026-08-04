//go:build (!darwin && !ios) || !cgo

package gio

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

func loadSSHDevicePrivateKey(preferencesPath string) ([]byte, error) {
	payload, err := os.ReadFile(preferencesPath + ".ssh-key")
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read SSH device key: %w", err)
	}
	return payload, nil
}

func saveSSHDevicePrivateKey(preferencesPath string, privateKey []byte) error {
	if err := os.MkdirAll(filepath.Dir(preferencesPath), 0o700); err != nil {
		return fmt.Errorf("create SSH device key directory: %w", err)
	}
	if err := os.WriteFile(preferencesPath+".ssh-key", privateKey, 0o600); err != nil {
		return fmt.Errorf("store SSH device key: %w", err)
	}
	return nil
}
