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

func runJobScript(
	runCtx context.Context,
	client *http.Client,
	serverURL, agentID, jobID, shell, execDir, script string,
	container *executionContainerContext,
	env []string,
	output *syncBuffer,
	defaultCurrentStep string,
	sensitive []string,
	traceShell bool,
) error {
	tracedScript := applyShellTracing(shell, script, traceShell)
	var cmd *exec.Cmd
	if container != nil {
		if normalizeShell(shell) != shellPosix {
			return fmt.Errorf("container execution supports only posix shell")
		}
		containerName := strings.TrimSpace(container.name)
		if containerName == "" {
			return fmt.Errorf("container execution requires container name")
		}
		args := []string{"exec"}
		if w := strings.TrimSpace(container.workdir); w != "" {
			args = append(args, "-w", w)
		}
		for _, e := range env {
			e = strings.TrimSpace(e)
			if e == "" || !strings.Contains(e, "=") {
				continue
			}
			args = append(args, "--env", e)
		}
		args = append(args, containerName, "sh", "-lc", tracedScript)
		cmd = exec.Command("docker", args...)
	} else {
		bin, args, err := commandForScript(shell, tracedScript)
		if err == nil && runtime.GOOS == "windows" && shell == shellCmd {
			stagedCmd, stageErr := stageCmdScript(execDir, tracedScript)
			if stageErr != nil {
				return fmt.Errorf("stage cmd script: %w", stageErr)
			}
			bin, args, err = commandForScript(shell, stagedCmd)
		}
		if err != nil {
			return fmt.Errorf("build shell command: %w", err)
		}
		cmd = exec.Command(bin, args...)
		cmd.Dir = execDir
		cmd.Env = env
	}
	prepareCommandForCancellation(cmd)
	cmd.Stdout = output
	cmd.Stderr = output

	stopStreaming := streamRunningUpdates(runCtx, client, serverURL, agentID, jobID, output, sensitive, defaultCurrentStep)
	defer stopStreaming()
	return runCancelableCommand(runCtx, cmd)
}

func runCancelableCommand(ctx context.Context, cmd *exec.Cmd) error {
	if err := cmd.Start(); err != nil {
		return err
	}

	waitCh := make(chan error, 1)
	go func() {
		waitCh <- cmd.Wait()
	}()

	select {
	case err := <-waitCh:
		return err
	case <-ctx.Done():
		_ = interruptCommandTree(cmd)
		timer := time.NewTimer(2 * time.Second)
		defer timer.Stop()
		select {
		case err := <-waitCh:
			if err != nil {
				return err
			}
			return ctx.Err()
		case <-timer.C:
			_ = killCommandTree(cmd)
			select {
			case err := <-waitCh:
				if err != nil {
					return err
				}
			case <-time.After(2 * time.Second):
			}
			return ctx.Err()
		}
	}
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

type executionContainerContext struct {
	name    string
	workdir string
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
		reportedEmptySnapshot := false
		sendSnapshot := func() {
			rawSnapshot := trimOutput(output.String())
			snapshot := redactSensitive(rawSnapshot, sensitive)
			currentStep := defaultCurrentStep
			if strings.TrimSpace(snapshot) == "" && strings.TrimSpace(currentStep) != "" {
				if !reportedEmptySnapshot {
					slog.Warn("running update has empty output snapshot", "job_execution_id", jobID, "agent_id", agentID, "current_step", currentStep)
					reportedEmptySnapshot = true
				}
			} else if strings.TrimSpace(snapshot) != "" {
				reportedEmptySnapshot = false
			}
			if snapshot == lastSent && currentStep == lastStep {
				return
			}
			lastSent = snapshot
			lastStep = currentStep
			if err := reportJobStatus(ctx, client, serverURL, jobID, protocol.JobExecutionStatusUpdateRequest{
				AgentID:      agentID,
				Status:       protocol.JobExecutionStatusRunning,
				Output:       snapshot,
				CurrentStep:  currentStep,
				TimestampUTC: time.Now().UTC(),
			}); err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
				slog.Error("stream running update failed", "job_execution_id", jobID, "error", err)
			}
		}
		// Don't wait for the first ticker interval; publish an initial snapshot immediately.
		sendSnapshot()
		for {
			select {
			case <-stopCh:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				sendSnapshot()
			}
		}
	}()

	return func() {
		ticker.Stop()
		close(stopCh)
		<-done
	}
}
