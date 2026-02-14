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

func requestAgentRestart() string {
	var serviceErr string
	switch runtime.GOOS {
	case "darwin":
		if msg, err, attempted := restartAgentViaLaunchd(); attempted {
			if err == nil {
				scheduleAgentExit()
				return msg
			}
			serviceErr = err.Error()
		}
	case "linux":
		if msg, err, attempted := restartAgentViaSystemd(); attempted {
			if err == nil {
				scheduleAgentExit()
				return msg
			}
			serviceErr = err.Error()
		}
	case "windows":
		if msg, err, attempted := restartAgentViaWindowsService(); attempted {
			if err == nil {
				scheduleAgentExit()
				return msg
			}
			serviceErr = err.Error()
		}
	}
	scheduleAgentExit()
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
	command := "ping -n 2 127.0.0.1 >NUL & sc.exe stop \"" + name + "\" >NUL & ping -n 2 127.0.0.1 >NUL & sc.exe start \"" + name + "\" >NUL"
	cmd := exec.Command("cmd.exe", "/C", command)
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("cmd.exe /C %q: %w", command, err), true
	}
	return "restart via windows service requested (" + name + ")", nil, true
}
