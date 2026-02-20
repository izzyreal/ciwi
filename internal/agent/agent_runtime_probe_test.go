package agent

import (
	"strings"
	"testing"
)

func TestValidateContainerToolRequirements(t *testing.T) {
	t.Run("no requirements", func(t *testing.T) {
		if err := validateContainerToolRequirements(nil, map[string]string{"container.tool.cmake": "3.29.0"}); err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
	})

	t.Run("satisfied", func(t *testing.T) {
		required := map[string]string{
			"requires.container.tool.cmake": ">=3.20.0",
		}
		runtimeCaps := map[string]string{
			"container.tool.cmake": "3.29.1",
		}
		if err := validateContainerToolRequirements(required, runtimeCaps); err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
	})

	t.Run("missing tool", func(t *testing.T) {
		required := map[string]string{
			"requires.container.tool.ccache": "*",
		}
		err := validateContainerToolRequirements(required, map[string]string{})
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "ccache") {
			t.Fatalf("expected error mentioning ccache, got %q", err.Error())
		}
	})

	t.Run("constraint mismatch", func(t *testing.T) {
		required := map[string]string{
			"requires.container.tool.cmake": ">=3.30.0",
		}
		runtimeCaps := map[string]string{
			"container.tool.cmake": "3.29.1",
		}
		err := validateContainerToolRequirements(required, runtimeCaps)
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "does not satisfy >=3.30.0") {
			t.Fatalf("unexpected error: %q", err.Error())
		}
	})
}

func TestRuntimeExecContainerDevicesFromMetadata(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		got := runtimeExecContainerDevicesFromMetadata(nil)
		if len(got) != 0 {
			t.Fatalf("expected no devices, got %v", got)
		}
	})

	t.Run("split and dedupe", func(t *testing.T) {
		got := runtimeExecContainerDevicesFromMetadata(map[string]string{
			"runtime_exec.container_devices": " /dev/snd, /dev/snd , /dev/dri ",
		})
		if len(got) != 2 {
			t.Fatalf("expected 2 devices, got %d (%v)", len(got), got)
		}
		if got[0] != "/dev/snd" || got[1] != "/dev/dri" {
			t.Fatalf("unexpected devices order/content: %v", got)
		}
	})
}

func TestRuntimeExecContainerGroupsFromMetadata(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		got := runtimeExecContainerGroupsFromMetadata(nil)
		if len(got) != 0 {
			t.Fatalf("expected no groups, got %v", got)
		}
	})

	t.Run("split and dedupe", func(t *testing.T) {
		got := runtimeExecContainerGroupsFromMetadata(map[string]string{
			"runtime_exec.container_groups": " audio, audio , render ",
		})
		if len(got) != 2 {
			t.Fatalf("expected 2 groups, got %d (%v)", len(got), got)
		}
		if got[0] != "audio" || got[1] != "render" {
			t.Fatalf("unexpected groups order/content: %v", got)
		}
	})
}
