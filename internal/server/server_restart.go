package server

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

type serverRestartResponse struct {
	Restarting bool   `json:"restarting"`
	Message    string `json:"message,omitempty"`
}

func (s *stateStore) serverRestartHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	message := s.requestServerRestart()
	_ = s.persistUpdateStatus(map[string]string{
		"update_message": message,
	})
	writeJSON(w, http.StatusOK, serverRestartResponse{
		Restarting: true,
		Message:    message,
	})
}

func (s *stateStore) requestServerRestart() string {
	if msg, err, attempted := restartServerViaService(); attempted {
		if err == nil {
			return msg
		}
		go func(restartFn func()) {
			time.Sleep(250 * time.Millisecond)
			if restartFn != nil {
				restartFn()
			}
		}(s.restartServerFn)
		return "service restart failed; fallback exit requested: " + err.Error()
	}
	go func(restartFn func()) {
		time.Sleep(250 * time.Millisecond)
		if restartFn != nil {
			restartFn()
		}
	}(s.restartServerFn)
	return "service restart unavailable; fallback exit requested"
}

func restartServerViaService() (string, error, bool) {
	switch runtime.GOOS {
	case "linux":
		return restartServerViaSystemd()
	case "darwin":
		return restartServerViaLaunchd()
	case "windows":
		return restartServerViaWindowsService()
	default:
		return "", nil, false
	}
}

func restartServerViaSystemd() (string, error, bool) {
	service := strings.TrimSpace(envOrDefault("CIWI_SERVER_SERVICE_NAME", "ciwi.service"))
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

func restartServerViaLaunchd() (string, error, bool) {
	label := strings.TrimSpace(envOrDefault("CIWI_SERVER_LAUNCHD_LABEL", ""))
	if label == "" {
		return "", nil, false
	}
	domain := strings.TrimSpace(envOrDefault("CIWI_SERVER_LAUNCHD_DOMAIN", "system"))
	if domain == "" {
		domain = "system"
	}
	if domain == "gui" {
		domain = "gui/" + strconv.Itoa(os.Getuid())
	}
	service := domain + "/" + label
	launchctlPath := strings.TrimSpace(envOrDefault("CIWI_LAUNCHCTL_PATH", "/bin/launchctl"))
	if launchctlPath == "" {
		launchctlPath = "/bin/launchctl"
	}
	cmd := exec.Command(launchctlPath, "kickstart", "-k", service)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s kickstart -k %s: %w (%s)", launchctlPath, service, err, strings.TrimSpace(string(out))), true
	}
	return "restart via launchctl requested (" + service + ")", nil, true
}

func restartServerViaWindowsService() (string, error, bool) {
	name := strings.TrimSpace(envOrDefault("CIWI_SERVER_WINDOWS_SERVICE_NAME", ""))
	if name == "" {
		return "", nil, false
	}
	command := "ping -n 2 127.0.0.1 >NUL & sc.exe stop \"" + name + "\" >NUL & ping -n 2 127.0.0.1 >NUL & sc.exe start \"" + name + "\" >NUL"
	cmd := exec.Command("cmd.exe", "/C", command)
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("cmd.exe /C %q: %w", command, err), true
	}
	return "restart via windows service requested (" + name + ")", nil, true
}
