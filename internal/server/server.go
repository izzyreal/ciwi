package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/izzyreal/ciwi/internal/store"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

const (
	jobStatusRunning   = "running"
	jobStatusSucceeded = "succeeded"
	jobStatusFailed    = "failed"
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
	UpdateAttempts       int               `json:"update_attempts,omitempty"`
	UpdateLastRequestUTC time.Time         `json:"update_last_request_utc,omitempty"`
	UpdateNextRetryUTC   time.Time         `json:"update_next_retry_utc,omitempty"`
}

type stateStore struct {
	mu               sync.Mutex
	agents           map[string]agentState
	agentUpdates     map[string]string
	agentToolRefresh map[string]bool
	db               *store.Store
	artifactsDir     string
	vaultTokens      *vaultTokenCache
	update           updateState
}

func Run(ctx context.Context) error {
	addr := envOrDefault("CIWI_SERVER_ADDR", ":8112")
	grpcAddr := strings.TrimSpace(envOrDefault("CIWI_GRPC_ADDR", ":8113"))
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
		db:               db,
		artifactsDir:     artifactsDir,
		vaultTokens:      newVaultTokenCache(),
	}
	if target, ok, err := db.GetAppState("agent_update_target"); err == nil && ok {
		s.update.mu.Lock()
		s.update.agentTarget = target
		s.update.mu.Unlock()
	}
	router := buildRouter(s, artifactsDir)
	srv := &http.Server{Addr: addr, Handler: router, ReadHeaderTimeout: 10 * time.Second}
	var grpcSrv *grpc.Server
	var grpcListener net.Listener
	if grpcAddr != "" {
		var err error
		grpcListener, err = net.Listen("tcp", grpcAddr)
		if err != nil {
			return fmt.Errorf("listen grpc: %w", err)
		}
		grpcSrv = grpc.NewServer()
		registerCiwiGRPCService(grpcSrv, newCiwiGRPCServer(router))
		reflection.Register(grpcSrv)
	}
	stopMDNS := startMDNSAdvertiser(addr)
	defer stopMDNS()

	httpErrCh := make(chan error, 1)
	go func() {
		slog.Info("ciwi server started", "addr", addr)
		err := srv.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			httpErrCh <- fmt.Errorf("listen and serve: %w", err)
			return
		}
		httpErrCh <- nil
	}()
	var grpcErrCh chan error
	if grpcSrv != nil {
		grpcErrCh = make(chan error, 1)
		go func() {
			slog.Info("ciwi gRPC server started", "addr", grpcAddr)
			err := grpcSrv.Serve(grpcListener)
			if err != nil && !errors.Is(err, grpc.ErrServerStopped) {
				grpcErrCh <- fmt.Errorf("serve grpc: %w", err)
				return
			}
			grpcErrCh <- nil
		}()
	}

	select {
	case <-ctx.Done():
		if grpcSrv != nil {
			done := make(chan struct{})
			go func() {
				grpcSrv.GracefulStop()
				close(done)
			}()
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				grpcSrv.Stop()
			}
		}
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutdown server: %w", err)
		}
		slog.Info("ciwi server stopped")
		return nil
	case err := <-httpErrCh:
		if grpcSrv != nil {
			grpcSrv.Stop()
		}
		if err != nil {
			return err
		}
		slog.Info("ciwi server stopped")
		return nil
	case err := <-grpcErrCh:
		if err != nil {
			_ = srv.Close()
			return err
		}
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
		slog.Info("ciwi gRPC server stopped")
		return nil
	}
}
