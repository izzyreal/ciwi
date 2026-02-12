package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"runtime"
	"strings"
	"time"

	"github.com/izzyreal/ciwi/internal/protocol"
)

const (
	terminalStatusMaxAttempts = 5
	terminalStatusAttemptTTL  = 30 * time.Second
)

func sendHeartbeat(ctx context.Context, client *http.Client, serverURL, agentID, hostname string, capabilities map[string]string) (protocol.HeartbeatResponse, error) {
	payload := protocol.HeartbeatRequest{
		AgentID:      agentID,
		Hostname:     hostname,
		OS:           runtime.GOOS,
		Arch:         runtime.GOARCH,
		Version:      currentVersion(),
		Capabilities: cloneMap(capabilities),
		TimestampUTC: time.Now().UTC(),
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return protocol.HeartbeatResponse{}, fmt.Errorf("marshal heartbeat: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, serverURL+"/api/v1/heartbeat", bytes.NewReader(body))
	if err != nil {
		return protocol.HeartbeatResponse{}, fmt.Errorf("create heartbeat request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return protocol.HeartbeatResponse{}, fmt.Errorf("send heartbeat: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4*1024))
		return protocol.HeartbeatResponse{}, fmt.Errorf("heartbeat rejected: status=%d body=%s", resp.StatusCode, bytes.TrimSpace(respBody))
	}

	var hbResp protocol.HeartbeatResponse
	if err := json.NewDecoder(resp.Body).Decode(&hbResp); err != nil {
		return protocol.HeartbeatResponse{}, fmt.Errorf("decode heartbeat response: %w", err)
	}

	slog.Info("heartbeat sent", "agent_id", agentID, "os", runtime.GOOS, "arch", runtime.GOARCH)
	return hbResp, nil
}

func leaseJob(ctx context.Context, client *http.Client, serverURL, agentID string, capabilities map[string]string) (*protocol.Job, error) {
	caps := cloneMap(capabilities)
	if caps == nil {
		caps = map[string]string{}
	}
	if caps["executor"] == "" {
		caps["executor"] = executorScript
	}
	if caps["shells"] == "" {
		caps["shells"] = strings.Join(supportedShellsForRuntime(), ",")
	}
	if caps["os"] == "" {
		caps["os"] = runtime.GOOS
	}
	if caps["arch"] == "" {
		caps["arch"] = runtime.GOARCH
	}
	payload := protocol.LeaseJobRequest{
		AgentID:      agentID,
		Capabilities: caps,
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

	slog.Info("job leased", "job_execution_id", leaseResp.Job.ID)
	return leaseResp.Job, nil
}

func reportFailure(ctx context.Context, client *http.Client, serverURL, agentID string, job protocol.Job, exitCode *int, failMsg, output string) error {
	return reportTerminalJobStatusWithRetry(client, serverURL, job.ID, protocol.JobStatusUpdateRequest{
		AgentID:      agentID,
		Status:       "failed",
		ExitCode:     exitCode,
		Error:        failMsg,
		Output:       output,
		TimestampUTC: time.Now().UTC(),
	})
}

func reportTerminalJobStatusWithRetry(client *http.Client, serverURL, jobID string, reqBody protocol.JobStatusUpdateRequest) error {
	var lastErr error
	for attempt := 1; attempt <= terminalStatusMaxAttempts; attempt++ {
		attemptCtx, cancel := context.WithTimeout(context.Background(), terminalStatusAttemptTTL)
		err := reportJobStatus(attemptCtx, client, serverURL, jobID, reqBody)
		cancel()
		if err == nil {
			return nil
		}
		lastErr = err
		if attempt < terminalStatusMaxAttempts {
			wait := time.Duration(1<<(attempt-1)) * time.Second
			slog.Warn("terminal status report failed; retrying", "job_execution_id", jobID, "status", reqBody.Status, "attempt", attempt, "next_wait", wait, "error", err)
			time.Sleep(wait)
		}
	}
	return fmt.Errorf("terminal status report failed after %d attempts: %w", terminalStatusMaxAttempts, lastErr)
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

func cloneMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
