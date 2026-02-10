package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/izzyreal/ciwi/internal/protocol"
)

type agentState struct {
	Hostname     string            `json:"hostname"`
	OS           string            `json:"os"`
	Arch         string            `json:"arch"`
	Capabilities map[string]string `json:"capabilities"`
	LastSeenUTC  time.Time         `json:"last_seen_utc"`
}

func Run(ctx context.Context) error {
	addr := envOrDefault("CIWI_SERVER_ADDR", ":8080")

	mux := http.NewServeMux()
	store := &agentStore{agents: make(map[string]agentState)}

	mux.HandleFunc("/healthz", healthzHandler)
	mux.HandleFunc("/api/v1/heartbeat", store.heartbeatHandler)

	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Printf("ciwi server started on %s", addr)
		err := srv.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("listen and serve: %w", err)
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutdown server: %w", err)
		}
		log.Println("ciwi server stopped")
		return nil
	case err := <-errCh:
		if err != nil {
			return err
		}
		log.Println("ciwi server stopped")
		return nil
	}
}

type agentStore struct {
	mu     sync.Mutex
	agents map[string]agentState
}

func (s *agentStore) heartbeatHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var hb protocol.HeartbeatRequest
	if err := json.NewDecoder(r.Body).Decode(&hb); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}

	if hb.AgentID == "" {
		http.Error(w, "agent_id is required", http.StatusBadRequest)
		return
	}

	if hb.TimestampUTC.IsZero() {
		hb.TimestampUTC = time.Now().UTC()
	}

	s.mu.Lock()
	s.agents[hb.AgentID] = agentState{
		Hostname:     hb.Hostname,
		OS:           hb.OS,
		Arch:         hb.Arch,
		Capabilities: hb.Capabilities,
		LastSeenUTC:  hb.TimestampUTC,
	}
	s.mu.Unlock()

	log.Printf("heartbeat: agent_id=%s hostname=%s os=%s arch=%s", hb.AgentID, hb.Hostname, hb.OS, hb.Arch)

	writeJSON(w, http.StatusOK, protocol.HeartbeatResponse{Accepted: true})
}

func healthzHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("encode JSON response: %v", err)
	}
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
