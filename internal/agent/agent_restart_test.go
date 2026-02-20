package agent

import (
	"strings"
	"testing"
)

func TestRestartAgentViaSystemd(t *testing.T) {
	t.Setenv("CIWI_AGENT_SYSTEMD_SERVICE_NAME", "ciwi-agent.service")
	t.Setenv("CIWI_SYSTEMCTL_PATH", "/usr/bin/true")
	msg, err, attempted := restartAgentViaSystemd()
	if !attempted || err != nil {
		t.Fatalf("expected systemd restart success, attempted=%v err=%v", attempted, err)
	}
	if !strings.Contains(msg, "ciwi-agent.service") {
		t.Fatalf("unexpected restart message: %q", msg)
	}

	t.Setenv("CIWI_SYSTEMCTL_PATH", "/usr/bin/false")
	_, err, attempted = restartAgentViaSystemd()
	if !attempted || err == nil {
		t.Fatalf("expected systemd restart failure, attempted=%v err=%v", attempted, err)
	}
}

func TestRestartAgentViaLaunchdAndPowerShellEscape(t *testing.T) {
	t.Setenv("CIWI_AGENT_LAUNCHD_LABEL", "")
	_, err, attempted := restartAgentViaLaunchd()
	if attempted || err != nil {
		t.Fatalf("expected no launchd attempt when label missing, attempted=%v err=%v", attempted, err)
	}

	t.Setenv("CIWI_AGENT_LAUNCHD_LABEL", "io.github.ciwi.agent")
	t.Setenv("CIWI_LAUNCHCTL_PATH", "/usr/bin/true")
	msg, err, attempted := restartAgentViaLaunchd()
	if !attempted || err != nil {
		t.Fatalf("expected launchd restart success, attempted=%v err=%v", attempted, err)
	}
	if !strings.Contains(msg, "io.github.ciwi.agent") {
		t.Fatalf("unexpected launchd message: %q", msg)
	}

	t.Setenv("CIWI_LAUNCHCTL_PATH", "/usr/bin/false")
	_, err, attempted = restartAgentViaLaunchd()
	if !attempted || err == nil {
		t.Fatalf("expected launchd restart failure, attempted=%v err=%v", attempted, err)
	}

	if got := escapePowerShellSingleQuoted("a'b''c"); got != "a''b''''c" {
		t.Fatalf("unexpected PowerShell single-quote escaping: %q", got)
	}
}
