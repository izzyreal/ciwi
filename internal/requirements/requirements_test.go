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

func TestDiagnoseUnmetRequirements(t *testing.T) {
	required := map[string]string{
		"os":                "linux",
		"arch":              "amd64",
		"shell":             "posix",
		"requires.tool.go":  ">=1.24.0",
		"requires.tool.git": "*",
		"agent_id":          "agent-a",
	}

	agents := []AgentSnapshot{
		{
			ID:   "agent-a",
			OS:   "linux",
			Arch: "amd64",
			Capabilities: map[string]string{
				"shells":   "posix,cmd",
				"tool.go":  "1.24.1",
				"tool.git": "2.49.0",
			},
		},
	}

	reasons := DiagnoseUnmetRequirements(required, agents)
	if len(reasons) != 0 {
		t.Fatalf("expected no unmet requirements, got %v", reasons)
	}
}

func TestDiagnoseUnmetRequirementsNoAgents(t *testing.T) {
	reasons := DiagnoseUnmetRequirements(map[string]string{"os": "linux"}, nil)
	if len(reasons) != 1 || reasons[0] != "no agents connected" {
		t.Fatalf("unexpected reasons: %v", reasons)
	}
}
