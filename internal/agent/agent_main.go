package agent

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"
)

func Run(ctx context.Context) error {
	serverURL := envOrDefault("CIWI_SERVER_URL", "http://127.0.0.1:8112")
	agentID := envOrDefault("CIWI_AGENT_ID", defaultAgentID())
	hostname, _ := os.Hostname()
	workDir := envOrDefault("CIWI_AGENT_WORKDIR", ".ciwi-agent")

	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return fmt.Errorf("create agent workdir: %w", err)
	}

	slog.Info("ciwi agent started", "agent_id", agentID, "server_url", serverURL)
	defer slog.Info("ciwi agent stopped", "agent_id", agentID)

	client := &http.Client{Timeout: 30 * time.Second}
	heartbeatTicker := time.NewTicker(10 * time.Second)
	defer heartbeatTicker.Stop()
	leaseTicker := time.NewTicker(3 * time.Second)
	defer leaseTicker.Stop()

	if hb, err := sendHeartbeat(ctx, client, serverURL, agentID, hostname); err != nil {
		slog.Error("initial heartbeat failed", "error", err)
	} else {
		if hb.UpdateRequested {
			slog.Info("server requested agent update", "target_version", hb.UpdateTarget)
			if err := selfUpdateAndRestart(ctx, hb.UpdateTarget, hb.UpdateRepository, hb.UpdateAPIBase, os.Args[1:]); err != nil {
				slog.Error("agent self-update failed", "error", err)
			}
		}
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-heartbeatTicker.C:
			hb, err := sendHeartbeat(ctx, client, serverURL, agentID, hostname)
			if err != nil {
				slog.Error("heartbeat failed", "error", err)
			} else if hb.UpdateRequested {
				slog.Info("server requested agent update", "target_version", hb.UpdateTarget)
				if err := selfUpdateAndRestart(ctx, hb.UpdateTarget, hb.UpdateRepository, hb.UpdateAPIBase, os.Args[1:]); err != nil {
					slog.Error("agent self-update failed", "error", err)
				}
			}
		case <-leaseTicker.C:
			job, err := leaseJob(ctx, client, serverURL, agentID)
			if err != nil {
				slog.Error("lease failed", "error", err)
				continue
			}
			if job == nil {
				continue
			}
			if err := executeLeasedJob(ctx, client, serverURL, agentID, workDir, *job); err != nil {
				slog.Error("execute job failed", "job_execution_id", job.ID, "error", err)
			}
		}
	}
}
