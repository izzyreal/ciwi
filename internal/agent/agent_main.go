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
			slog.Info("server requested agent update", "target_version", hb.UpdateTarget)
			if err := selfUpdateAndRestart(ctx, hb.UpdateTarget, hb.UpdateRepository, hb.UpdateAPIBase, os.Args[1:]); err != nil {
				slog.Error("agent self-update failed", "error", err)
				pendingUpdateFailure = err.Error()
			}
		}
	}

	for {
		select {
		case <-ctx.Done():
			return nil
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
					slog.Info("server requested agent update", "target_version", hb.UpdateTarget)
					if err := selfUpdateAndRestart(ctx, hb.UpdateTarget, hb.UpdateRepository, hb.UpdateAPIBase, os.Args[1:]); err != nil {
						slog.Error("agent self-update failed", "error", err)
						pendingUpdateFailure = err.Error()
					}
				}
			}
		case <-leaseTicker.C:
			job, err := leaseJob(ctx, client, serverURL, agentID, capabilities)
			if err != nil {
				slog.Error("lease failed", "error", err)
				continue
			}
			if job == nil {
				continue
			}
			if err := executeLeasedJob(ctx, client, serverURL, agentID, workDir, capabilities, *job); err != nil {
				slog.Error("execute job failed", "job_execution_id", job.ID, "error", err)
			}
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
