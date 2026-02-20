package agent

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/izzyreal/ciwi/internal/protocol"
)

func checkoutSource(ctx context.Context, sourceDir string, source protocol.SourceSpec) (string, error) {
	var output strings.Builder

	if _, err := exec.LookPath("git"); err != nil {
		return "", fmt.Errorf("git is required on the agent: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(sourceDir), 0o755); err != nil {
		return "", fmt.Errorf("prepare source parent directory: %w", err)
	}

	cloneAttempts := [][]string{
		{"clone", "--depth", "1", source.Repo, sourceDir},
		{"-c", "http.version=HTTP/1.1", "clone", "--depth", "1", source.Repo, sourceDir},
		{"-c", "http.version=HTTP/1.1", "clone", "--depth", "1", source.Repo, sourceDir},
	}
	if err := runGitWithRetry(ctx, &output, "clone", cloneAttempts, func() {
		_ = os.RemoveAll(sourceDir)
	}); err != nil {
		return output.String(), err
	}

	if strings.TrimSpace(source.Ref) == "" {
		return output.String(), nil
	}

	fetchAttempts := [][]string{
		{"-C", sourceDir, "fetch", "--depth", "1", "origin", source.Ref},
		{"-C", sourceDir, "-c", "http.version=HTTP/1.1", "fetch", "--depth", "1", "origin", source.Ref},
		{"-C", sourceDir, "-c", "http.version=HTTP/1.1", "fetch", "--depth", "1", "origin", source.Ref},
	}
	if err := runGitWithRetry(ctx, &output, fmt.Sprintf("fetch ref %q", source.Ref), fetchAttempts, nil); err != nil {
		return output.String(), err
	}

	checkoutOut, err := runCommandCapture(ctx, "", "git", "-C", sourceDir, "checkout", "--force", "FETCH_HEAD")
	output.WriteString(checkoutOut)
	if err != nil {
		return output.String(), fmt.Errorf("git checkout FETCH_HEAD: %w", err)
	}

	return output.String(), nil
}

func runGitWithRetry(ctx context.Context, output *strings.Builder, phase string, attempts [][]string, onRetry func()) error {
	for i, args := range attempts {
		runOut, err := runCommandCapture(ctx, "", "git", args...)
		output.WriteString(runOut)
		if err == nil {
			return nil
		}
		if i == len(attempts)-1 || !isRetryableGitTransportError(runOut, err) {
			return fmt.Errorf("git %s: %w", phase, err)
		}
		if onRetry != nil {
			onRetry()
		}
		output.WriteString(fmt.Sprintf("[checkout] transient git %s failure; retrying (%d/%d)\n", phase, i+2, len(attempts)))
		select {
		case <-ctx.Done():
			return fmt.Errorf("git %s: %w", phase, ctx.Err())
		case <-time.After(time.Duration(i+1) * time.Second):
		}
	}
	return fmt.Errorf("git %s: no attempts configured", phase)
}

func isRetryableGitTransportError(runOutput string, err error) bool {
	if err == nil {
		return false
	}
	combined := strings.ToLower(strings.TrimSpace(runOutput + "\n" + err.Error()))
	if combined == "" {
		return false
	}
	nonRetryable := []string{
		"authentication failed",
		"repository not found",
		"could not read username",
		"permission denied",
		"access denied",
		"invalid username or password",
	}
	for _, marker := range nonRetryable {
		if strings.Contains(combined, marker) {
			return false
		}
	}
	retryable := []string{
		"http/2 stream",
		"stream was not closed cleanly",
		"remote end hung up unexpectedly",
		"early eof",
		"unexpected eof",
		"connection reset by peer",
		"connection timed out",
		"tls handshake timeout",
		"failed to connect",
		"network is unreachable",
		"temporary failure",
		"timeout",
	}
	for _, marker := range retryable {
		if strings.Contains(combined, marker) {
			return true
		}
	}
	return false
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
