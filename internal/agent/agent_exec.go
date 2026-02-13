package agent

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/izzyreal/ciwi/internal/protocol"
)

const (
	executorScript  = "script"
	shellPosix      = "posix"
	shellCmd        = "cmd"
	shellPowerShell = "powershell"
)

func executeLeasedJob(ctx context.Context, client *http.Client, serverURL, agentID, workDir string, agentCapabilities map[string]string, job protocol.Job) error {
	if err := reportJobStatus(ctx, client, serverURL, job.ID, protocol.JobStatusUpdateRequest{
		AgentID:      agentID,
		Status:       "running",
		CurrentStep:  "Preparing execution",
		TimestampUTC: time.Now().UTC(),
	}); err != nil {
		return fmt.Errorf("report running status: %w", err)
	}

	jobDir := filepath.Join(workDir, job.ID)
	if err := os.RemoveAll(jobDir); err != nil {
		return reportFailure(ctx, client, serverURL, agentID, job, nil, fmt.Sprintf("prepare workdir: %v", err), "")
	}
	if err := os.MkdirAll(jobDir, 0o755); err != nil {
		return reportFailure(ctx, client, serverURL, agentID, job, nil, fmt.Sprintf("create workdir: %v", err), "")
	}

	runCtx := ctx
	cancel := func() {}
	if job.TimeoutSeconds > 0 {
		runCtx, cancel = context.WithTimeout(ctx, time.Duration(job.TimeoutSeconds)*time.Second)
	}
	defer cancel()

	var output syncBuffer
	fmt.Fprintf(&output, "[meta] agent=%s os=%s arch=%s\n", agentID, runtime.GOOS, runtime.GOARCH)
	fmt.Fprintf(&output, "[meta] job_id=%s timeout_seconds=%d\n", job.ID, job.TimeoutSeconds)

	execDir := jobDir
	if job.Source != nil && strings.TrimSpace(job.Source.Repo) != "" {
		sourceDir := filepath.Join(jobDir, "src")
		checkoutStart := time.Now()
		fmt.Fprintf(&output, "[checkout] repo=%s ref=%s\n", job.Source.Repo, job.Source.Ref)
		if err := reportJobStatus(ctx, client, serverURL, job.ID, protocol.JobStatusUpdateRequest{
			AgentID:      agentID,
			Status:       "running",
			CurrentStep:  "Checking out source",
			TimestampUTC: time.Now().UTC(),
		}); err != nil {
			return fmt.Errorf("report checkout status: %w", err)
		}
		checkoutOutput, checkoutErr := checkoutSource(runCtx, sourceDir, *job.Source)
		output.WriteString(checkoutOutput)
		fmt.Fprintf(&output, "[checkout] duration=%s\n", time.Since(checkoutStart).Round(time.Millisecond))
		if checkoutErr != nil {
			exitCode := exitCodeFromErr(checkoutErr)
			failMsg := "checkout failed: " + checkoutErr.Error()
			trimmedOutput := redactSensitive(trimOutput(output.String()), job.SensitiveValues)
			if reportErr := reportFailure(ctx, client, serverURL, agentID, job, exitCode, failMsg, trimmedOutput); reportErr != nil {
				return reportErr
			}
			slog.Error("job failed during checkout", "job_execution_id", job.ID, "error", failMsg)
			return nil
		}
		execDir = sourceDir
	}
	cacheEnv, cacheLogs := resolveJobCacheEnv(workDir, execDir, job, agentCapabilities)
	for _, line := range cacheLogs {
		fmt.Fprintf(&output, "[cache] %s\n", line)
	}

	traceShell := boolEnv("CIWI_AGENT_TRACE_SHELL", true)
	verboseGo := boolEnv("CIWI_AGENT_GO_BUILD_VERBOSE", true)
	if job.Metadata != nil {
		// Keep ad-hoc runs readable and avoid leaking shell traces for secret-backed jobs.
		if job.Metadata["has_secrets"] == "1" || job.Metadata["adhoc"] == "1" {
			traceShell = false
		}
	}

	shell, err := resolveJobShell(job.RequiredCapabilities)
	if err != nil {
		return reportFailure(ctx, client, serverURL, agentID, job, nil, fmt.Sprintf("resolve job shell: %v", err), "")
	}
	tracedScript := applyShellTracing(shell, job.Script, traceShell)

	fmt.Fprintf(&output, "[run] shell_trace=%t go_build_verbose=%t\n", traceShell, verboseGo)
	fmt.Fprintf(&output, "[run] shell=%s\n", shell)

	bin, args, err := commandForScript(shell, tracedScript)
	if err == nil && runtime.GOOS == "windows" && shell == shellCmd {
		stagedCmd, stageErr := stageCmdScript(execDir, tracedScript)
		if stageErr != nil {
			return reportFailure(ctx, client, serverURL, agentID, job, nil, fmt.Sprintf("stage cmd script: %v", stageErr), "")
		}
		bin, args, err = commandForScript(shell, stagedCmd)
	}
	if err != nil {
		return reportFailure(ctx, client, serverURL, agentID, job, nil, fmt.Sprintf("build shell command: %v", err), "")
	}
	cmd := exec.CommandContext(runCtx, bin, args...)
	cmd.Dir = execDir
	cmd.Stdout = &output
	cmd.Stderr = &output
	cmd.Env = withGoVerbose(mergeEnv(mergeEnv(os.Environ(), job.Env), cacheEnv), verboseGo)

	stopStreaming := streamRunningUpdates(runCtx, client, serverURL, agentID, job.ID, &output, job.SensitiveValues, "Running job script")
	defer stopStreaming()

	runStart := time.Now()
	err = cmd.Run()
	duration := time.Since(runStart).Round(time.Millisecond)
	fmt.Fprintf(&output, "\n[run] duration=%s\n", duration)

	if len(job.ArtifactGlobs) > 0 {
		fmt.Fprintf(&output, "[artifacts] collecting...\n")
		note, uploadErr := collectAndUploadArtifacts(ctx, client, serverURL, agentID, job.ID, execDir, job.ArtifactGlobs, func(msg string) {
			fmt.Fprintf(&output, "%s\n", msg)
		})
		if note != "" {
			output.WriteString(note)
			if !strings.HasSuffix(note, "\n") {
				output.WriteString("\n")
			}
		}
		if uploadErr != nil {
			fmt.Fprintf(&output, "[artifacts] upload_failed=%v\n", uploadErr)
			trimmedOutput := redactSensitive(trimOutput(output.String()), job.SensitiveValues)
			failMsg := "artifact upload failed: " + uploadErr.Error()
			if reportErr := reportFailure(ctx, client, serverURL, agentID, job, nil, failMsg, trimmedOutput); reportErr != nil {
				return reportErr
			}
			slog.Error("job failed", "job_execution_id", job.ID, "error", failMsg)
			return nil
		}
	}

	trimmedOutput := redactSensitive(trimOutput(output.String()), job.SensitiveValues)
	testReport := parseJobTestReport(output.String())
	if testReport.Total > 0 {
		if err := uploadTestReport(ctx, client, serverURL, agentID, job.ID, testReport); err != nil {
			fmt.Fprintf(&output, "[tests] upload_failed=%v\n", err)
		} else {
			fmt.Fprintf(&output, "%s\n", testReportSummary(testReport))
		}
		trimmedOutput = redactSensitive(trimOutput(output.String()), job.SensitiveValues)
	}

	if err == nil {
		exitCode := 0
		if reportErr := reportTerminalJobStatusWithRetry(client, serverURL, job.ID, protocol.JobStatusUpdateRequest{
			AgentID:      agentID,
			Status:       "succeeded",
			ExitCode:     &exitCode,
			Output:       trimmedOutput,
			CurrentStep:  "",
			TimestampUTC: time.Now().UTC(),
		}); reportErr != nil {
			return fmt.Errorf("report succeeded status: %w", reportErr)
		}
		slog.Info("job succeeded", "job_execution_id", job.ID)
		return nil
	}

	exitCode := exitCodeFromErr(err)
	failMsg := err.Error()
	if runCtx.Err() == context.DeadlineExceeded {
		failMsg = "job timed out"
	}
	if reportErr := reportFailure(ctx, client, serverURL, agentID, job, exitCode, failMsg, trimmedOutput); reportErr != nil {
		return reportErr
	}
	slog.Error("job failed", "job_execution_id", job.ID, "exit_code", exitCode, "error", failMsg)
	return nil
}

func commandForScript(shell, script string) (string, []string, error) {
	switch normalizeShell(shell) {
	case shellPosix:
		return "sh", []string{"-c", script}, nil
	case shellCmd:
		if runtime.GOOS != "windows" {
			return "", nil, fmt.Errorf("shell %q is only supported on windows agents", shellCmd)
		}
		return "cmd.exe", []string{"/d", "/c", script}, nil
	case shellPowerShell:
		if runtime.GOOS != "windows" {
			return "", nil, fmt.Errorf("shell %q is only supported on windows agents", shellPowerShell)
		}
		return "powershell", []string{"-NoProfile", "-NonInteractive", "-Command", script}, nil
	default:
		return "", nil, fmt.Errorf("unsupported shell %q", shell)
	}
}

func stageCmdScript(execDir, script string) (string, error) {
	if strings.TrimSpace(execDir) == "" {
		return "", fmt.Errorf("exec dir is required")
	}
	path := filepath.Join(execDir, "ciwi-job-script.cmd")
	normalized := strings.ReplaceAll(script, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	normalized = strings.ReplaceAll(normalized, "\n", "\r\n")
	if !strings.HasSuffix(normalized, "\r\n") {
		normalized += "\r\n"
	}
	if err := os.WriteFile(path, []byte(normalized), 0o644); err != nil {
		return "", fmt.Errorf("write staged cmd script: %w", err)
	}
	return path, nil
}

func applyShellTracing(shell, script string, trace bool) string {
	switch normalizeShell(shell) {
	case shellCmd:
		prefix := "@echo off\r\n"
		if trace {
			prefix = "@echo on\r\n"
		}
		return prefix + script
	case shellPowerShell:
		prefix := "$ErrorActionPreference='Stop'\n"
		if trace {
			prefix += "Set-PSDebug -Trace 1\n"
		}
		return prefix + script
	default:
		prefix := "set -e\n"
		if trace {
			prefix += "set -x\n"
		}
		return prefix + script
	}
}

func resolveJobShell(requiredCaps map[string]string) (string, error) {
	raw := strings.TrimSpace(requiredCaps["shell"])
	if raw == "" {
		return defaultShellForRuntime(), nil
	}
	if v := normalizeShell(raw); v != "" {
		return v, nil
	}
	return "", fmt.Errorf("unsupported shell %q", raw)
}

func defaultShellForRuntime() string {
	if runtime.GOOS == "windows" {
		return shellCmd
	}
	return shellPosix
}

func supportedShellsForRuntime() []string {
	if runtime.GOOS == "windows" {
		return []string{shellCmd, shellPowerShell}
	}
	return []string{shellPosix}
}

func normalizeShell(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case shellPosix:
		return shellPosix
	case shellCmd:
		return shellCmd
	case shellPowerShell:
		return shellPowerShell
	default:
		return ""
	}
}

type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) WriteString(str string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.WriteString(str)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

func streamRunningUpdates(ctx context.Context, client *http.Client, serverURL, agentID, jobID string, output *syncBuffer, sensitive []string, defaultCurrentStep string) func() {
	ticker := time.NewTicker(500 * time.Millisecond)
	done := make(chan struct{})
	stopCh := make(chan struct{})

	go func() {
		defer close(done)
		lastSent := ""
		lastStep := ""
		for {
			select {
			case <-stopCh:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				rawSnapshot := trimOutput(output.String())
				snapshot := redactSensitive(rawSnapshot, sensitive)
				currentStep := extractCurrentStepFromOutput(rawSnapshot)
				if currentStep == "" {
					currentStep = defaultCurrentStep
				}
				if snapshot == lastSent && currentStep == lastStep {
					continue
				}
				lastSent = snapshot
				lastStep = currentStep
				if err := reportJobStatus(ctx, client, serverURL, jobID, protocol.JobStatusUpdateRequest{
					AgentID:      agentID,
					Status:       "running",
					Output:       snapshot,
					CurrentStep:  currentStep,
					TimestampUTC: time.Now().UTC(),
				}); err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
					slog.Error("stream running update failed", "job_execution_id", jobID, "error", err)
				}
			}
		}
	}()

	return func() {
		ticker.Stop()
		close(stopCh)
		<-done
	}
}

func checkoutSource(ctx context.Context, sourceDir string, source protocol.SourceSpec) (string, error) {
	var output strings.Builder

	if _, err := exec.LookPath("git"); err != nil {
		return "", fmt.Errorf("git is required on the agent: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(sourceDir), 0o755); err != nil {
		return "", fmt.Errorf("prepare source parent directory: %w", err)
	}

	cloneOut, err := runCommandCapture(ctx, "", "git", "clone", "--depth", "1", source.Repo, sourceDir)
	output.WriteString(cloneOut)
	if err != nil {
		return output.String(), fmt.Errorf("git clone: %w", err)
	}

	if strings.TrimSpace(source.Ref) == "" {
		return output.String(), nil
	}

	fetchOut, err := runCommandCapture(ctx, "", "git", "-C", sourceDir, "fetch", "--depth", "1", "origin", source.Ref)
	output.WriteString(fetchOut)
	if err != nil {
		return output.String(), fmt.Errorf("git fetch ref %q: %w", source.Ref, err)
	}

	checkoutOut, err := runCommandCapture(ctx, "", "git", "-C", sourceDir, "checkout", "--force", "FETCH_HEAD")
	output.WriteString(checkoutOut)
	if err != nil {
		return output.String(), fmt.Errorf("git checkout FETCH_HEAD: %w", err)
	}

	return output.String(), nil
}

func runCommandCapture(ctx context.Context, dir, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	return out.String(), err
}

func withGoVerbose(env []string, enabled bool) []string {
	if !enabled {
		return env
	}
	for _, e := range env {
		if strings.HasPrefix(e, "GOFLAGS=") {
			return env
		}
	}
	return append(env, "GOFLAGS=-v")
}

func mergeEnv(base []string, extra map[string]string) []string {
	if len(extra) == 0 {
		return base
	}
	out := append([]string(nil), base...)
	index := map[string]int{}
	for i, e := range out {
		if eq := strings.IndexByte(e, '='); eq > 0 {
			index[e[:eq]] = i
		}
	}
	for k, v := range extra {
		entry := k + "=" + v
		if pos, ok := index[k]; ok {
			out[pos] = entry
		} else {
			out = append(out, entry)
		}
	}
	return out
}

func redactSensitive(in string, sensitive []string) string {
	out := in
	for _, secret := range sensitive {
		if strings.TrimSpace(secret) == "" {
			continue
		}
		out = strings.ReplaceAll(out, secret, "***")
	}
	return out
}

func boolEnv(key string, defaultValue bool) bool {
	v, ok := os.LookupEnv(key)
	if !ok {
		return defaultValue
	}
	v = strings.ToLower(strings.TrimSpace(v))
	switch v {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return defaultValue
	}
}

func trimOutput(output string) string {
	if len(output) <= maxReportedOutputBytes {
		return output
	}
	return output[len(output)-maxReportedOutputBytes:]
}

func exitCodeFromErr(err error) *int {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		code := exitErr.ExitCode()
		return &code
	}
	return nil
}

func defaultAgentID() string {
	hostname, err := os.Hostname()
	if err != nil || hostname == "" {
		return "agent-unknown"
	}
	return "agent-" + hostname
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
