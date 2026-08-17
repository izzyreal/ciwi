//go:build (darwin && !ios) || linux || windows

package gio

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

func revealDownloadedFile(path string) error {
	command, err := revealDownloadedFileCommand(path)
	if err != nil {
		return err
	}
	if err := command.Start(); err != nil {
		return err
	}
	go func() { _ = command.Wait() }()
	return nil
}

func revealDownloadedFileCommand(path string) (*exec.Cmd, error) {
	path = strings.TrimSpace(path)
	if path == "" || !filepath.IsAbs(path) {
		return nil, fmt.Errorf("invalid downloaded file path")
	}
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", "-R", path), nil
	case "windows":
		return exec.Command("explorer.exe", "/select,", path), nil
	case "linux":
		return exec.Command("xdg-open", filepath.Dir(path)), nil
	default:
		return nil, fmt.Errorf("revealing downloaded files is unavailable on %s", runtime.GOOS)
	}
}
