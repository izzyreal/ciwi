package darwinupdater

import (
	"os"
	"path/filepath"
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

func TestAdHocSignBinaryUsesConfigurableCodesignPath(t *testing.T) {
	tmp := t.TempDir()
	bin := filepath.Join(tmp, "agent-bin")
	if err := os.WriteFile(bin, []byte("binary"), 0o755); err != nil {
		t.Fatalf("write binary: %v", err)
	}

	t.Setenv("CIWI_CODESIGN_PATH", "/usr/bin/true")
	if err := adHocSignBinary(bin); err != nil {
		t.Fatalf("expected adHocSignBinary success with /usr/bin/true, got %v", err)
	}

	t.Setenv("CIWI_CODESIGN_PATH", "/usr/bin/false")
	if err := adHocSignBinary(bin); err == nil {
		t.Fatalf("expected adHocSignBinary failure with /usr/bin/false")
	}
}
