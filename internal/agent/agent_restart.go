package agent

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

var (
	agentRuntimeGOOS                   = runtime.GOOS
	restartViaLaunchd                  = restartAgentViaLaunchd
	restartViaSystemd                  = restartAgentViaSystemd
	restartViaWinSvc                   = restartAgentViaWindowsService
	scheduleAgentExitFn                = scheduleAgentExit
	scheduleAgentExitWithCodeFn        = scheduleAgentExitWithCode
	windowsServiceInfoFn               = windowsServiceInfo
	startWindowsServiceRestartHelperFn = startWindowsServiceRestartHelper
	windowsPowerShellCommandFn         = windowsPowerShellCommand
)

const windowsServiceRestartExitCode = 23

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
				scheduleAgentExitWithCodeFn(windowsServiceRestartExitCode)
				return msg
			}
			serviceErr = err.Error()
		}
	}
	if agentRuntimeGOOS == "windows" {
		scheduleAgentExitWithCodeFn(windowsServiceRestartExitCode)
	} else {
		scheduleAgentExitFn()
	}
	if strings.TrimSpace(serviceErr) != "" {
		return "service restart failed; fallback exit requested: " + serviceErr
	}
	return "service restart unavailable; fallback exit requested"
}

func scheduleAgentExit() {
	scheduleAgentExitWithCode(0)
}

func scheduleAgentExitWithCode(code int) {
	go func() {
		time.Sleep(250 * time.Millisecond)
		os.Exit(code)
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
	active, name := windowsServiceInfoFn()
	name = strings.TrimSpace(name)
	if !active || name == "" {
		return "", nil, false
	}

	// Run restart logic in a detached process so it survives this service instance
	// being stopped.
	if err := startWindowsServiceRestartHelperFn(name); err != nil {
		return "", fmt.Errorf("start detached windows restart helper for %q: %w", name, err), true
	}
	return "restart via windows service requested (" + name + ")", nil, true
}

func startWindowsServiceRestartHelper(name string) error {
	cmd := buildWindowsServiceRestartHelperCommand(name)
	if err := cmd.Start(); err != nil {
		return err
	}
	if cmd.Process != nil {
		_ = cmd.Process.Release()
	}
	return nil
}

func buildWindowsServiceRestartHelperCommand(name string) *exec.Cmd {
	cmd := exec.Command(
		windowsPowerShellCommandFn(),
		"-NoProfile",
		"-NonInteractive",
		"-ExecutionPolicy",
		"Bypass",
		"-Command",
		windowsServiceRestartScript(name),
	)
	prepareDetachedWindowsRestartCommand(cmd)
	return cmd
}

func windowsPowerShellCommand() string {
	roots := []string{
		strings.TrimSpace(os.Getenv("SystemRoot")),
		strings.TrimSpace(os.Getenv("WINDIR")),
		`C:\Windows`,
	}
	for _, root := range roots {
		if root == "" {
			continue
		}
		candidate := filepath.Join(root, "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return "powershell.exe"
}

func windowsServiceRestartScript(name string) string {
	psName := escapePowerShellSingleQuoted(name)
	return "$name='" + psName + "'; " +
		"try { Stop-Service -Name $name -ErrorAction SilentlyContinue } catch {} ; " +
		"try { $svc = Get-Service -Name $name -ErrorAction Stop; " +
		"if ($svc.Status -ne [System.ServiceProcess.ServiceControllerStatus]::Stopped) { " +
		"$svc.WaitForStatus([System.ServiceProcess.ServiceControllerStatus]::Stopped, [TimeSpan]::FromSeconds(45)) } } catch {} ; " +
		"$startDeadline=(Get-Date).AddSeconds(20); " +
		"do { $started=$false; " +
		"try { Start-Service -Name $name -ErrorAction Stop; $started=$true } catch { " +
		"$s = sc.exe start \"$name\" 2>&1; $txt = ($s | Out-String); " +
		"if (($LASTEXITCODE -eq 0) -or ($txt -match '1056') -or ($txt -match 'already (running|been started)')) { $started=$true } }; " +
		"if ($started) { break }; Start-Sleep -Milliseconds 500 } while ((Get-Date) -lt $startDeadline)"
}

func escapePowerShellSingleQuoted(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}
