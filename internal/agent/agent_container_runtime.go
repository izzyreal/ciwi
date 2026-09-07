package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/izzyreal/ciwi/internal/requirements"
)

var containerPullRetryDelay = 10 * time.Second

// containerRuntime owns CLI differences. The execution workflow never chooses
// another runtime after image preparation or workload execution has begun.
type containerRuntime interface {
	name() string
	command() string
	running(context.Context, string) error
	imageCommand(string, ...string) []string
}

type cliContainerRuntime string

func (r cliContainerRuntime) name() string { return string(r) }
func (r cliContainerRuntime) command() string {
	if r == "apple" {
		return "container"
	}
	return "docker"
}
func (r cliContainerRuntime) imageCommand(action string, args ...string) []string {
	prefix := []string{"image", action}
	if r != "apple" && action == "pull" {
		prefix = []string{"pull"}
	}
	return append(prefix, args...)
}
func (r cliContainerRuntime) running(ctx context.Context, name string) error {
	args := []string{"inspect", "-f", "{{.State.Running}}", name}
	if r == "apple" {
		args = []string{"inspect", name}
	}
	out, err := runCommandCapture(ctx, "", r.command(), args...)
	if err != nil {
		return fmt.Errorf("inspect %s container %q: %w: %s", r.name(), name, err, strings.TrimSpace(out))
	}
	ready := strings.TrimSpace(out) == "true"
	if r == "apple" {
		var entries []struct {
			Status json.RawMessage `json:"status"`
		}
		if err := json.Unmarshal([]byte(out), &entries); err != nil {
			return fmt.Errorf("decode Apple container inspection: %w", err)
		}
		if len(entries) == 1 {
			var state string
			if json.Unmarshal(entries[0].Status, &state) != nil {
				var status struct {
					State string `json:"state"`
				}
				if err := json.Unmarshal(entries[0].Status, &status); err != nil {
					return fmt.Errorf("decode Apple container status: %w", err)
				}
				state = status.State
			}
			ready = state == "running"
		}
	}
	if !ready {
		return fmt.Errorf("%s container %q is not running", r.name(), name)
	}
	return nil
}

func runtimeOrDocker(backends []containerRuntime) containerRuntime {
	if len(backends) > 0 && backends[0] != nil {
		return backends[0]
	}
	return cliContainerRuntime("docker")
}

func normalizeContainerArch(arch string) string {
	switch strings.TrimSpace(arch) {
	case "x86_64", "amd64":
		return "amd64"
	case "aarch64", "arm64":
		return "arm64"
	}
	return strings.TrimSpace(arch)
}

// Refresh only cheap runtime health probes, independently of the full tool scan.
func refreshContainerCapabilities(ctx context.Context, caps map[string]string) {
	for k := range caps {
		if strings.HasPrefix(k, "container.") {
			delete(caps, k)
		}
	}
	caps[requirements.ContainerExecutionCapability] = "1"
	for _, name := range []string{"docker", "apple"} {
		r := cliContainerRuntime(name)
		if _, err := exec.LookPath(r.command()); err != nil {
			caps["container.error."+name] = r.command() + " CLI not found"
			continue
		}
		probeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		version, err := runCommandCapture(probeCtx, "", r.command(), "--version")
		cancel()
		match := versionPattern.FindStringSubmatch(version)
		if err != nil || len(match) < 2 {
			caps["container.error."+name] = "cannot read CLI version"
			continue
		}
		v := match[1]
		if name == "apple" && (runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" || !requirements.ToolConstraintMatch(v, ">=1.2.2")) {
			caps["container.error."+name] = "requires Apple silicon macOS and container >=1.2.2"
			continue
		}
		probeCtx, cancel = context.WithTimeout(ctx, 5*time.Second)
		native := "linux/arm64"
		if name == "apple" {
			var output string
			output, err = runCommandCapture(probeCtx, "", r.command(), "system", "status", "--format", "json")
			if err == nil {
				var health struct {
					Status string `json:"status"`
				}
				if decodeErr := json.Unmarshal([]byte(output), &health); decodeErr != nil {
					err = decodeErr
				} else if health.Status != "running" {
					err = fmt.Errorf("Apple Container service is %s", health.Status)
				}
			}
		} else {
			var info string
			info, err = runCommandCapture(probeCtx, "", r.command(), "info", "--format", "{{.OSType}}/{{.Architecture}}")
			parts := strings.Split(strings.TrimSpace(info), "/")
			if err == nil && (len(parts) != 2 || parts[0] != "linux") {
				err = fmt.Errorf("Docker must use Linux containers")
			}
			if err == nil {
				native = "linux/" + normalizeContainerArch(parts[1])
			}
		}
		cancel()
		if err != nil {
			caps["container.error."+name] = "service unavailable to agent user: " + err.Error()
			continue
		}
		caps["container.runtime."+name] = v
		caps["container.native."+name] = native
		caps["container.platforms."+name] = native
		if name == "apple" {
			if _, err := os.Stat("/Library/Apple/usr/libexec/oah/libRosettaRuntime"); err == nil {
				caps["container.platforms.apple"] += ",linux/amd64"
			}
		}
	}
}

func containerPathWithin(root, relative string) (string, error) {
	base, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	target, err := filepath.EvalSymlinks(filepath.Join(base, relative))
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(base, target)
	if err != nil || filepath.IsAbs(relative) || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("container build path %q escapes %q", relative, root)
	}
	return target, nil
}

// Image preparation is charged to the job deadline, not the short VM startup
// timeout. Stream logs through the existing redacted job output channel.
func prepareContainerImage(ctx context.Context, r containerRuntime, cfg runtimeContainerConfig, sourceDir, buildContext, buildFile string, output *syncBuffer) error {
	run := func(args ...string) error {
		fmt.Fprintf(output, "[runtime] %s %s\n", r.command(), shellJoin(args))
		cmd := exec.Command(r.command(), args...)
		prepareCommandForCancellation(cmd)
		cmd.Stdout, cmd.Stderr = output, output
		return runCancelableCommand(ctx, cmd)
	}
	if buildContext != "" {
		dir, err := containerPathWithin(sourceDir, buildContext)
		if err != nil {
			return fmt.Errorf("build context: %w", err)
		}
		if buildFile == "" {
			buildFile = "Dockerfile"
		}
		file, err := containerPathWithin(dir, buildFile)
		if err != nil {
			return fmt.Errorf("Dockerfile: %w", err)
		}
		args := []string{"build", "--platform", cfg.platform, "--tag", cfg.image, "--file", file, "--progress", "plain", dir}
		if err := run(args...); err != nil {
			return fmt.Errorf("%s image build: %w", r.name(), err)
		}
		return nil
	}
	_, err := inspectContainerImage(ctx, r, cfg.image, cfg.platform)
	if err == nil {
		return nil
	}
	for attempt := 1; attempt <= 3; attempt++ {
		err = run(r.imageCommand("pull", "--platform", cfg.platform, cfg.image)...)
		if err == nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if attempt < 3 {
			delay := time.Duration(attempt) * containerPullRetryDelay
			fmt.Fprintf(output, "[runtime] image pull attempt %d/3 failed; retrying in %s\n", attempt, delay)
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
		}
	}
	return fmt.Errorf("%s image pull failed after 3 attempts: %w", r.name(), err)
}

func cleanupContainerImage(r containerRuntime, tag string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	existing, inspectErr := runCommandCapture(ctx, "", r.command(), r.imageCommand("inspect", tag)...)
	lower := strings.ToLower(existing)
	if inspectErr != nil && (strings.Contains(lower, "not found") || strings.Contains(lower, "no such")) {
		return
	}
	out, err := runCommandCapture(ctx, "", r.command(), r.imageCommand("rm", tag)...)
	logContainerCleanupFailure(r.name(), tag, out, err)
}

// Inspect the requested variant, not just the tag: a cached arm64 image is not
// sufficient for an amd64 job. Return an immutable identity for the job record.
func inspectContainerImage(ctx context.Context, r containerRuntime, image, platform string) (string, error) {
	probeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	out, err := runCommandCapture(probeCtx, "", r.command(), r.imageCommand("inspect", image)...)
	if err != nil {
		return "", fmt.Errorf("inspect image %q: %w", image, err)
	}
	return parseContainerImageIdentity(r.name(), []byte(out), platform)
}

func parseContainerImageIdentity(backend string, raw []byte, platform string) (string, error) {
	if backend == "docker" {
		var entries []struct {
			ID   string `json:"Id"`
			OS   string `json:"Os"`
			Arch string `json:"Architecture"`
		}
		if err := json.Unmarshal(raw, &entries); err != nil {
			return "", err
		}
		for _, e := range entries {
			if e.OS+"/"+normalizeContainerArch(e.Arch) == platform && e.ID != "" {
				return e.ID, nil
			}
		}
	} else {
		var entries []struct {
			Variants []struct {
				Digest   string `json:"digest"`
				Platform struct {
					OS   string `json:"os"`
					Arch string `json:"architecture"`
				} `json:"platform"`
			} `json:"variants"`
		}
		if err := json.Unmarshal(raw, &entries); err != nil {
			return "", err
		}
		for _, e := range entries {
			for _, v := range e.Variants {
				if v.Platform.OS+"/"+normalizeContainerArch(v.Platform.Arch) == platform && v.Digest != "" {
					return v.Digest, nil
				}
			}
		}
	}
	return "", fmt.Errorf("image has no local %s variant", platform)
}

func logContainerCleanupFailure(backend, name, output string, err error) {
	if err == nil {
		return
	}
	lower := strings.ToLower(output)
	if strings.Contains(lower, "not found") || strings.Contains(lower, "no such") {
		return
	}
	slog.Warn("container resource cleanup failed", "runtime", backend, "resource", name, "error", err, "output", strings.TrimSpace(output))
}
