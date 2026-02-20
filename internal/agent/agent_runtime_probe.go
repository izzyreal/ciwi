package agent

import (
	"context"
	"fmt"
	"hash/fnv"
	"os"
	"os/exec"
	"os/user"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/izzyreal/ciwi/internal/requirements"
)

const containerToolRequirementPrefix = "requires.container.tool."

func collectRuntimeCapabilities(agentCapabilities map[string]string, probeContainer string) map[string]string {
	out := map[string]string{}
	for k, v := range agentCapabilities {
		k = strings.TrimSpace(k)
		if k == "" || strings.TrimSpace(v) == "" {
			continue
		}
		if strings.HasPrefix(k, "tool.") || k == "os" || k == "arch" || k == "shells" || k == "executor" || k == "run_mode" {
			out["host."+k] = strings.TrimSpace(v)
		}
	}
	container := strings.TrimSpace(probeContainer)
	if container == "" {
		return out
	}
	out["container.name"] = container
	if _, err := exec.LookPath("docker"); err != nil {
		out["container.probe_error"] = "docker not found on agent"
		return out
	}
	tools := []struct {
		name string
		cmd  string
		args []string
	}{
		{name: "git", cmd: "git", args: []string{"--version"}},
		{name: "go", cmd: "go", args: []string{"version"}},
		{name: "gh", cmd: "gh", args: []string{"--version"}},
		{name: "cmake", cmd: "cmake", args: []string{"--version"}},
		{name: "ccache", cmd: "ccache", args: []string{"--version"}},
		{name: "ninja", cmd: "ninja", args: []string{"--version"}},
		{name: "gcc", cmd: "gcc", args: []string{"--version"}},
		{name: "clang", cmd: "clang", args: []string{"--version"}},
		{name: "docker", cmd: "docker", args: []string{"--version"}},
		{name: "xcodebuild", cmd: "xcodebuild", args: []string{"-version"}},
		{name: "iscc", cmd: "iscc", args: []string{"/?"}},
		{name: "signtool", cmd: "signtool", args: []string{"/?"}},
	}
	for _, t := range tools {
		if v := detectToolVersionInContainer(container, t.cmd, t.args...); v != "" {
			out["container.tool."+t.name] = v
		}
	}
	if runtime.GOOS == "windows" {
		// Probing a Linux container from Windows host usually still works; no extra host-specific keys needed.
	}
	return out
}

func detectToolVersionInContainer(container, cmd string, args ...string) string {
	container = strings.TrimSpace(container)
	cmd = strings.TrimSpace(cmd)
	if container == "" || cmd == "" {
		return ""
	}
	quoted := shellQuote(cmd)
	for _, arg := range args {
		quoted += " " + shellQuote(arg)
	}
	script := quoted
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	out, err := runCommandCapture(ctx, "", "docker", "exec", container, "sh", "-lc", script)
	if err != nil && strings.TrimSpace(out) == "" {
		return ""
	}
	text := strings.TrimSpace(out)
	if text == "" {
		return ""
	}
	if strings.Contains(text, "go version go") {
		text = strings.ReplaceAll(text, "go version go", "go version ")
	}
	if m := versionPattern.FindStringSubmatch(text); len(m) >= 2 {
		return m[1]
	}
	return ""
}

func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	if !strings.ContainsAny(s, " \t\n\r'\"\\$`!&|;<>()[]{}*?~") {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}

func runtimeProbeContainerImageFromMetadata(meta map[string]string) string {
	if len(meta) == 0 {
		return ""
	}
	return strings.TrimSpace(meta["runtime_probe.container_image"])
}

func runtimeExecContainerWorkdirFromMetadata(meta map[string]string) string {
	if len(meta) == 0 {
		return ""
	}
	return strings.TrimSpace(meta["runtime_exec.container_workdir"])
}

func runtimeExecContainerUserFromMetadata(meta map[string]string) string {
	if len(meta) == 0 {
		return ""
	}
	return strings.TrimSpace(meta["runtime_exec.container_user"])
}

func runtimeProbeContainerName(jobID string, meta map[string]string) string {
	if runtimeProbeContainerImageFromMetadata(meta) == "" {
		return ""
	}
	return "ciwi-probe-" + shortStableID(jobID)
}

func shortStableID(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "unknown"
	}
	h := fnv.New64a()
	_, _ = h.Write([]byte(s))
	return fmt.Sprintf("%x", h.Sum64())
}

type runtimeContainerMount struct {
	hostPath      string
	containerPath string
}

type runtimeContainerConfig struct {
	name    string
	image   string
	workdir string
	user    string
	mounts  []runtimeContainerMount
}

func defaultContainerUserSpec() string {
	if runtime.GOOS == "windows" {
		return ""
	}
	u, err := user.Current()
	if err != nil {
		return ""
	}
	uid := strings.TrimSpace(u.Uid)
	gid := strings.TrimSpace(u.Gid)
	if uid == "" || gid == "" {
		return ""
	}
	return uid + ":" + gid
}

func ensureHostMountPaths(mounts []runtimeContainerMount) error {
	for _, m := range mounts {
		hostPath := strings.TrimSpace(m.hostPath)
		if hostPath == "" {
			continue
		}
		if err := os.MkdirAll(hostPath, 0o755); err != nil {
			return fmt.Errorf("prepare host mount path %q: %w", hostPath, err)
		}
	}
	return nil
}

func startRuntimeContainer(ctx context.Context, cfg runtimeContainerConfig) error {
	name := strings.TrimSpace(cfg.name)
	image := strings.TrimSpace(cfg.image)
	if name == "" || image == "" {
		return nil
	}
	name = strings.TrimSpace(name)
	if _, err := exec.LookPath("docker"); err != nil {
		return fmt.Errorf("docker not found on agent")
	}
	if err := ensureHostMountPaths(cfg.mounts); err != nil {
		return err
	}
	cleanupCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	_, _ = runCommandCapture(cleanupCtx, "", "docker", "rm", "-f", name)

	args := []string{"run", "-d", "--name", name}
	if workdir := strings.TrimSpace(cfg.workdir); workdir != "" {
		args = append(args, "-w", workdir)
	}
	if userSpec := strings.TrimSpace(cfg.user); userSpec != "" {
		args = append(args, "--user", userSpec)
	}
	for _, m := range cfg.mounts {
		hostPath := strings.TrimSpace(m.hostPath)
		containerPath := strings.TrimSpace(m.containerPath)
		if hostPath == "" || containerPath == "" {
			continue
		}
		args = append(args, "-v", hostPath+":"+containerPath)
	}
	args = append(args, image, "sleep", "infinity")

	startCtx, startCancel := context.WithTimeout(ctx, 15*time.Second)
	defer startCancel()
	if _, err := runCommandCapture(startCtx, "", "docker", args...); err != nil {
		return fmt.Errorf("start runtime container %q from %q: %w", name, image, err)
	}
	return nil
}

func cleanupRuntimeProbeContainer(ctx context.Context, name string) {
	name = strings.TrimSpace(name)
	if name == "" {
		return
	}
	if _, err := exec.LookPath("docker"); err != nil {
		return
	}
	cleanupCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	_, _ = runCommandCapture(cleanupCtx, "", "docker", "rm", "-f", name)
}

func validateProbeContainerReady(ctx context.Context, name, image string) error {
	name = strings.TrimSpace(name)
	image = strings.TrimSpace(image)
	if name == "" || image == "" {
		return nil
	}
	if _, err := exec.LookPath("docker"); err != nil {
		return fmt.Errorf("docker not found on agent")
	}
	checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	out, err := runCommandCapture(checkCtx, "", "docker", "inspect", "-f", "{{.State.Running}}", name)
	if err != nil {
		return fmt.Errorf("runtime container %q is not inspectable: %w", name, err)
	}
	if !strings.EqualFold(strings.TrimSpace(out), "true") {
		return fmt.Errorf("runtime container %q is not running", name)
	}
	return nil
}

func runtimeProbeSummary(runtimeCaps map[string]string) string {
	if len(runtimeCaps) == 0 {
		return ""
	}
	hostTools := 0
	containerTools := 0
	for k := range runtimeCaps {
		if strings.HasPrefix(k, "host.tool.") {
			hostTools++
		}
		if strings.HasPrefix(k, "container.tool.") {
			containerTools++
		}
	}
	return fmt.Sprintf("[runtime] host_tools=%d container_tools=%d", hostTools, containerTools)
}

func containerToolRequirements(requiredCaps map[string]string) map[string]string {
	if len(requiredCaps) == 0 {
		return nil
	}
	out := map[string]string{}
	for key, value := range requiredCaps {
		if !strings.HasPrefix(key, containerToolRequirementPrefix) {
			continue
		}
		tool := strings.TrimSpace(strings.TrimPrefix(key, containerToolRequirementPrefix))
		if tool == "" {
			continue
		}
		out[tool] = strings.TrimSpace(value)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func validateContainerToolRequirements(requiredCaps, runtimeCaps map[string]string) error {
	reqs := containerToolRequirements(requiredCaps)
	if len(reqs) == 0 {
		return nil
	}
	failed := []string{}
	for tool, constraint := range reqs {
		observed := strings.TrimSpace(runtimeCaps["container.tool."+tool])
		if !requirements.ToolConstraintMatch(observed, constraint) {
			if observed == "" {
				if constraint == "" || constraint == "*" {
					failed = append(failed, fmt.Sprintf("%s missing in runtime container", tool))
				} else {
					failed = append(failed, fmt.Sprintf("%s missing in runtime container (required %s)", tool, constraint))
				}
			} else if constraint == "" || constraint == "*" {
				failed = append(failed, fmt.Sprintf("%s unavailable in runtime container", tool))
			} else {
				failed = append(failed, fmt.Sprintf("%s=%s does not satisfy %s", tool, observed, constraint))
			}
		}
	}
	if len(failed) == 0 {
		return nil
	}
	sort.Strings(failed)
	return fmt.Errorf("container tool requirements unmet: %s", strings.Join(failed, "; "))
}
