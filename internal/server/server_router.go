package server

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/izzyreal/ciwi/internal/server/webui"
)

func buildRouter(s *stateStore, artifactsDir string) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)

	// UI/static
	r.HandleFunc("/", webui.Handler)
	r.HandleFunc("/favicon.ico", webui.Handler)
	r.HandleFunc("/ciwi-favicon.png", webui.Handler)
	r.HandleFunc("/ciwi-logo.png", webui.Handler)
	r.HandleFunc("/ui/icons.svg", webui.Handler)
	r.HandleFunc("/ui/theme.js", webui.Handler)
	r.HandleFunc("/ui/shared.js", webui.Handler)
	r.HandleFunc("/ui/pages.js", webui.Handler)
	r.HandleFunc("/settings", webui.Handler)
	r.HandleFunc("/vault", webui.Handler)
	r.HandleFunc("/agents", webui.Handler)
	r.HandleFunc("/agents/*", webui.Handler)
	r.HandleFunc("/projects/*", webui.Handler)
	r.HandleFunc("/jobs/*", webui.Handler)

	// Health/info
	r.Get("/healthz", healthzHandler)
	r.Get("/api/v1/server-info", serverInfoHandler)
	r.Get("/api/v1/runtime-state", s.runtimeStateHandler)

	// Agent API
	r.Post("/api/v1/heartbeat", s.heartbeatHandler)
	r.Get("/api/v1/agents", s.listAgentsHandler)
	r.HandleFunc("/api/v1/agents/*", s.agentByIDHandler)
	r.Post("/api/v1/agent/lease", s.leaseJobHandler)

	// Project/pipeline APIs
	r.Post("/api/v1/projects/import", s.importProjectHandler)
	r.Post("/api/v1/projects/managed-yaml/validate", s.validateManagedYAMLHandler)
	r.Post("/api/v1/projects/managed-yaml", s.createManagedYAMLHandler)
	r.HandleFunc("/api/v1/projects", s.listProjectsHandler)
	r.HandleFunc("/api/v1/projects/*", s.projectByIDHandler)
	r.HandleFunc("/api/v1/pipelines/*", s.pipelineByIDHandler)

	// Vault APIs
	r.HandleFunc("/api/v1/vault/connections", s.vaultConnectionsHandler)
	r.HandleFunc("/api/v1/vault/connections/*", s.vaultConnectionByIDHandler)

	// Job APIs
	r.Get("/api/v1/job-queue/layout", s.jobQueueLayoutHandler)
	r.Get("/api/v1/job-queue/cards", s.jobQueueCardsHandler)
	r.Get("/api/v1/job-history/layout", s.jobHistoryLayoutHandler)
	r.Get("/api/v1/job-history/cards", s.jobHistoryCardsHandler)
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
