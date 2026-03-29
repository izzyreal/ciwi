package server

import (
	"net/http"

	"github.com/izzyreal/ciwi/internal/server/jobhistory"
)

func (s *stateStore) jobHistoryHandlerDeps() jobhistory.HandlerDeps {
	return jobhistory.HandlerDeps{
		Store:                   s.jobExecutionStore(),
		AttachTestSummaries:     s.attachJobExecutionTestSummaries,
		AttachUnmetRequirements: s.attachJobExecutionUnmetRequirements,
	}
}

func (s *stateStore) jobHistoryLayoutHandler(w http.ResponseWriter, r *http.Request) {
	jobhistory.HandleLayout(w, r, s.jobHistoryHandlerDeps())
}

func (s *stateStore) jobHistoryCardsHandler(w http.ResponseWriter, r *http.Request) {
	jobhistory.HandleCards(w, r, s.jobHistoryHandlerDeps())
}

func (s *stateStore) jobQueueLayoutHandler(w http.ResponseWriter, r *http.Request) {
	jobhistory.HandleQueueLayout(w, r, s.jobHistoryHandlerDeps())
}

func (s *stateStore) jobQueueCardsHandler(w http.ResponseWriter, r *http.Request) {
	jobhistory.HandleQueueCards(w, r, s.jobHistoryHandlerDeps())
}
