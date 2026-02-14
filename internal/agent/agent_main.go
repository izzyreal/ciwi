package agent

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/izzyreal/ciwi/internal/protocol"
)

func Run(ctx context.Context) error {
	loadAgentPlatformEnv()
	if handled, err := runAsWindowsServiceIfNeeded(runLoop); handled {
		return err
	}
	return runLoop(ctx)
}

func runLoop(ctx context.Context) error {
	serverURL := envOrDefault("CIWI_SERVER_URL", "http://127.0.0.1:8112")
	agentID := envOrDefault("CIWI_AGENT_ID", defaultAgentID())
	hostname, _ := os.Hostname()
	workDir := envOrDefault("CIWI_AGENT_WORKDIR", ".ciwi-agent")

	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return fmt.Errorf("create agent workdir: %w", err)
	}
	if reason := selfUpdateWritabilityWarning(); reason != "" {
		slog.Warn("agent self-update preflight warning", "reason", reason)
	}

	slog.Info("ciwi agent started", "agent_id", agentID, "server_url", serverURL)
	defer slog.Info("ciwi agent stopped", "agent_id", agentID)

	client := &http.Client{Timeout: 10 * time.Minute}
	heartbeatTicker := time.NewTicker(protocol.AgentHeartbeatInterval)
	defer heartbeatTicker.Stop()
	leaseTicker := time.NewTicker(3 * time.Second)
	defer leaseTicker.Stop()
	capabilities := detectAgentCapabilities()
	pendingUpdateFailure := ""
	jobInProgress := false
	pendingRestart := false
	type pendingUpdateRequest struct {
		target     string
		repository string
		apiBase    string
	}
	type jobResult struct {
		jobID string
		err   error
	}
	var pendingUpdate *pendingUpdateRequest
	jobDoneCh := make(chan jobResult, 1)

	runOrDeferUpdate := func(target, repository, apiBase string) {
		target = strings.TrimSpace(target)
		if target == "" {
			return
		}
		if jobInProgress {
			pendingUpdate = &pendingUpdateRequest{
				target:     target,
				repository: repository,
				apiBase:    apiBase,
			}
			slog.Info("server requested agent update; deferring until current job completes", "target_version", target)
			return
		}
		slog.Info("server requested agent update", "target_version", target)
		if err := selfUpdateAndRestart(ctx, target, repository, apiBase, os.Args[1:]); err != nil {
			slog.Error("agent self-update failed", "error", err)
			pendingUpdateFailure = err.Error()
		}
	}
	runOrDeferRestart := func() {
		if jobInProgress {
			pendingRestart = true
			slog.Info("server requested agent restart; deferring until current job completes")
			return
		}
		slog.Info("server requested agent restart")
		restartAgentProcess()
	}

	if hb, err := sendHeartbeat(ctx, client, serverURL, agentID, hostname, capabilities, pendingUpdateFailure); err != nil {
		slog.Error("initial heartbeat failed", "error", err)
	} else {
		pendingUpdateFailure = ""
		if hb.RefreshToolsRequested {
			capabilities = detectAgentCapabilities()
			slog.Info("server requested tools refresh")
			if _, err := sendHeartbeat(ctx, client, serverURL, agentID, hostname, capabilities, pendingUpdateFailure); err != nil {
				slog.Error("heartbeat failed", "error", err)
			} else {
				pendingUpdateFailure = ""
			}
		}
		if hb.UpdateRequested {
			runOrDeferUpdate(hb.UpdateTarget, hb.UpdateRepository, hb.UpdateAPIBase)
		}
		if hb.RestartRequested {
			runOrDeferRestart()
		}
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case done := <-jobDoneCh:
			jobInProgress = false
			if done.err != nil {
				slog.Error("execute job failed", "job_execution_id", done.jobID, "error", done.err)
			}
			if pendingUpdate != nil {
				req := *pendingUpdate
				pendingUpdate = nil
				runOrDeferUpdate(req.target, req.repository, req.apiBase)
			}
			if pendingRestart {
				pendingRestart = false
				runOrDeferRestart()
			}
		case <-heartbeatTicker.C:
			hb, err := sendHeartbeat(ctx, client, serverURL, agentID, hostname, capabilities, pendingUpdateFailure)
			if err != nil {
				slog.Error("heartbeat failed", "error", err)
			} else {
				pendingUpdateFailure = ""
				if hb.RefreshToolsRequested {
					capabilities = detectAgentCapabilities()
					slog.Info("server requested tools refresh")
					if _, err := sendHeartbeat(ctx, client, serverURL, agentID, hostname, capabilities, pendingUpdateFailure); err != nil {
						slog.Error("heartbeat failed", "error", err)
					} else {
						pendingUpdateFailure = ""
					}
				}
				if hb.UpdateRequested {
					runOrDeferUpdate(hb.UpdateTarget, hb.UpdateRepository, hb.UpdateAPIBase)
				}
				if hb.RestartRequested {
					runOrDeferRestart()
				}
			}
		case <-leaseTicker.C:
			if jobInProgress {
				continue
			}
			job, err := leaseJob(ctx, client, serverURL, agentID, capabilities)
			if err != nil {
				slog.Error("lease failed", "error", err)
				continue
			}
			if job == nil {
				continue
			}
			jobInProgress = true
			jobCaps := cloneMap(capabilities)
			go func(leased protocol.JobExecution, caps map[string]string) {
				jobDoneCh <- jobResult{
					jobID: leased.ID,
					err:   executeLeasedJob(ctx, client, serverURL, agentID, workDir, caps, leased),
				}
			}(*job, jobCaps)
		}
	}
}

func selfUpdateWritabilityWarning() string {
	exePath, err := os.Executable()
	if err != nil {
		return "cannot resolve executable path: " + err.Error()
	}
	if looksLikeGoRunBinary(exePath) {
		return "running via go run binary path; self-update is unavailable"
	}
	f, err := os.OpenFile(exePath, os.O_WRONLY, 0)
	if err != nil {
		return "binary path is not writable by current user (" + strings.TrimSpace(exePath) + "): " + err.Error()
	}
	_ = f.Close()
	return ""
}
