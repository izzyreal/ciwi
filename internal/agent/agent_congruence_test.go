package agent

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestRuntimeHostToolProbeAndValidationAreAligned(t *testing.T) {
	shell := defaultShellForRuntime()
	binDir := t.TempDir()
	toolName := "ciwi_probe_tool"
	toolPath := filepath.Join(binDir, toolName)
	if runtime.GOOS == "windows" {
		toolPath += ".bat"
		if err := os.WriteFile(toolPath, []byte("@echo ciwi_probe_tool version 9.8.7\r\n"), 0o755); err != nil {
			t.Fatalf("write fake tool: %v", err)
		}
	} else {
		if err := os.WriteFile(toolPath, []byte("#!/bin/sh\necho 'ciwi_probe_tool version 9.8.7'\n"), 0o755); err != nil {
			t.Fatalf("write fake tool: %v", err)
		}
	}

	originalPath := os.Getenv("PATH")
	sep := string(os.PathListSeparator)
	if originalPath == "" {
		t.Setenv("PATH", binDir)
	} else {
		t.Setenv("PATH", binDir+sep+originalPath)
	}

	required := map[string]string{
		"requires.tool." + toolName: ">=9.0.0",
	}
	runtimeCaps := map[string]string{}

	enrichRuntimeHostToolCapabilities(runtimeCaps, required, shell)
	if got := runtimeCaps["host.tool."+toolName]; got == "" {
		t.Fatalf("expected runtime host capability for %q, got empty", toolName)
	}
	if err := validateHostToolRequirements(required, runtimeCaps); err != nil {
		t.Fatalf("expected host tool requirements to pass, got %v", err)
	}
}
