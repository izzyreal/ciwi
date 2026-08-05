package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/izzyreal/ciwi/internal/adapters/nativecnp"
	"github.com/izzyreal/ciwi/internal/adapters/nativequic"
	"github.com/izzyreal/ciwi/internal/adapters/nativetcp"
	"github.com/izzyreal/ciwi/internal/server/jobprogress"
	servervault "github.com/izzyreal/ciwi/internal/server/vault"
	"github.com/izzyreal/ciwi/internal/store"
)

type agentState struct {
	Hostname             string            `json:"hostname"`
	OS                   string            `json:"os"`
	Arch                 string            `json:"arch"`
	Version              string            `json:"version,omitempty"`
	Authorized           bool              `json:"authorized"`
	Deactivated          bool              `json:"deactivated,omitempty"`
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
	mu                sync.Mutex
	applicationOnce   sync.Once
	application       *serverApplication
	dependencyMu      sync.Mutex
	agents            map[string]agentState
	agentUpdates      map[string]string
	agentToolRefresh  map[string]bool
	agentRestarts     map[string]bool
	agentCacheWipes   map[string]bool
	agentHistoryWipes map[string]bool
	agentDeactivated  map[string]bool
	agentRollout      agentUpdateRolloutState
	projectIcons      map[int64]projectIconState
	db                *store.Store
	artifactsDir      string
	vaultTokens       *servervault.TokenCache
	jobProgress       *jobprogress.Estimator
	update            updateState
	restartServerFn   func()
	installationID    string
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
	installationID, err := ensureServerInstallationID(db)
	if err != nil {
		return err
	}
	if err := db.MarkPendingCommandReceiptsUnknown(); err != nil {
		return fmt.Errorf("recover command receipts: %w", err)
	}

	if err := os.MkdirAll(artifactsDir, 0o755); err != nil {
		return fmt.Errorf("create artifacts dir: %w", err)
	}

	s := &stateStore{
		agents:            make(map[string]agentState),
		agentUpdates:      make(map[string]string),
		agentToolRefresh:  make(map[string]bool),
		agentRestarts:     make(map[string]bool),
		agentCacheWipes:   make(map[string]bool),
		agentHistoryWipes: make(map[string]bool),
		agentDeactivated:  make(map[string]bool),
		agentRollout: agentUpdateRolloutState{
			Slots: make(map[string]int),
		},
		projectIcons: make(map[int64]projectIconState),
		db:           db,
		artifactsDir: artifactsDir,
		vaultTokens:  servervault.NewTokenCache(),
		jobProgress:  jobprogress.New(db),
		restartServerFn: func() {
			os.Exit(0)
		},
		installationID: installationID,
	}
	if target, ok, err := db.GetAppState("agent_update_target"); err == nil && ok {
		s.update.mu.Lock()
		s.update.agentTarget = target
		s.update.mu.Unlock()
	}
	if appState, err := db.ListAppState(); err == nil {
		s.hydrateAgentStateFromAppState(appState)
	}
	go s.warmProjectIconsOnStartup(ctx)
	s.maybeRunPostUpdateProjectReload(ctx)
	if err := s.runJobExecutionMaintenancePass(time.Now().UTC()); err != nil {
		slog.Error("initial job execution maintenance pass failed", "error", err)
	}
	if err := s.reconcileBlockedJobExecutions(); err != nil {
		slog.Error("initial blocked job reconciliation failed", "error", err)
	}
	go s.runJobExecutionMaintenanceLoop(ctx)
	srv := &http.Server{Addr: addr, Handler: buildRouter(s, artifactsDir), ReadHeaderTimeout: 10 * time.Second}
	stopMDNS := startMDNSAdvertiser(addr)
	defer stopMDNS()

	var nativeQUICServer *nativequic.Server
	var nativeTCPServer *nativetcp.Server
	nativeAddresses := nativeListenAddresses()
	if nativeAddresses.QUIC != "" || nativeAddresses.TCP != "" {
		app := s.app()
		handler, handlerErr := nativecnp.NewHandler(nativecnp.Services{
			Server: app.server, Projects: app.projects, ProjectCommands: app.projectCommands, Updates: app.updates, FrontPage: app.frontPage,
			ProjectDetails: app.projectDetails,
			ProjectIcons:   s,
			JobDetails:     app.jobDetails,
			Pipelines:      app.pipelines, PipelineChains: app.pipelineChains,
			RunOptions:        app.runOptions,
			Agents:            app.agents,
			AgentCommands:     app.agentCommands,
			AgentScripts:      app.agentScripts,
			ExecutionCommands: app.executionCommands, ExecutionControls: app.executionControls,
			CommandReceipts: app.commandReceipts,
			Changes:         app.changes, Version: currentVersion(),
		})
		if handlerErr != nil {
			return handlerErr
		}
		tlsConfig, tlsErr := nativecnp.ServerTLSConfig()
		if tlsErr != nil {
			return fmt.Errorf("create native TLS configuration: %w", tlsErr)
		}
		if nativeAddresses.QUIC != "" {
			nativeQUICServer, err = nativequic.ListenWithHandler(nativeAddresses.QUIC, handler, tlsConfig)
			if err != nil {
				return err
			}
			defer nativeQUICServer.Close()
			stopNativeQUICMDNS := startNativeMDNSAdvertiser(nativeQUICServer.Addr(), "quic")
			defer stopNativeQUICMDNS()
		}
		if nativeAddresses.TCP != "" {
			nativeTCPServer, err = nativetcp.ListenWithHandler(nativeAddresses.TCP, handler, tlsConfig)
			if err != nil {
				if nativeQUICServer != nil {
					_ = nativeQUICServer.Close()
				}
				return err
			}
			defer nativeTCPServer.Close()
			stopNativeTCPMDNS := startNativeMDNSAdvertiser(nativeTCPServer.Addr(), "tcp")
			defer stopNativeTCPMDNS()
		}
	}

	errCh := make(chan error, 3)
	go func() {
		slog.Info("ciwi server started", "addr", addr)
		err := srv.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("listen and serve: %w", err)
			return
		}
		errCh <- nil
	}()
	if nativeQUICServer != nil {
		go func() {
			slog.Info("ciwi native server started", "addr", nativeQUICServer.Addr(), "transport", "quic", "protocol", nativequic.ALPN)
			errCh <- nativeQUICServer.Serve(ctx)
		}()
	}
	if nativeTCPServer != nil {
		go func() {
			slog.Info("ciwi native server started", "addr", nativeTCPServer.Addr(), "transport", "tcp", "protocol", nativetcp.ALPN)
			errCh <- nativeTCPServer.Serve(ctx)
		}()
	}

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutdown server: %w", err)
		}
		if nativeQUICServer != nil {
			_ = nativeQUICServer.Close()
		}
		if nativeTCPServer != nil {
			_ = nativeTCPServer.Close()
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

func nativeListenAddr() string {
	value, configured := os.LookupEnv("CIWI_NATIVE_ADDR")
	if !configured {
		value = ":8113"
	}
	return normalizedNativeListenAddr(value)
}

type nativeAddresses struct {
	QUIC string
	TCP  string
}

func nativeListenAddresses() nativeAddresses {
	common := nativeListenAddr()
	return nativeAddresses{
		QUIC: nativeTransportListenAddr("CIWI_NATIVE_QUIC_ADDR", common),
		TCP:  nativeTransportListenAddr("CIWI_NATIVE_TCP_ADDR", common),
	}
}

func nativeTransportListenAddr(key, fallback string) string {
	value, configured := os.LookupEnv(key)
	if !configured {
		return fallback
	}
	return normalizedNativeListenAddr(value)
}

func normalizedNativeListenAddr(value string) string {
	value = strings.TrimSpace(value)
	switch strings.ToLower(value) {
	case "", "off", "disabled", "false", "none":
		return ""
	default:
		return value
	}
}
