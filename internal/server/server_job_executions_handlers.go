package server

import (
	"net/http"
	"strings"

	"github.com/izzyreal/ciwi/internal/application"
	"github.com/izzyreal/ciwi/internal/protocol"
	"github.com/izzyreal/ciwi/internal/server/jobexecution"
)

func (s *stateStore) jobExecutionHandlerDeps() jobexecution.HandlerDeps {
	attachProgress := func(*protocol.JobExecution) {}
	if s.jobProgress != nil {
		attachProgress = func(job *protocol.JobExecution) { _ = s.jobProgress.AttachDetailEstimate(job) }
	}
	return jobexecution.HandlerDeps{
		Store:                     s.jobExecutionStore(),
		ExecutionCommands:         s.app().executionCommands,
		ExecutionControls:         s.app().executionControls,
		ArtifactsDir:              s.artifactsDir,
		AttachTestSummaries:       s.attachJobExecutionTestSummaries,
		AttachSchedulingDiagnoses: s.attachJobExecutionSchedulingDiagnoses,
		AttachTestSummary:         s.attachJobExecutionTestSummary,
		AttachSchedulingDiagnosis: s.attachJobExecutionSchedulingDiagnosis,
		MarkAgentSeen:             s.markAgentSeen,
		OnJobUpdated:              s.onJobExecutionUpdated,
		OnJobStateChanged: func(job protocol.JobExecution) {
			s.app().changes.PublishForJobExecution(job.ID, application.ChangeQueue, application.ChangeHistory)
		},
		OnQueueChanged: func() {
			s.app().changes.Publish(application.ChangeQueue)
		},
		OnHistoryChanged: func() {
			s.app().changes.Publish(application.ChangeHistory)
		},
		OnJobHistoryChanged: func(jobExecutionID string) {
			s.app().changes.PublishForJobExecution(jobExecutionID, application.ChangeHistory)
		},
		PrepareRerun:   s.prepareJobExecutionRerun,
		AttachProgress: attachProgress,
	}
}

func (s *stateStore) jobExecutionsHandler(w http.ResponseWriter, r *http.Request) {
	jobexecution.HandleCollection(w, r, s.jobExecutionHandlerDeps())
}

func (s *stateStore) jobExecutionByIDHandler(w http.ResponseWriter, r *http.Request) {
	const graphContextSuffix = "/graph-context"
	path := strings.TrimSuffix(strings.TrimSpace(r.URL.Path), "/")
	if strings.HasSuffix(path, graphContextSuffix) {
		prefix := strings.TrimSuffix(path, graphContextSuffix)
		jobID := strings.Trim(strings.TrimPrefix(prefix, "/api/v1/jobs/"), "/")
		if jobID == "" || strings.Contains(jobID, "/") {
			http.NotFound(w, r)
			return
		}
		s.jobExecutionGraphContextHandler(w, r, jobID)
		return
	}
	jobexecution.HandleByID(w, r, s.jobExecutionHandlerDeps())
}

func (s *stateStore) clearJobExecutionQueueHandler(w http.ResponseWriter, r *http.Request) {
	jobexecution.HandleClearQueue(w, r, s.jobExecutionHandlerDeps())
}

func (s *stateStore) flushJobExecutionHistoryHandler(w http.ResponseWriter, r *http.Request) {
	jobexecution.HandleFlushHistory(w, r, s.jobExecutionHandlerDeps())
}
