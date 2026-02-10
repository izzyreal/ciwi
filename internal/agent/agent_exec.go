package agent

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/izzyreal/ciwi/internal/protocol"
)

func executeLeasedJob(ctx context.Context, client *http.Client, serverURL, agentID, workDir string, job protocol.Job) error {
	if err := reportJobStatus(ctx, client, serverURL, job.ID, protocol.JobStatusUpdateRequest{
		AgentID:      agentID,
		Status:       "running",
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

	var output bytes.Buffer
	fmt.Fprintf(&output, "[meta] agent=%s os=%s arch=%s\n", agentID, runtime.GOOS, runtime.GOARCH)
	fmt.Fprintf(&output, "[meta] job_id=%s timeout_seconds=%d\n", job.ID, job.TimeoutSeconds)

	execDir := jobDir
	if job.Source != nil && strings.TrimSpace(job.Source.Repo) != "" {
		sourceDir := filepath.Join(jobDir, "src")
		checkoutStart := time.Now()
		fmt.Fprintf(&output, "[checkout] repo=%s ref=%s\n", job.Source.Repo, job.Source.Ref)
		checkoutOutput, checkoutErr := checkoutSource(runCtx, sourceDir, *job.Source)
		output.WriteString(checkoutOutput)
		fmt.Fprintf(&output, "[checkout] duration=%s\n", time.Since(checkoutStart).Round(time.Millisecond))
		if checkoutErr != nil {
			exitCode := exitCodeFromErr(checkoutErr)
			failMsg := "checkout failed: " + checkoutErr.Error()
			trimmedOutput := trimOutput(output.String())
			if reportErr := reportFailure(ctx, client, serverURL, agentID, job, exitCode, failMsg, trimmedOutput); reportErr != nil {
				return reportErr
			}
			log.Printf("job failed during checkout: id=%s err=%s", job.ID, failMsg)
			return nil
		}
		execDir = sourceDir
	}

	traceShell := boolEnv("CIWI_AGENT_TRACE_SHELL", true)
	verboseGo := boolEnv("CIWI_AGENT_GO_BUILD_VERBOSE", true)

	tracedScript := job.Script
	if runtime.GOOS == "windows" {
		prefix := "$ErrorActionPreference='Stop'\n"
		if traceShell {
			prefix += "Set-PSDebug -Trace 1\n"
		}
		tracedScript = prefix + tracedScript
	} else {
		prefix := "set -e\n"
		if traceShell {
			prefix += "set -x\n"
		}
		tracedScript = prefix + tracedScript
	}

	fmt.Fprintf(&output, "[run] shell_trace=%t go_build_verbose=%t\n", traceShell, verboseGo)

	bin, args := commandForScript(tracedScript)
	cmd := exec.CommandContext(runCtx, bin, args...)
	cmd.Dir = execDir
	cmd.Stdout = &output
	cmd.Stderr = &output
	cmd.Env = withGoVerbose(os.Environ(), verboseGo)

	runStart := time.Now()
	err := cmd.Run()
	duration := time.Since(runStart).Round(time.Millisecond)
	fmt.Fprintf(&output, "\n[run] duration=%s\n", duration)

	if len(job.ArtifactGlobs) > 0 {
		note, uploadErr := collectAndUploadArtifacts(ctx, client, serverURL, agentID, job.ID, execDir, job.ArtifactGlobs)
		if note != "" {
			output.WriteString(note)
			if !strings.HasSuffix(note, "\n") {
				output.WriteString("\n")
			}
		}
		if uploadErr != nil {
			fmt.Fprintf(&output, "[artifacts] upload_failed=%v\n", uploadErr)
		}
	}

	trimmedOutput := trimOutput(output.String())

	if err == nil {
		exitCode := 0
		if reportErr := reportJobStatus(ctx, client, serverURL, job.ID, protocol.JobStatusUpdateRequest{
			AgentID:      agentID,
			Status:       "succeeded",
			ExitCode:     &exitCode,
			Output:       trimmedOutput,
			TimestampUTC: time.Now().UTC(),
		}); reportErr != nil {
			return fmt.Errorf("report succeeded status: %w", reportErr)
		}
		log.Printf("job succeeded: id=%s", job.ID)
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
	log.Printf("job failed: id=%s exit=%v err=%s", job.ID, exitCode, failMsg)
	return nil
}

func commandForScript(script string) (string, []string) {
	if runtime.GOOS == "windows" {
		return "powershell", []string{"-NoProfile", "-NonInteractive", "-Command", script}
	}
	return "sh", []string{"-lc", script}
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
	output, err := cmd.CombinedOutput()
	return string(output), err
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
