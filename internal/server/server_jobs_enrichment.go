package server

import (
	"strings"
	"time"

	"github.com/izzyreal/ciwi/internal/protocol"
)

func (s *stateStore) attachTestSummaries(jobs []protocol.Job) {
	for i := range jobs {
		s.attachTestSummary(&jobs[i])
	}
}

func (s *stateStore) markAgentSeen(agentID string, ts time.Time) {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return
	}
	if ts.IsZero() {
		ts = time.Now().UTC()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.agents[agentID]
	if !ok {
		return
	}
	a.LastSeenUTC = ts
	s.agents[agentID] = a
}

func (s *stateStore) attachTestSummary(job *protocol.Job) {
	if job == nil || strings.TrimSpace(job.ID) == "" {
		return
	}
	report, found, err := s.db.GetJobTestReport(job.ID)
	if err != nil || !found {
		return
	}
	job.TestSummary = &protocol.JobTestSummary{
		Total:   report.Total,
		Passed:  report.Passed,
		Failed:  report.Failed,
		Skipped: report.Skipped,
	}
}

func (s *stateStore) attachUnmetRequirements(jobs []protocol.Job) {
	s.mu.Lock()
	agents := make(map[string]agentState, len(s.agents))
	for id, a := range s.agents {
		agents[id] = a
	}
	s.mu.Unlock()
	for i := range jobs {
		if !protocol.IsQueuedJobStatus(jobs[i].Status) {
			continue
		}
		jobs[i].UnmetRequirements = diagnoseUnmetRequirements(jobs[i].RequiredCapabilities, agents)
	}
}

func (s *stateStore) attachUnmetRequirementsToJob(job *protocol.Job) {
	if job == nil {
		return
	}
	if !protocol.IsQueuedJobStatus(job.Status) {
		return
	}
	s.mu.Lock()
	agents := make(map[string]agentState, len(s.agents))
	for id, a := range s.agents {
		agents[id] = a
	}
	s.mu.Unlock()
	job.UnmetRequirements = diagnoseUnmetRequirements(job.RequiredCapabilities, agents)
}
