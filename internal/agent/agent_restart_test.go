package agent

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRequestAgentRestartUsesServicePathByRuntime(t *testing.T) {
	origGOOS := agentRuntimeGOOS
	origLaunchd := restartViaLaunchd
	origSystemd := restartViaSystemd
	origWinSvc := restartViaWinSvc
	origExit := scheduleAgentExitFn
	t.Cleanup(func() {
		agentRuntimeGOOS = origGOOS
		restartViaLaunchd = origLaunchd
		restartViaSystemd = origSystemd
		restartViaWinSvc = origWinSvc
		scheduleAgentExitFn = origExit
	})

	exitCalls := 0
	scheduleAgentExitFn = func() { exitCalls++ }

	t.Run("linux success", func(t *testing.T) {
		agentRuntimeGOOS = "linux"
		restartViaSystemd = func() (string, error, bool) {
			return "restart via systemd requested (ciwi-agent.service)", nil, true
		}
		msg := requestAgentRestart()
		if !strings.Contains(msg, "restart via systemd requested") {
			t.Fatalf("unexpected message: %q", msg)
		}
	})

	t.Run("linux attempted failure falls back to exit", func(t *testing.T) {
		agentRuntimeGOOS = "linux"
		restartViaSystemd = func() (string, error, bool) {
			return "", errors.New("restart failed"), true
		}
		msg := requestAgentRestart()
		if !strings.Contains(msg, "service restart failed; fallback exit requested: restart failed") {
			t.Fatalf("unexpected message: %q", msg)
		}
	})

	t.Run("unknown runtime fallback", func(t *testing.T) {
		agentRuntimeGOOS = "plan9"
		msg := requestAgentRestart()
		if msg != "service restart unavailable; fallback exit requested" {
			t.Fatalf("unexpected fallback message: %q", msg)
		}
	})

	if exitCalls != 3 {
		t.Fatalf("expected one scheduled exit per request, got %d", exitCalls)
	}
}

func TestRestartAgentViaSystemdInvokesConfiguredBinary(t *testing.T) {
	tmp := t.TempDir()
	argsPath := filepath.Join(tmp, "args.txt")
	systemctl := filepath.Join(tmp, "systemctl")
	script := "#!/bin/sh\n" +
		"echo \"$@\" > \"" + argsPath + "\"\n"
	if err := os.WriteFile(systemctl, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake systemctl: %v", err)
	}

	t.Setenv("CIWI_SYSTEMCTL_PATH", systemctl)
	t.Setenv("CIWI_AGENT_SYSTEMD_SERVICE_NAME", "ciwi-agent-custom.service")
	msg, err, attempted := restartAgentViaSystemd()
	if !attempted {
		t.Fatalf("expected attempted=true")
	}
	if err != nil {
		t.Fatalf("restartAgentViaSystemd err: %v", err)
	}
	if !strings.Contains(msg, "ciwi-agent-custom.service") {
		t.Fatalf("unexpected message: %q", msg)
	}

	rawArgs, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatalf("read args: %v", err)
	}
	got := strings.TrimSpace(string(rawArgs))
	if got != "restart ciwi-agent-custom.service" {
		t.Fatalf("unexpected args: %q", got)
	}
}

func TestRestartAgentViaSystemdNotAttemptedWhenServiceNameBlank(t *testing.T) {
	t.Setenv("CIWI_AGENT_SYSTEMD_SERVICE_NAME", "  ")
	msg, err, attempted := restartAgentViaSystemd()
	if attempted {
		t.Fatalf("expected attempted=false")
	}
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if msg != "" {
		t.Fatalf("unexpected msg: %q", msg)
	}
}

func TestRestartAgentViaLaunchdBranches(t *testing.T) {
	t.Run("not attempted when label is missing", func(t *testing.T) {
		t.Setenv("CIWI_AGENT_LAUNCHD_LABEL", "")
		msg, err, attempted := restartAgentViaLaunchd()
		if attempted {
			t.Fatalf("expected attempted=false")
		}
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if msg != "" {
			t.Fatalf("unexpected msg: %q", msg)
		}
	})

	t.Run("attempted with launchctl path", func(t *testing.T) {
		t.Setenv("CIWI_AGENT_LAUNCHD_LABEL", "io.github.ciwi.agent")
		t.Setenv("CIWI_LAUNCHCTL_PATH", "/usr/bin/true")
		msg, err, attempted := restartAgentViaLaunchd()
		if !attempted {
			t.Fatalf("expected attempted=true")
		}
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !strings.Contains(msg, "io.github.ciwi.agent") {
			t.Fatalf("unexpected msg: %q", msg)
		}
	})
}

func TestEscapePowerShellSingleQuoted(t *testing.T) {
	if got := escapePowerShellSingleQuoted("O'Hara"); got != "O''Hara" {
		t.Fatalf("unexpected escaped string: %q", got)
	}
}
