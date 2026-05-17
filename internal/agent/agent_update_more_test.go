package agent

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestAgentUpdateUtilityWrappers(t *testing.T) {
	if got := expectedAssetName("linux", "amd64"); strings.TrimSpace(got) == "" {
		t.Fatalf("expected known asset name for linux/amd64")
	}
	if got := expectedAssetName("unknown", "unknown"); got != "" {
		t.Fatalf("expected empty asset name for unknown platform, got %q", got)
	}

	if !isVersionNewer("v1.1.0", "v1.0.0") {
		t.Fatalf("expected v1.1.0 to be newer than v1.0.0")
	}
	if isVersionNewer("v1.0.0", "v1.0.0") {
		t.Fatalf("did not expect equal versions to be newer")
	}
	if !isVersionDifferent("v1.0.1", "v1.0.0") {
		t.Fatalf("expected different versions")
	}
	if isVersionDifferent("v1.0.0", "v1.0.0") {
		t.Fatalf("did not expect equal versions to differ")
	}
	if !looksLikeGoRunBinary("/tmp/go-build1234/b001/exe/main") {
		t.Fatalf("expected go-run path detection")
	}

	tmp := t.TempDir()
	src := filepath.Join(tmp, "src.bin")
	dst := filepath.Join(tmp, "dst.bin")
	if err := os.WriteFile(src, []byte("copy"), 0o644); err != nil {
		t.Fatalf("write src: %v", err)
	}
	if err := copyFile(src, dst, 0o700); err != nil {
		t.Fatalf("copyFile: %v", err)
	}
	raw, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read dst: %v", err)
	}
	if string(raw) != "copy" {
		t.Fatalf("unexpected copied content: %q", string(raw))
	}
}

func TestRunLaunchctl(t *testing.T) {
	t.Setenv("CIWI_LAUNCHCTL_PATH", "/usr/bin/true")
	if err := runLaunchctl("kickstart", "-k", "dummy/service"); err != nil {
		t.Fatalf("runLaunchctl with /usr/bin/true: %v", err)
	}
	t.Setenv("CIWI_LAUNCHCTL_PATH", "/usr/bin/false")
	if err := runLaunchctl("kickstart", "-k", "dummy/service"); err == nil {
		t.Fatalf("expected runLaunchctl failure with /usr/bin/false")
	}
}

func TestDarwinUpdaterConfigGates(t *testing.T) {
	t.Setenv("CIWI_AGENT_LAUNCHD_LABEL", "")
	t.Setenv("CIWI_AGENT_LAUNCHD_PLIST", "")
	if err := stageAndTriggerDarwinUpdater("v1.0.0", "asset", "/tmp/target", "/tmp/staged"); err == nil || !strings.Contains(err.Error(), "missing launchd agent configuration") {
		t.Fatalf("expected missing launchd config error, got %v", err)
	}

	// Non-darwin runtimes should never advertise launchd updater readiness.
	if runtime.GOOS != "darwin" {
		t.Setenv("CIWI_AGENT_LAUNCHD_LABEL", "agent")
		t.Setenv("CIWI_AGENT_LAUNCHD_PLIST", "/tmp/agent.plist")
		if hasDarwinUpdaterConfig() {
			t.Fatalf("expected hasDarwinUpdaterConfig=false on non-darwin")
		}
	}
	if runtime.GOOS == "darwin" {
		t.Setenv("CIWI_AGENT_LAUNCHD_LABEL", "agent")
		t.Setenv("CIWI_AGENT_LAUNCHD_PLIST", "/tmp/agent.plist")
		if !hasDarwinUpdaterConfig() {
			t.Fatalf("expected hasDarwinUpdaterConfig=true on darwin with launchd env")
		}
	}
}

func TestFindAppBundleRoot(t *testing.T) {
	got := findAppBundleRoot("/tmp/CiwiAgent.app/Contents/MacOS/ciwi")
	if got != "/tmp/CiwiAgent.app" {
		t.Fatalf("unexpected bundle root: %q", got)
	}
	if got := findAppBundleRoot("/tmp/ciwi"); got != "" {
		t.Fatalf("expected empty bundle root, got %q", got)
	}
}

func TestStageAndTriggerDarwinUpdaterStartsDetachedHelper(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("darwin-specific launchd updater staging")
	}

	tmp := t.TempDir()
	targetBundle := filepath.Join(tmp, "installed", "CiwiAgent.app")
	targetBinary := filepath.Join(targetBundle, "Contents", "MacOS", "ciwi")
	stagedBundle := filepath.Join(tmp, "downloaded", "CiwiAgent.app")
	stagedBinary := filepath.Join(stagedBundle, "Contents", "MacOS", "ciwi")
	manifestPath := filepath.Join(tmp, "updates", "pending.json")

	if err := os.MkdirAll(filepath.Dir(targetBinary), 0o755); err != nil {
		t.Fatalf("mkdir target binary dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(stagedBinary), 0o755); err != nil {
		t.Fatalf("mkdir staged binary dir: %v", err)
	}
	if err := os.WriteFile(targetBinary, []byte("installed"), 0o755); err != nil {
		t.Fatalf("write target binary: %v", err)
	}
	if err := os.WriteFile(stagedBinary, []byte("staged"), 0o755); err != nil {
		t.Fatalf("write staged binary: %v", err)
	}

	t.Setenv("CIWI_AGENT_LAUNCHD_LABEL", "nl.izmar.ciwi.agent")
	t.Setenv("CIWI_AGENT_LAUNCHD_PLIST", filepath.Join(tmp, "nl.izmar.ciwi.agent.plist"))
	t.Setenv("CIWI_AGENT_APP_BUNDLE", targetBundle)
	t.Setenv("CIWI_AGENT_UPDATE_MANIFEST", manifestPath)

	origStartDarwinUpdater := agentStartDarwinUpdaterFn
	defer func() { agentStartDarwinUpdaterFn = origStartDarwinUpdater }()

	var gotHelperPath string
	var gotManifestPath string
	agentStartDarwinUpdaterFn = func(helperPath, manifestPath string) error {
		gotHelperPath = helperPath
		gotManifestPath = manifestPath
		return nil
	}

	if err := stageAndTriggerDarwinUpdater("v1.0.0", "ciwi-darwin-arm64.zip", targetBinary, stagedBinary); err != nil {
		t.Fatalf("stageAndTriggerDarwinUpdater: %v", err)
	}
	if gotHelperPath == "" || gotManifestPath == "" {
		t.Fatalf("expected detached helper launch, got helper=%q manifest=%q", gotHelperPath, gotManifestPath)
	}
	if gotManifestPath != manifestPath {
		t.Fatalf("unexpected manifest path: got %q want %q", gotManifestPath, manifestPath)
	}
	if filepath.Base(gotHelperPath) != "ciwi" {
		t.Fatalf("unexpected helper executable name: %q", gotHelperPath)
	}
	wantHelperPath := filepath.Join(filepath.Dir(manifestPath), "CiwiAgent.app", "Contents", "MacOS", "ciwi")
	if gotHelperPath != wantHelperPath {
		t.Fatalf("expected staged bundle binary as helper, got %q want %q", gotHelperPath, wantHelperPath)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(manifestPath), "CiwiAgent.app", "Contents", "MacOS", "ciwi")); err != nil {
		t.Fatalf("expected staged bundle in update dir: %v", err)
	}
	if _, err := os.Stat(manifestPath); err != nil {
		t.Fatalf("expected manifest written: %v", err)
	}
}
