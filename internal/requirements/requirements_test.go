package requirements

import "testing"

func TestShellCapabilityMatch(t *testing.T) {
	caps := map[string]string{
		"shells": "posix, powershell",
	}
	if !ShellCapabilityMatch(caps, "posix") {
		t.Fatal("expected posix shell to match")
	}
	if !ShellCapabilityMatch(caps, "PowerShell") {
		t.Fatal("expected case-insensitive shell match")
	}
	if ShellCapabilityMatch(caps, "cmd") {
		t.Fatal("did not expect cmd shell to match")
	}
}

func TestToolConstraintMatch(t *testing.T) {
	if !ToolConstraintMatch("1.24.1", ">=1.24.0") {
		t.Fatal("expected semver constraint to match")
	}
	if ToolConstraintMatch("1.23.9", ">=1.24.0") {
		t.Fatal("expected semver constraint to fail")
	}
	if !ToolConstraintMatch("go1.24", "go1.24") {
		t.Fatal("expected literal equality constraint to match")
	}
	if ToolConstraintMatch("go1.23", ">=go1.24") {
		t.Fatal("expected invalid semver inequality constraint to fail")
	}
}

func TestMatchAgentRequiresOneAgentToSatisfyEverything(t *testing.T) {
	required := map[string]string{
		"os":                "linux",
		"arch":              "amd64",
		"shell":             "posix",
		"requires.tool.go":  ">=1.24.0",
		"requires.tool.git": "*",
		"agent_id":          "agent-a",
	}

	agent := AgentSnapshot{
		ID:   "agent-a",
		OS:   "linux",
		Arch: "amd64",
		Capabilities: map[string]string{
			"shells":   "posix,cmd",
			"tool.go":  "1.24.1",
			"tool.git": "2.49.0",
		},
	}
	if result := MatchAgent(required, agent); !result.Matches {
		t.Fatalf("expected requirements to match, got %v", result.Issues)
	}
}

func TestDiagnoseSchedulingDoesNotCombineDifferentAgents(t *testing.T) {
	diagnosis := DiagnoseScheduling(map[string]string{"os": "windows", "requires.tool.wix": ">=6.0.0"}, []AgentSnapshot{
		{ID: "windows-without-wix", OS: "windows", Freshness: "online", Authorized: true},
		{ID: "linux-with-wix", OS: "linux", Freshness: "online", Authorized: true, Capabilities: map[string]string{"tool.wix": "6.0.2"}},
	})
	if diagnosis.State != DiagnosisIncompatible {
		t.Fatalf("state = %q, want %q: %+v", diagnosis.State, DiagnosisIncompatible, diagnosis)
	}
}

func TestDiagnoseSchedulingAvailabilityAndContainerTools(t *testing.T) {
	required := map[string]string{
		"os":                             "linux",
		"arch":                           "amd64",
		"shell":                          "posix",
		"requires.tool.docker":           "*",
		"requires.container.tool.cmake":  "*",
		"requires.container.tool.ninja":  "*",
		"requires.container.tool.ccache": "*",
	}

	diagnosis := DiagnoseScheduling(required, []AgentSnapshot{
		{
			ID:   "agent-a",
			OS:   "linux",
			Arch: "amd64",
			Capabilities: map[string]string{
				"shells":      "posix",
				"tool.docker": "28.0.0",
			},
			Freshness: "online", Authorized: true, Busy: true,
		},
	})
	if diagnosis.State != DiagnosisWaiting || len(diagnosis.Agents) != 1 || !diagnosis.Agents[0].CapabilityMatch {
		t.Fatalf("unexpected diagnosis: %+v", diagnosis)
	}
	if got := diagnosis.Agents[0].AvailabilityIssues; len(got) != 1 || got[0] != "busy" {
		t.Fatalf("availability issues = %v", got)
	}
}

func TestDiagnoseSchedulingNoAgentsAndReadyAgent(t *testing.T) {
	missing := DiagnoseScheduling(map[string]string{"os": "linux"}, nil)
	if missing.Summary != "No agents are registered" || missing.State != DiagnosisIncompatible {
		t.Fatalf("unexpected empty-fleet diagnosis: %+v", missing)
	}
	ready := DiagnoseScheduling(map[string]string{"os": "linux"}, []AgentSnapshot{{
		ID: "agent-a", OS: "linux", Freshness: "online", Authorized: true,
	}})
	if ready.State != DiagnosisReady || ready.Summary == "" {
		t.Fatalf("unexpected ready diagnosis: %+v", ready)
	}
}
