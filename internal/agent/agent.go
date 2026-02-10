package agent

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/izzyreal/ciwi/internal/protocol"
)

const (
	maxReportedOutputBytes = 512 * 1024
	maxArtifactFileBytes   = 25 * 1024 * 1024
	maxArtifactsPerJob     = 32
)

func Run(ctx context.Context) error {
	serverURL := envOrDefault("CIWI_SERVER_URL", "http://127.0.0.1:8080")
	agentID := envOrDefault("CIWI_AGENT_ID", defaultAgentID())
	hostname, _ := os.Hostname()
	workDir := envOrDefault("CIWI_AGENT_WORKDIR", ".ciwi-agent")

	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return fmt.Errorf("create agent workdir: %w", err)
	}

	log.Printf("ciwi agent started: id=%s server=%s", agentID, serverURL)
	defer log.Println("ciwi agent stopped")

	client := &http.Client{Timeout: 30 * time.Second}
	heartbeatTicker := time.NewTicker(10 * time.Second)
	defer heartbeatTicker.Stop()
	leaseTicker := time.NewTicker(3 * time.Second)
	defer leaseTicker.Stop()

	if err := sendHeartbeat(ctx, client, serverURL, agentID, hostname); err != nil {
		log.Printf("initial heartbeat failed: %v", err)
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-heartbeatTicker.C:
			if err := sendHeartbeat(ctx, client, serverURL, agentID, hostname); err != nil {
				log.Printf("heartbeat failed: %v", err)
			}
		case <-leaseTicker.C:
			job, err := leaseJob(ctx, client, serverURL, agentID)
			if err != nil {
				log.Printf("lease failed: %v", err)
				continue
			}
			if job == nil {
				continue
			}
			if err := executeLeasedJob(ctx, client, serverURL, agentID, workDir, *job); err != nil {
				log.Printf("execute job failed: id=%s err=%v", job.ID, err)
			}
		}
	}
}

func sendHeartbeat(ctx context.Context, client *http.Client, serverURL, agentID, hostname string) error {
	payload := protocol.HeartbeatRequest{
		AgentID:      agentID,
		Hostname:     hostname,
		OS:           runtime.GOOS,
		Arch:         runtime.GOARCH,
		Capabilities: map[string]string{"executor": "shell"},
		TimestampUTC: time.Now().UTC(),
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal heartbeat: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, serverURL+"/api/v1/heartbeat", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create heartbeat request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("send heartbeat: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4*1024))
		return fmt.Errorf("heartbeat rejected: status=%d body=%s", resp.StatusCode, bytes.TrimSpace(respBody))
	}

	log.Printf("heartbeat sent: id=%s os=%s arch=%s", agentID, runtime.GOOS, runtime.GOARCH)
	return nil
}

func leaseJob(ctx context.Context, client *http.Client, serverURL, agentID string) (*protocol.Job, error) {
	payload := protocol.LeaseJobRequest{
		AgentID: agentID,
		Capabilities: map[string]string{
			"os":       runtime.GOOS,
			"arch":     runtime.GOARCH,
			"executor": "shell",
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal lease request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, serverURL+"/api/v1/agent/lease", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create lease request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send lease request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4*1024))
		return nil, fmt.Errorf("lease rejected: status=%d body=%s", resp.StatusCode, bytes.TrimSpace(respBody))
	}

	var leaseResp protocol.LeaseJobResponse
	if err := json.NewDecoder(resp.Body).Decode(&leaseResp); err != nil {
		return nil, fmt.Errorf("decode lease response: %w", err)
	}
	if !leaseResp.Assigned || leaseResp.Job == nil {
		return nil, nil
	}

	log.Printf("job leased: id=%s", leaseResp.Job.ID)
	return leaseResp.Job, nil
}

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

func reportFailure(ctx context.Context, client *http.Client, serverURL, agentID string, job protocol.Job, exitCode *int, failMsg, output string) error {
	return reportJobStatus(ctx, client, serverURL, job.ID, protocol.JobStatusUpdateRequest{
		AgentID:      agentID,
		Status:       "failed",
		ExitCode:     exitCode,
		Error:        failMsg,
		Output:       output,
		TimestampUTC: time.Now().UTC(),
	})
}

func reportJobStatus(ctx context.Context, client *http.Client, serverURL, jobID string, reqBody protocol.JobStatusUpdateRequest) error {
	body, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("marshal job status: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, serverURL+"/api/v1/jobs/"+jobID+"/status", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create job status request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("send job status request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4*1024))
		return fmt.Errorf("status rejected: status=%d body=%s", resp.StatusCode, bytes.TrimSpace(respBody))
	}

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

func collectAndUploadArtifacts(ctx context.Context, client *http.Client, serverURL, agentID, jobID, execDir string, globs []string) (string, error) {
	artifacts, summary, err := collectArtifacts(execDir, globs)
	if err != nil {
		return summary, err
	}
	if len(artifacts) == 0 {
		return summary, nil
	}

	reqBody := protocol.UploadArtifactsRequest{AgentID: agentID, Artifacts: artifacts}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return summary, fmt.Errorf("marshal artifact upload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, serverURL+"/api/v1/jobs/"+jobID+"/artifacts", bytes.NewReader(body))
	if err != nil {
		return summary, fmt.Errorf("create artifact upload request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return summary, fmt.Errorf("send artifact upload: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4*1024))
		return summary, fmt.Errorf("artifact upload rejected: status=%d body=%s", resp.StatusCode, bytes.TrimSpace(respBody))
	}

	return summary + "\n[artifacts] uploaded", nil
}

func collectArtifacts(execDir string, globs []string) ([]protocol.UploadArtifact, string, error) {
	if len(globs) == 0 {
		return nil, "", nil
	}

	var summary strings.Builder
	fmt.Fprintf(&summary, "[artifacts] globs=%s\n", strings.Join(globs, ", "))

	seen := map[string]struct{}{}
	matched := make([]string, 0)
	for _, pattern := range globs {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}
		ms, err := doublestar.Glob(os.DirFS(execDir), pattern)
		if err != nil {
			fmt.Fprintf(&summary, "[artifacts] invalid_glob=%q err=%v\n", pattern, err)
			continue
		}
		for _, m := range ms {
			m = filepath.ToSlash(filepath.Clean(m))
			if m == "." || strings.HasPrefix(m, "../") || strings.HasPrefix(m, "/") {
				continue
			}
			if _, ok := seen[m]; ok {
				continue
			}
			seen[m] = struct{}{}
			matched = append(matched, m)
		}
	}
	sort.Strings(matched)

	uploads := make([]protocol.UploadArtifact, 0)
	for _, rel := range matched {
		if len(uploads) >= maxArtifactsPerJob {
			fmt.Fprintf(&summary, "[artifacts] cap_reached=%d\n", maxArtifactsPerJob)
			break
		}
		full := filepath.Join(execDir, filepath.FromSlash(rel))
		info, err := os.Stat(full)
		if err != nil {
			fmt.Fprintf(&summary, "[artifacts] skip=%s err=%v\n", rel, err)
			continue
		}
		if info.IsDir() {
			continue
		}
		if info.Size() > maxArtifactFileBytes {
			fmt.Fprintf(&summary, "[artifacts] skip=%s reason=size(%d>%d)\n", rel, info.Size(), maxArtifactFileBytes)
			continue
		}

		content, err := os.ReadFile(full)
		if err != nil {
			fmt.Fprintf(&summary, "[artifacts] skip=%s err=%v\n", rel, err)
			continue
		}
		uploads = append(uploads, protocol.UploadArtifact{Path: rel, DataBase64: base64.StdEncoding.EncodeToString(content)})
		fmt.Fprintf(&summary, "[artifacts] include=%s size=%d\n", rel, len(content))
	}

	if len(uploads) == 0 {
		fmt.Fprintf(&summary, "[artifacts] none\n")
	}
	return uploads, summary.String(), nil
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
