package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"runtime"
	"time"

	"github.com/izzyreal/ciwi/internal/protocol"
)

func Run(ctx context.Context) error {
	serverURL := envOrDefault("CIWI_SERVER_URL", "http://127.0.0.1:8080")
	agentID := envOrDefault("CIWI_AGENT_ID", defaultAgentID())
	hostname, _ := os.Hostname()

	log.Printf("ciwi agent started: id=%s server=%s", agentID, serverURL)
	defer log.Println("ciwi agent stopped")

	client := &http.Client{Timeout: 10 * time.Second}
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	if err := sendHeartbeat(ctx, client, serverURL, agentID, hostname); err != nil {
		log.Printf("initial heartbeat failed: %v", err)
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := sendHeartbeat(ctx, client, serverURL, agentID, hostname); err != nil {
				log.Printf("heartbeat failed: %v", err)
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
