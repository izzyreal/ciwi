package server

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func buildRouter(s *stateStore, artifactsDir string) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)

	// UI/static
	r.HandleFunc("/", s.uiHandler)
	r.HandleFunc("/favicon.ico", s.uiHandler)
	r.HandleFunc("/ciwi-favicon.png", s.uiHandler)
	r.HandleFunc("/ciwi-logo.png", s.uiHandler)
	r.HandleFunc("/ui/shared.js", s.uiHandler)
	r.HandleFunc("/ui/pages.js", s.uiHandler)
	r.HandleFunc("/settings", s.uiHandler)
	r.HandleFunc("/vault", s.uiHandler)
	r.HandleFunc("/agents", s.uiHandler)
	r.HandleFunc("/agents/*", s.uiHandler)
	r.HandleFunc("/projects/*", s.uiHandler)
	r.HandleFunc("/jobs/*", s.uiHandler)

	// Health/info
	r.Get("/healthz", healthzHandler)
	r.Get("/api/v1/server-info", serverInfoHandler)

	// Agent API
	r.Post("/api/v1/heartbeat", s.heartbeatHandler)
	r.Get("/api/v1/agents", s.listAgentsHandler)
	r.HandleFunc("/api/v1/agents/*", s.agentByIDHandler)
	r.Post("/api/v1/agent/lease", s.leaseJobHandler)

	// Project/pipeline APIs
	r.Post("/api/v1/config/load", s.loadConfigHandler)
	r.Post("/api/v1/projects/import", s.importProjectHandler)
	r.HandleFunc("/api/v1/projects", s.listProjectsHandler)
	r.HandleFunc("/api/v1/projects/*", s.projectByIDHandler)
	r.Post("/api/v1/pipelines/run", s.runPipelineFromConfigHandler)
	r.HandleFunc("/api/v1/pipelines/*", s.pipelineByIDHandler)
	r.HandleFunc("/api/v1/pipeline-chains/*", s.pipelineChainByIDHandler)

	// Vault APIs
	r.HandleFunc("/api/v1/vault/connections", s.vaultConnectionsHandler)
	r.HandleFunc("/api/v1/vault/connections/*", s.vaultConnectionByIDHandler)

	// Job APIs
	r.Post("/api/v1/jobs/clear-queue", s.clearJobExecutionQueueHandler)
	r.Post("/api/v1/jobs/flush-history", s.flushJobExecutionHistoryHandler)
	r.HandleFunc("/api/v1/jobs", s.jobExecutionsHandler)
	r.HandleFunc("/api/v1/jobs/*", s.jobExecutionByIDHandler)

	// Update APIs
	r.Post("/api/v1/update/check", s.updateCheckHandler)
	r.Post("/api/v1/update/apply", s.updateApplyHandler)
	r.Post("/api/v1/update/rollback", s.updateRollbackHandler)
	r.Post("/api/v1/server/restart", s.serverRestartHandler)
	r.Get("/api/v1/update/tags", s.updateTagsHandler)
	r.Get("/api/v1/update/status", s.updateStatusHandler)

	r.Handle("/artifacts/*", http.StripPrefix("/artifacts/", http.FileServer(http.Dir(artifactsDir))))

	return r
}
