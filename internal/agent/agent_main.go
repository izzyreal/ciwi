package agent

import (
	"context"
	"fmt"
	"log"
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
