package agent

import "testing"

func TestRuntimeProbeSummary(t *testing.T) {
	if got := runtimeProbeSummary(nil); got != "" {
		t.Fatalf("expected empty summary for nil map, got %q", got)
	}
	got := runtimeProbeSummary(map[string]string{
		"host.tool.git":        "2.39.5",
		"host.tool.go":         "1.26.0",
		"container.tool.cmake": "3.31.5",
		"container.name":       "ubuntu-vmpc",
	})
	if got != "[runtime] host_tools=2 container_tools=1" {
		t.Fatalf("unexpected runtimeProbeSummary: %q", got)
	}
}

func TestContainerToolRequirements(t *testing.T) {
	reqs := containerToolRequirements(map[string]string{
		"requires.container.tool.cmake":  ">=3.20.0",
		"requires.container.tool.ccache": "*",
		"requires.tool.git":              "*",
	})
	if len(reqs) != 2 {
		t.Fatalf("expected 2 container tool requirements, got %d (%v)", len(reqs), reqs)
	}
	if reqs["cmake"] != ">=3.20.0" || reqs["ccache"] != "*" {
		t.Fatalf("unexpected requirement extraction: %v", reqs)
	}
}

func TestRuntimeProbeMetadataReaders(t *testing.T) {
	meta := map[string]string{
		"runtime_probe.container_image":  " ubuntu-vmpc ",
		"runtime_exec.container_workdir": " /workspace ",
		"runtime_exec.container_user":    " 1000:1000 ",
	}
	if got := runtimeProbeContainerImageFromMetadata(meta); got != "ubuntu-vmpc" {
		t.Fatalf("unexpected container image: %q", got)
	}
	if got := runtimeExecContainerWorkdirFromMetadata(meta); got != "/workspace" {
		t.Fatalf("unexpected container workdir: %q", got)
	}
	if got := runtimeExecContainerUserFromMetadata(meta); got != "1000:1000" {
		t.Fatalf("unexpected container user: %q", got)
	}
	if got := runtimeProbeContainerName("job-123", meta); got == "" {
		t.Fatalf("expected derived container name")
	}
	if got := runtimeProbeContainerName("job-123", map[string]string{}); got != "" {
		t.Fatalf("expected empty name without image metadata, got %q", got)
	}
}

func TestShellQuoteAndShortStableID(t *testing.T) {
	if got := shellQuote("plain"); got != "plain" {
		t.Fatalf("unexpected plain quote result: %q", got)
	}
	if got := shellQuote("a b"); got != "'a b'" {
		t.Fatalf("unexpected quoted result: %q", got)
	}
	if got := shellQuote("it's"); got == "it's" {
		t.Fatalf("expected escaping for single quote, got %q", got)
	}
	if shortStableID("same") != shortStableID("same") {
		t.Fatalf("shortStableID should be deterministic")
	}
	if shortStableID("") != "unknown" {
		t.Fatalf("expected unknown for empty shortStableID input")
	}
}
