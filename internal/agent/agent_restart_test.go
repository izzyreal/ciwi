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
	origExitCode := scheduleAgentExitWithCodeFn
	t.Cleanup(func() {
		agentRuntimeGOOS = origGOOS
		restartViaLaunchd = origLaunchd
		restartViaSystemd = origSystemd
		restartViaWinSvc = origWinSvc
		scheduleAgentExitFn = origExit
		scheduleAgentExitWithCodeFn = origExitCode
	})

	exitCalls := 0
	scheduleAgentExitFn = func() { exitCalls++ }
	scheduleAgentExitWithCodeFn = func(int) {}

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

func TestRequestAgentRestartWindowsSuccessUsesCleanExit(t *testing.T) {
	origGOOS := agentRuntimeGOOS
	origWinSvc := restartViaWinSvc
	origExit := scheduleAgentExitFn
	origExitCode := scheduleAgentExitWithCodeFn
	t.Cleanup(func() {
		agentRuntimeGOOS = origGOOS
		restartViaWinSvc = origWinSvc
		scheduleAgentExitFn = origExit
		scheduleAgentExitWithCodeFn = origExitCode
	})

	agentRuntimeGOOS = "windows"
	restartViaWinSvc = func() (string, error, bool) {
		return "restart via windows service requested (ciwi-agent)", nil, true
	}
	exitCalls := 0
	scheduleAgentExitFn = func() { exitCalls++ }
	exitCodeCalls := 0
	scheduleAgentExitWithCodeFn = func(int) { exitCodeCalls++ }

	msg := requestAgentRestart()
	if !strings.Contains(msg, "restart via windows service requested (ciwi-agent)") {
		t.Fatalf("unexpected message: %q", msg)
	}
	if exitCalls != 1 {
		t.Fatalf("expected clean exit scheduling once, got %d", exitCalls)
	}
	if exitCodeCalls != 0 {
		t.Fatalf("expected no non-zero exit scheduling on successful helper launch, got %d", exitCodeCalls)
	}
}

func TestRequestAgentRestartWindowsFallbackUsesNonZeroExit(t *testing.T) {
	origGOOS := agentRuntimeGOOS
	origWinSvc := restartViaWinSvc
	origExit := scheduleAgentExitFn
	origExitCode := scheduleAgentExitWithCodeFn
	t.Cleanup(func() {
		agentRuntimeGOOS = origGOOS
		restartViaWinSvc = origWinSvc
		scheduleAgentExitFn = origExit
		scheduleAgentExitWithCodeFn = origExitCode
	})

	agentRuntimeGOOS = "windows"
	restartViaWinSvc = func() (string, error, bool) {
		return "", errors.New("helper launch failed"), true
	}
	cleanExitCalls := 0
	scheduleAgentExitFn = func() { cleanExitCalls++ }
	exitCode := -1
	scheduleAgentExitWithCodeFn = func(code int) { exitCode = code }

	msg := requestAgentRestart()
	if !strings.Contains(msg, "service restart failed; fallback exit requested: helper launch failed") {
		t.Fatalf("unexpected message: %q", msg)
	}
	if cleanExitCalls != 0 {
		t.Fatalf("expected no clean-exit scheduling on fallback, got %d", cleanExitCalls)
	}
	if exitCode != windowsServiceRestartExitCode {
		t.Fatalf("expected fallback windows restart exit code %d, got %d", windowsServiceRestartExitCode, exitCode)
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
	t.Setenv("INVOCATION_ID", "")
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

func TestRestartAgentViaSystemdServiceContextUsesSelfExitPath(t *testing.T) {
	t.Setenv("CIWI_AGENT_SYSTEMD_SERVICE_NAME", "ciwi-agent.service")
	t.Setenv("CIWI_SYSTEMCTL_PATH", "/definitely/missing/systemctl")
	t.Setenv("INVOCATION_ID", "abc123")
	msg, err, attempted := restartAgentViaSystemd()
	if !attempted {
		t.Fatalf("expected attempted=true")
	}
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !strings.Contains(msg, "self-exit requested (ciwi-agent.service)") {
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

func TestRestartAgentViaWindowsServiceBranches(t *testing.T) {
	origInfo := windowsServiceInfoFn
	origStart := startWindowsServiceRestartHelperFn
	t.Cleanup(func() {
		windowsServiceInfoFn = origInfo
		startWindowsServiceRestartHelperFn = origStart
	})

	t.Run("not attempted when service is inactive", func(t *testing.T) {
		windowsServiceInfoFn = func() (bool, string) { return false, "" }
		startWindowsServiceRestartHelperFn = func(string) error {
			t.Fatalf("helper launcher should not be called when service is inactive")
			return nil
		}

		msg, err, attempted := restartAgentViaWindowsService()
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

	t.Run("attempted helper error", func(t *testing.T) {
		windowsServiceInfoFn = func() (bool, string) { return true, "ciwi-agent" }
		startWindowsServiceRestartHelperFn = func(string) error { return errors.New("spawn failed") }

		msg, err, attempted := restartAgentViaWindowsService()
		if !attempted {
			t.Fatalf("expected attempted=true")
		}
		if err == nil || !strings.Contains(err.Error(), "spawn failed") {
			t.Fatalf("expected spawn failure, got %v", err)
		}
		if msg != "" {
			t.Fatalf("unexpected msg on error: %q", msg)
		}
	})

	t.Run("attempted helper success", func(t *testing.T) {
		windowsServiceInfoFn = func() (bool, string) { return true, "ciwi-agent" }
		launched := ""
		startWindowsServiceRestartHelperFn = func(name string) error {
			launched = name
			return nil
		}

		msg, err, attempted := restartAgentViaWindowsService()
		if !attempted {
			t.Fatalf("expected attempted=true")
		}
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if launched != "ciwi-agent" {
			t.Fatalf("expected helper launch for ciwi-agent, got %q", launched)
		}
		if !strings.Contains(msg, "restart via windows service requested (ciwi-agent)") {
			t.Fatalf("unexpected msg: %q", msg)
		}
	})
}

func TestBuildWindowsServiceRestartHelperCommand(t *testing.T) {
	origPowerShell := windowsPowerShellCommandFn
	t.Cleanup(func() {
		windowsPowerShellCommandFn = origPowerShell
	})

	windowsPowerShellCommandFn = func() string { return `C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe` }
	cmd := buildWindowsServiceRestartHelperCommand("ciwi-agent")
	if cmd == nil {
		t.Fatalf("expected command")
	}
	if cmd.Path != `C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe` {
		t.Fatalf("unexpected powershell path: %q", cmd.Path)
	}
	if len(cmd.Args) < 7 {
		t.Fatalf("unexpected args: %#v", cmd.Args)
	}
	if cmd.Args[1] != "-NoProfile" || cmd.Args[2] != "-NonInteractive" || cmd.Args[3] != "-ExecutionPolicy" || cmd.Args[4] != "Bypass" || cmd.Args[5] != "-Command" {
		t.Fatalf("unexpected prologue args: %#v", cmd.Args)
	}
	if !strings.Contains(cmd.Args[6], "Stop-Service -Name $name") {
		t.Fatalf("expected restart script payload, got %q", cmd.Args[6])
	}
}

func TestEscapePowerShellSingleQuoted(t *testing.T) {
	if got := escapePowerShellSingleQuoted("O'Hara"); got != "O''Hara" {
		t.Fatalf("unexpected escaped string: %q", got)
	}
}

func TestWindowsServiceRestartScriptIncludesRobustRestartFlow(t *testing.T) {
	script := windowsServiceRestartScript("ciwi-agent-O'Hara")
	if !strings.Contains(script, "$name='ciwi-agent-O''Hara'") {
		t.Fatalf("expected escaped service name in script, got %q", script)
	}
	if !strings.Contains(script, "Stop-Service -Name $name") {
		t.Fatalf("expected Stop-Service in script, got %q", script)
	}
	if !strings.Contains(script, "WaitForStatus([System.ServiceProcess.ServiceControllerStatus]::Stopped") {
		t.Fatalf("expected WaitForStatus stop wait in script, got %q", script)
	}
	if !strings.Contains(script, "Start-Service -Name $name") {
		t.Fatalf("expected Start-Service in script, got %q", script)
	}
	if !strings.Contains(script, "sc.exe start \"$name\"") {
		t.Fatalf("expected sc.exe fallback start in script, got %q", script)
	}
}
