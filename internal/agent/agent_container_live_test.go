//go:build integration

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

	"github.com/izzyreal/ciwi/internal/requirements"
)

func TestContainerLiveCancellation(t *testing.T) {
	backend := os.Getenv("CIWI_TEST_CONTAINER_RUNTIME")
	if backend != "apple" && backend != "docker" {
		t.Fatal("set CIWI_TEST_CONTAINER_RUNTIME=apple or docker for container cancellation integration")
	}
	caps := map[string]string{"os": runtime.GOOS, "arch": runtime.GOARCH}
	refreshContainerCapabilities(context.Background(), caps)
	name, platform, err := requirements.SelectContainerRuntime(map[string]string{"requires.container.runtime": backend}, caps)
	if err != nil {
		t.Fatal(err)
	}
	r := cliContainerRuntime(name)
	mount := t.TempDir()
	cfg := runtimeContainerConfig{backend: r, name: "ciwi-cancel-" + shortStableID(mount), image: "alpine:3.22", platform: platform, workdir: "/workspace", user: defaultContainerUserSpec(), mounts: []runtimeContainerMount{{hostPath: mount, containerPath: "/workspace"}}}
	var output syncBuffer
	prepCtx, prepCancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer prepCancel()
	if err := prepareContainerImage(prepCtx, r, cfg, mount, "", "", &output); err != nil {
		t.Fatalf("prepare: %v %s", err, output.String())
	}
	defer cleanupRuntimeProbeContainer(context.Background(), cfg.name, r)
	if err := startRuntimeContainer(prepCtx, cfg); err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{}`)), Header: make(http.Header)}, nil
	})}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	err = runJobScript(ctx, client, "http://example.local", "test", "test", "posix", "", `printf started > started; exec sleep 60`, &executionContainerContext{name: cfg.name, workdir: cfg.workdir, backend: r}, nil, &output, nil, &outputReportState{}, "test", nil, false)
	if err == nil {
		t.Fatal("timed out command succeeded")
	}
	if _, err := os.Stat(filepath.Join(mount, "started")); err != nil {
		t.Fatalf("workload never started: %v %s", err, output.String())
	}
	inspection, err := runCommandCapture(context.Background(), "", r.command(), "inspect", cfg.name)
	if err == nil || (!strings.Contains(strings.ToLower(inspection), "not found") && !strings.Contains(strings.ToLower(inspection), "no such")) {
		t.Fatalf("cancelled container still exists: %v %s", err, inspection)
	}
}
