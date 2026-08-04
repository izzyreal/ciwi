//go:build (darwin && !ios) || linux || windows

package gio

import (
	"fmt"
	"net/url"
	"os/exec"
	"runtime"
	"strings"
)

func openPlatformURL(raw string) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") {
		return fmt.Errorf("invalid web address")
	}
	var command *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		command = exec.Command("open", parsed.String())
	case "linux":
		command = exec.Command("xdg-open", parsed.String())
	case "windows":
		command = exec.Command("rundll32", "url.dll,FileProtocolHandler", parsed.String())
	default:
		return fmt.Errorf("opening links is unavailable on %s", runtime.GOOS)
	}
	if err := command.Start(); err != nil {
		return err
	}
	go func() { _ = command.Wait() }()
	return nil
}
