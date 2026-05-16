package darwinupdater

import (
	"strings"
	"testing"
)

func TestRunCmdErrorIncludesOutput(t *testing.T) {
	err := runCmd("/bin/sh", "-c", "echo boom && exit 7")
	if err == nil {
		t.Fatalf("expected runCmd failure")
	}
	msg := err.Error()
	if !strings.Contains(msg, "/bin/sh -c") || !strings.Contains(msg, "boom") {
		t.Fatalf("expected error to include command and output, got %q", msg)
	}
}
