package agent

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func fakeApple(t *testing.T, script string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("POSIX fixture")
	}
	dir := t.TempDir()
	log := filepath.Join(dir, "calls")
	t.Setenv("CIWI_CONTAINER_CALLS", log)
	if err := os.WriteFile(filepath.Join(dir, "container"), []byte("#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$CIWI_CONTAINER_CALLS\"\n"+script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	return log
}

func TestAppleInspectionAndOptions(t *testing.T) {
	log := fakeApple(t, `if [ "$1" = inspect ]; then printf '%s' "$CIWI_INSPECTION"; fi`)
	r := cliContainerRuntime("apple")
	for _, fixture := range []struct {
		json  string
		ready bool
	}{
		{`[{"status":"running"}]`, true},
		{`[{"status":{"state":"running"}}]`, true},
		{`[{"status":"stopped"}]`, false},
		{`[]`, false}, {`not json`, false},
	} {
		t.Setenv("CIWI_INSPECTION", fixture.json)
		if err := r.running(context.Background(), "job"); (err == nil) != fixture.ready {
			t.Fatalf("%s: %v", fixture.json, err)
		}
	}
	cfg := runtimeContainerConfig{backend: r, name: "job", image: "test", platform: "linux/amd64", memory: "4G", cpus: "4", shmSize: "1G", user: "501:20", workdir: "/workspace", mounts: []runtimeContainerMount{{hostPath: filepath.Join(t.TempDir(), "with spaces"), containerPath: "/workspace"}}}
	if err := startRuntimeContainer(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	cleanupRuntimeProbeContainer(context.Background(), cfg.name, r)
	calls, _ := os.ReadFile(log)
	for _, value := range []string{"--platform linux/amd64", "--memory 4G", "--shm-size 1G", "--user 501:20", "rm -f job"} {
		if !strings.Contains(string(calls), value) {
			t.Fatalf("missing %s: %s", value, calls)
		}
	}
	if strings.Contains(string(calls), "inspect -f") {
		t.Fatal("used Docker inspection syntax")
	}
	cfg.devices = []string{"/dev/snd"}
	if startRuntimeContainer(context.Background(), cfg) == nil {
		t.Fatal("Apple devices accepted")
	}
}

func TestPrepareContainerImageBuildAndCancellation(t *testing.T) {
	log := fakeApple(t, `if [ "$1" = image ] && [ "$2" = inspect ]; then exit 1; fi
if [ "$CIWI_SLOW" = 1 ]; then /bin/sleep 30; fi
`)
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "Dockerfile"), []byte("FROM scratch\n"), 0644); err != nil {
		t.Fatal(err)
	}
	cfg := runtimeContainerConfig{platform: "linux/arm64", image: "ciwi-owned:latest"}
	var out syncBuffer
	if err := prepareContainerImage(context.Background(), cliContainerRuntime("apple"), cfg, root, ".", "", &out); err != nil {
		t.Fatal(err)
	}
	calls, _ := os.ReadFile(log)
	if !strings.Contains(string(calls), "build --platform linux/arm64 --tag ciwi-owned:latest") {
		t.Fatal(string(calls))
	}
	t.Setenv("CIWI_SLOW", "1")
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	started := time.Now()
	if prepareContainerImage(ctx, cliContainerRuntime("apple"), cfg, root, ".", "", &out) == nil {
		t.Fatal("cancelled build succeeded")
	}
	if time.Since(started) > 5*time.Second {
		t.Fatal("build cancellation did not stop CLI")
	}
}

func TestContainerBuildPathRejectsEscapingSymlink(t *testing.T) {
	root := t.TempDir()
	if err := os.Symlink(t.TempDir(), filepath.Join(root, "outside")); err != nil {
		t.Skip(err)
	}
	if _, err := containerPathWithin(root, "outside"); err == nil {
		t.Fatal("accepted external build context")
	}
}

func TestImageIdentityRequiresRequestedVariant(t *testing.T) {
	for _, tc := range []struct{ backend, raw string }{
		{"docker", `[{"Id":"sha256:abc","Os":"linux","Architecture":"amd64"}]`},
		{"apple", `[{"variants":[{"digest":"sha256:abc","platform":{"os":"linux","architecture":"amd64"}}]}]`},
	} {
		if got, err := parseContainerImageIdentity(tc.backend, []byte(tc.raw), "linux/amd64"); got != "sha256:abc" || err != nil {
			t.Fatalf("%s: %s %v", tc.backend, got, err)
		}
		if _, err := parseContainerImageIdentity(tc.backend, []byte(tc.raw), "linux/arm64"); err == nil {
			t.Fatalf("%s reused wrong architecture", tc.backend)
		}
	}
}

func TestContainerExecutionPreservesEnvironmentAndCleansOnTimeout(t *testing.T) {
	log := fakeApple(t, `if [ "$1" = exec ]; then
 shift
 while [ "$1" != job ]; do
 case "$1" in
 --env) export "$2"; shift 2 ;;
 -w) shift 2 ;;
 *) exit 91 ;;
 esac
 done
 shift 2
 exec /bin/sh "$@"
fi
`)
	var out syncBuffer
	progress := &outputReportState{}
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{}`)), Header: make(http.Header)}, nil
	})}
	container := &executionContainerContext{name: "job", workdir: "/workspace", backend: cliContainerRuntime("apple")}
	err := runJobScript(context.Background(), client, "http://example.local", "agent", "job", "posix", "", `[ "$VALUE" = '  exact value  ' ] && [ "$HOME" = /tmp/chosen ] && printf passed`, container, []string{"VALUE=  exact value  ", "HOME=/tmp/chosen"}, &out, nil, progress, "test", nil, false)
	if err != nil || !strings.Contains(out.String(), "passed") {
		t.Fatalf("environment lost: %v %s", err, out.String())
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	err = runJobScript(ctx, client, "http://example.local", "agent", "job", "posix", "", `exec /bin/sleep 30`, container, nil, &out, nil, progress, "test", nil, false)
	if err == nil {
		t.Fatal("timeout succeeded")
	}
	calls, _ := os.ReadFile(log)
	if !strings.Contains(string(calls), "rm -f job") {
		t.Fatalf("timed out workload was not removed: %s", calls)
	}
}

func TestContainerReadinessRefreshClearsUnavailableRuntime(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX fixture")
	}
	dir, log := writeFakeDocker(t, `case "$1" in
 --version) echo 'Docker version 28.0.0';;
 info) if [ "$CIWI_DOCKER_DOWN" = 1 ]; then exit 1; else echo linux/x86_64; fi;;
 esac`)
	t.Setenv("PATH", dir)
	t.Setenv("CIWI_DOCKER_LOG", log)
	caps := map[string]string{"os": "linux", "container.runtime.apple": "stale"}
	refreshContainerCapabilities(context.Background(), caps)
	if caps["container.runtime.docker"] != "28.0.0" || caps["container.native.docker"] != "linux/amd64" || caps["container.runtime.apple"] != "" {
		t.Fatal(caps)
	}
	t.Setenv("CIWI_DOCKER_DOWN", "1")
	refreshContainerCapabilities(context.Background(), caps)
	if caps["container.runtime.docker"] != "" || caps["container.error.docker"] == "" {
		t.Fatal(caps)
	}
}

func TestContainerPullRetriesAreBounded(t *testing.T) {
	log := fakeApple(t, `if [ "$1" = image ] && [ "$2" = inspect ]; then exit 1; fi
if [ "$1" = image ] && [ "$2" = pull ]; then echo registry-unavailable >&2; exit 1; fi
`)
	oldDelay := containerPullRetryDelay
	containerPullRetryDelay = time.Millisecond
	t.Cleanup(func() { containerPullRetryDelay = oldDelay })
	var out syncBuffer
	err := prepareContainerImage(context.Background(), cliContainerRuntime("apple"), runtimeContainerConfig{image: "missing:test", platform: "linux/arm64"}, t.TempDir(), "", "", &out)
	if err == nil || !strings.Contains(err.Error(), "after 3 attempts") {
		t.Fatalf("got %v", err)
	}
	calls, _ := os.ReadFile(log)
	if strings.Count(string(calls), "image pull --platform linux/arm64 missing:test") != 3 {
		t.Fatalf("retry count: %s", calls)
	}
	if !strings.Contains(out.String(), "registry-unavailable") {
		t.Fatal("pull failure output was lost")
	}
}
