package server

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/izzyreal/ciwi/internal/store"
)

const (
	jobStatusRunning   = "running"
	jobStatusSucceeded = "succeeded"
	jobStatusFailed    = "failed"
)

type agentState struct {
	Hostname     string            `json:"hostname"`
	OS           string            `json:"os"`
	Arch         string            `json:"arch"`
	Version      string            `json:"version,omitempty"`
	Capabilities map[string]string `json:"capabilities"`
	LastSeenUTC  time.Time         `json:"last_seen_utc"`
}

type stateStore struct {
	mu           sync.Mutex
	agents       map[string]agentState
	db           *store.Store
	artifactsDir string
	vaultTokens  *vaultTokenCache
	update       updateState
}

func Run(ctx context.Context) error {
	addr := envOrDefault("CIWI_SERVER_ADDR", ":8112")
	dbPath := envOrDefault("CIWI_DB_PATH", "ciwi.db")
	artifactsDir := envOrDefault("CIWI_ARTIFACTS_DIR", "ciwi-artifacts")

	db, err := store.Open(dbPath)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer db.Close()

	if err := os.MkdirAll(artifactsDir, 0o755); err != nil {
		return fmt.Errorf("create artifacts dir: %w", err)
	}

	s := &stateStore{agents: make(map[string]agentState), db: db, artifactsDir: artifactsDir, vaultTokens: newVaultTokenCache()}

	mux := http.NewServeMux()
	mux.HandleFunc("/", s.uiHandler)
	mux.HandleFunc("/healthz", healthzHandler)
	mux.HandleFunc("/api/v1/server-info", serverInfoHandler)
	mux.HandleFunc("/api/v1/heartbeat", s.heartbeatHandler)
	mux.HandleFunc("/api/v1/agents", s.listAgentsHandler)
	mux.HandleFunc("/api/v1/config/load", s.loadConfigHandler)
	mux.HandleFunc("/api/v1/projects/import", s.importProjectHandler)
	mux.HandleFunc("/api/v1/projects", s.listProjectsHandler)
	mux.HandleFunc("/api/v1/projects/", s.projectByIDHandler)
	mux.HandleFunc("/api/v1/vault/connections", s.vaultConnectionsHandler)
	mux.HandleFunc("/api/v1/vault/connections/", s.vaultConnectionByIDHandler)
	mux.HandleFunc("/api/v1/jobs", s.jobsHandler)
	mux.HandleFunc("/api/v1/jobs/", s.jobByIDHandler)
	mux.HandleFunc("/api/v1/jobs/clear-queue", s.clearQueueHandler)
	mux.HandleFunc("/api/v1/jobs/flush-history", s.flushHistoryHandler)
	mux.HandleFunc("/api/v1/agent/lease", s.leaseJobHandler)
	mux.HandleFunc("/api/v1/pipelines/run", s.runPipelineFromConfigHandler)
	mux.HandleFunc("/api/v1/pipelines/", s.pipelineByIDHandler)
	mux.HandleFunc("/api/v1/update/check", s.updateCheckHandler)
	mux.HandleFunc("/api/v1/update/apply", s.updateApplyHandler)
	mux.HandleFunc("/api/v1/update/status", s.updateStatusHandler)
	mux.Handle("/artifacts/", http.StripPrefix("/artifacts/", http.FileServer(http.Dir(artifactsDir))))

	srv := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	stopMDNS := startMDNSAdvertiser(addr)
	defer stopMDNS()

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
