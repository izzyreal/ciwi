package server

import (
	"net/http"

	"github.com/izzyreal/ciwi/internal/server/jobexecution"
)

func (s *stateStore) jobExecutionHandlerDeps() jobexecution.HandlerDeps {
	return jobexecution.HandlerDeps{
		Store:                              s.jobExecutionStore(),
		ArtifactsDir:                       s.artifactsDir,
		AttachTestSummaries:                s.attachJobExecutionTestSummaries,
		AttachUnmetRequirements:            s.attachJobExecutionUnmetRequirements,
		AttachTestSummary:                  s.attachJobExecutionTestSummary,
		AttachUnmetRequirementsToExecution: s.attachJobExecutionUnmetRequirementsToJobExecution,
		MarkAgentSeen:                      s.markAgentSeen,
	}
}

func (s *stateStore) jobExecutionsHandler(w http.ResponseWriter, r *http.Request) {
	jobexecution.HandleCollection(w, r, s.jobExecutionHandlerDeps())
}

func (s *stateStore) jobExecutionByIDHandler(w http.ResponseWriter, r *http.Request) {
	jobexecution.HandleByID(w, r, s.jobExecutionHandlerDeps())
}

func (s *stateStore) clearJobExecutionQueueHandler(w http.ResponseWriter, r *http.Request) {
	jobexecution.HandleClearQueue(w, r, s.jobExecutionHandlerDeps())
}

func (s *stateStore) flushJobExecutionHistoryHandler(w http.ResponseWriter, r *http.Request) {
	jobexecution.HandleFlushHistory(w, r, s.jobExecutionHandlerDeps())
}
