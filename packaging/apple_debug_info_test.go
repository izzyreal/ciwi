//go:build integration

package packaging_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestAppleDebugInfoVerifierAcceptsRelWithDebInfoAndRejectsStrippedBinary(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Fatal("Apple debug-info integration requires macOS and the Xcode command-line tools")
	}

	for _, tool := range []string{"go", "xcrun", "lipo", "sh", "cp"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Fatalf("Apple debug-info integration requires %s: %v", tool, err)
		}
	}

	architecture := runtime.GOARCH
	if architecture == "amd64" {
		architecture = "x86_64"
	}
	temporaryDirectory := t.TempDir()
	source := filepath.Join(temporaryDirectory, "main.go")
	if err := os.WriteFile(source, []byte("package main\nfunc main() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	debugBinary := filepath.Join(temporaryDirectory, "debug-app")
	run(t, "go", "build", "-trimpath", "-ldflags=-compressdwarf=false", "-o", debugBinary, source)
	dsym := debugBinary + ".dSYM"
	run(t, "xcrun", "dsymutil", "--quiet", debugBinary, "-o", dsym)
	dsymBinary := filepath.Join(dsym, "Contents", "Resources", "DWARF", filepath.Base(debugBinary))
	run(t, "cp", debugBinary, dsymBinary)
	run(t, "sh", "verify-apple-debug-info.sh", debugBinary, architecture, "main.main", dsym)

	strippedBinary := filepath.Join(temporaryDirectory, "stripped-app")
	run(t, "go", "build", "-trimpath", "-ldflags=-s -w", "-o", strippedBinary, source)
	command := exec.Command("sh", "verify-apple-debug-info.sh", strippedBinary, architecture, "main.main")
	if output, err := command.CombinedOutput(); err == nil {
		t.Fatalf("stripped binary unexpectedly passed verification:\n%s", output)
	}
}

func run(t *testing.T, name string, arguments ...string) {
	t.Helper()
	command := exec.Command(name, arguments...)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("%s %v: %v\n%s", name, arguments, err, output)
	}
}
