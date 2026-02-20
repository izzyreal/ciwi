package agent

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func writeFakeDocker(t *testing.T, scriptBody string) (binDir string, logPath string) {
	t.Helper()
	binDir = t.TempDir()
	logPath = filepath.Join(binDir, "docker.log")
	dockerPath := filepath.Join(binDir, "docker")
	content := "#!/bin/sh\n" +
		"echo \"$@\" >> \"$CIWI_DOCKER_LOG\"\n" +
		scriptBody + "\n"
	if err := os.WriteFile(dockerPath, []byte(content), 0o755); err != nil {
		t.Fatalf("write fake docker: %v", err)
	}
	return binDir, logPath
}

func TestRuntimeContainerLifecycleHelpers(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake docker shell script tests are posix-only")
	}

	binDir, logPath := writeFakeDocker(t, `
if [ "$1" = "inspect" ]; then
  if [ "${CIWI_DOCKER_INSPECT_RUNNING:-true}" = "true" ]; then
    echo true
  else
    echo false
  fi
  exit 0
fi
if [ "$1" = "exec" ]; then
  case "$5" in
    *"cmake --version"*) echo "cmake version 3.31.5"; exit 0 ;;
    *"ninja --version"*) echo "1.11.1"; exit 0 ;;
    *"go version"*) echo "go version go1.26.0 linux/amd64"; exit 0 ;;
    *) exit 1 ;;
  esac
fi
exit 0
`)
	t.Setenv("PATH", binDir)
	t.Setenv("CIWI_DOCKER_LOG", logPath)

	hostMount := filepath.Join(t.TempDir(), "workspace")
	cfg := runtimeContainerConfig{
		name:    "ciwi-probe-1",
		image:   "ubuntu-vmpc",
		workdir: "/workspace",
		user:    "1000:1000",
		mounts:  []runtimeContainerMount{{hostPath: hostMount, containerPath: "/workspace"}},
		devices: []string{"/dev/snd"},
		groups:  []string{"audio"},
	}
	if err := startRuntimeContainer(context.Background(), cfg); err != nil {
		t.Fatalf("startRuntimeContainer: %v", err)
	}
	if _, err := os.Stat(hostMount); err != nil {
		t.Fatalf("expected host mount path prepared: %v", err)
	}

	if err := validateProbeContainerReady(context.Background(), cfg.name, cfg.image); err != nil {
		t.Fatalf("validateProbeContainerReady should pass when inspect=true: %v", err)
	}
	t.Setenv("CIWI_DOCKER_INSPECT_RUNNING", "false")
	if err := validateProbeContainerReady(context.Background(), cfg.name, cfg.image); err == nil {
		t.Fatalf("validateProbeContainerReady should fail when inspect=false")
	}
	t.Setenv("CIWI_DOCKER_INSPECT_RUNNING", "true")

	cleanupRuntimeProbeContainer(context.Background(), cfg.name)

	logRaw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read fake docker log: %v", err)
	}
	log := string(logRaw)
	if !strings.Contains(log, "rm -f ciwi-probe-1") || !strings.Contains(log, "run -d --name ciwi-probe-1") {
		t.Fatalf("expected start/cleanup docker calls in log, got:\n%s", log)
	}
	if !strings.Contains(log, "inspect -f {{.State.Running}} ciwi-probe-1") {
		t.Fatalf("expected inspect call in log, got:\n%s", log)
	}
}

func TestCollectRuntimeCapabilitiesWithContainerProbe(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake docker shell script tests are posix-only")
	}
	binDir, _ := writeFakeDocker(t, `
if [ "$1" = "exec" ]; then
  case "$5" in
    *"cmake --version"*) echo "cmake version 3.31.5"; exit 0 ;;
    *"ninja --version"*) echo "1.11.1"; exit 0 ;;
    *"go version"*) echo "go version go1.26.0 linux/amd64"; exit 0 ;;
    *) exit 1 ;;
  esac
fi
exit 0
`)
	t.Setenv("PATH", binDir)

	caps := collectRuntimeCapabilities(map[string]string{
		"os":         "linux",
		"arch":       "amd64",
		"executor":   "script",
		"shells":     "posix",
		"run_mode":   "service",
		"tool.git":   "2.39.5",
		"tool.cmake": "3.25.1",
	}, "ubuntu-vmpc")

	if caps["container.name"] != "ubuntu-vmpc" {
		t.Fatalf("expected container.name capability, got %v", caps)
	}
	if caps["host.tool.git"] != "2.39.5" {
		t.Fatalf("expected host tool capability propagation, got %v", caps)
	}
	if caps["container.tool.cmake"] != "3.31.5" || caps["container.tool.ninja"] != "1.11.1" || caps["container.tool.go"] != "1.26.0" {
		t.Fatalf("expected probed container tools, got %v", caps)
	}
}

func TestEnsureHostMountPathsError(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "file")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatalf("write file fixture: %v", err)
	}
	err := ensureHostMountPaths([]runtimeContainerMount{{hostPath: filepath.Join(file, "sub"), containerPath: "/workspace"}})
	if err == nil {
		t.Fatalf("expected ensureHostMountPaths to fail when host path prefix is a file")
	}
}
