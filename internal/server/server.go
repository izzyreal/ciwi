package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"sync"
	"time"

	servervault "github.com/izzyreal/ciwi/internal/server/vault"
	"github.com/izzyreal/ciwi/internal/store"
)

type agentState struct {
	Hostname             string            `json:"hostname"`
	OS                   string            `json:"os"`
	Arch                 string            `json:"arch"`
	Version              string            `json:"version,omitempty"`
	Capabilities         map[string]string `json:"capabilities"`
	LastSeenUTC          time.Time         `json:"last_seen_utc"`
	RecentLog            []string          `json:"recent_log,omitempty"`
	UpdateTarget         string            `json:"update_target,omitempty"`
	UpdateSource         string            `json:"update_source,omitempty"`
	UpdateAttempts       int               `json:"update_attempts,omitempty"`
	UpdateInProgress     bool              `json:"update_in_progress,omitempty"`
	UpdateLastRequestUTC time.Time         `json:"update_last_request_utc,omitempty"`
	UpdateNextRetryUTC   time.Time         `json:"update_next_retry_utc,omitempty"`
	UpdateLastError      string            `json:"update_last_error,omitempty"`
	UpdateLastErrorUTC   time.Time         `json:"update_last_error_utc,omitempty"`
}

type agentUpdateRolloutState struct {
	Target     string
	StartedUTC time.Time
	NextSlot   int
	Slots      map[string]int
}

type stateStore struct {
	mu               sync.Mutex
	agents           map[string]agentState
	agentUpdates     map[string]string
	agentToolRefresh map[string]bool
	agentRestarts    map[string]bool
	agentRollout     agentUpdateRolloutState
	projectIcons     map[int64]projectIconState
	db               *store.Store
	artifactsDir     string
	vaultTokens      *servervault.TokenCache
	update           updateState
	restartServerFn  func()
}

type projectIconState struct {
	ContentType string
	Data        []byte
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

	s := &stateStore{
		agents:           make(map[string]agentState),
		agentUpdates:     make(map[string]string),
		agentToolRefresh: make(map[string]bool),
		agentRestarts:    make(map[string]bool),
		agentRollout: agentUpdateRolloutState{
			Slots: make(map[string]int),
		},
		projectIcons: make(map[int64]projectIconState),
		db:           db,
		artifactsDir: artifactsDir,
		vaultTokens:  servervault.NewTokenCache(),
		restartServerFn: func() {
			os.Exit(0)
		},
	}
	if target, ok, err := db.GetAppState("agent_update_target"); err == nil && ok {
		s.update.mu.Lock()
		s.update.agentTarget = target
		s.update.mu.Unlock()
	}
	s.maybeRunPostUpdateProjectReload(ctx)
	if err := s.runJobExecutionMaintenancePass(time.Now().UTC()); err != nil {
		slog.Error("initial job execution maintenance pass failed", "error", err)
	}
	go s.runJobExecutionMaintenanceLoop(ctx)
	srv := &http.Server{Addr: addr, Handler: buildRouter(s, artifactsDir), ReadHeaderTimeout: 10 * time.Second}
	stopMDNS := startMDNSAdvertiser(addr)
	defer stopMDNS()

	errCh := make(chan error, 1)
	go func() {
		slog.Info("ciwi server started", "addr", addr)
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
		slog.Info("ciwi server stopped")
		return nil
	case err := <-errCh:
		if err != nil {
			return err
		}
		slog.Info("ciwi server stopped")
		return nil
	}
}
