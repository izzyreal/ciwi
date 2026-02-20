package agent

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

var (
	agentRuntimeGOOS    = runtime.GOOS
	restartViaLaunchd   = restartAgentViaLaunchd
	restartViaSystemd   = restartAgentViaSystemd
	restartViaWinSvc    = restartAgentViaWindowsService
	scheduleAgentExitFn = scheduleAgentExit
)

func requestAgentRestart() string {
	var serviceErr string
	switch agentRuntimeGOOS {
	case "darwin":
		if msg, err, attempted := restartViaLaunchd(); attempted {
			if err == nil {
				scheduleAgentExitFn()
				return msg
			}
			serviceErr = err.Error()
		}
	case "linux":
		if msg, err, attempted := restartViaSystemd(); attempted {
			if err == nil {
				scheduleAgentExitFn()
				return msg
			}
			serviceErr = err.Error()
		}
	case "windows":
		if msg, err, attempted := restartViaWinSvc(); attempted {
			if err == nil {
				scheduleAgentExitFn()
				return msg
			}
			serviceErr = err.Error()
		}
	}
	scheduleAgentExitFn()
	if strings.TrimSpace(serviceErr) != "" {
		return "service restart failed; fallback exit requested: " + serviceErr
	}
	return "service restart unavailable; fallback exit requested"
}

func scheduleAgentExit() {
	go func() {
		time.Sleep(250 * time.Millisecond)
		os.Exit(0)
	}()
}

func restartAgentViaLaunchd() (string, error, bool) {
	label := strings.TrimSpace(envOrDefault("CIWI_AGENT_LAUNCHD_LABEL", ""))
	if label == "" {
		return "", nil, false
	}
	domain := "gui/" + strconv.Itoa(os.Getuid())
	service := domain + "/" + label
	if err := runLaunchctl("kickstart", "-k", service); err != nil {
		return "", fmt.Errorf("launchctl kickstart %s: %w", service, err), true
	}
	return "restart via launchctl requested (" + service + ")", nil, true
}

func restartAgentViaSystemd() (string, error, bool) {
	service := strings.TrimSpace(envOrDefault("CIWI_AGENT_SYSTEMD_SERVICE_NAME", "ciwi-agent.service"))
	if service == "" {
		return "", nil, false
	}
	systemctlPath := strings.TrimSpace(envOrDefault("CIWI_SYSTEMCTL_PATH", "/bin/systemctl"))
	if systemctlPath == "" {
		systemctlPath = "/bin/systemctl"
	}
	cmd := exec.Command(systemctlPath, "restart", service)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s restart %s: %w (%s)", systemctlPath, service, err, strings.TrimSpace(string(out))), true
	}
	return "restart via systemd requested (" + service + ")", nil, true
}

func restartAgentViaWindowsService() (string, error, bool) {
	active, name := windowsServiceInfo()
	name = strings.TrimSpace(name)
	if !active || name == "" {
		return "", nil, false
	}

	// Run restart logic in a detached process so it survives this service instance
	// being stopped.
	psName := escapePowerShellSingleQuoted(name)
	script := "$name='" + psName + "'; " +
		"sc.exe stop \"$name\" *> $null; " +
		"$deadline=(Get-Date).AddSeconds(45); " +
		"do { $q = sc.exe query \"$name\" 2>$null; " +
		"if ($q -match 'STATE\\s+:\\s+1\\s+STOPPED') { break }; " +
		"Start-Sleep -Seconds 1 } while ((Get-Date) -lt $deadline); " +
		"sc.exe start \"$name\" *> $null"

	cmd := exec.Command(
		"cmd.exe",
		"/C",
		"start",
		"\"\"",
		"/min",
		"powershell.exe",
		"-NoProfile",
		"-NonInteractive",
		"-ExecutionPolicy",
		"Bypass",
		"-Command",
		script,
	)
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("start detached windows restart helper for %q: %w", name, err), true
	}
	return "restart via windows service requested (" + name + ")", nil, true
}

func escapePowerShellSingleQuoted(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}
