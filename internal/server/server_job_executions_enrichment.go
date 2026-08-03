package server

import (
	"strings"
	"time"

	"github.com/izzyreal/ciwi/internal/protocol"
	"github.com/izzyreal/ciwi/internal/requirements"
)

func (s *stateStore) attachJobExecutionTestSummaries(jobs []protocol.JobExecution) {
	for i := range jobs {
		s.attachJobExecutionTestSummary(&jobs[i])
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

func (s *stateStore) attachJobExecutionTestSummary(job *protocol.JobExecution) {
	if job == nil || strings.TrimSpace(job.ID) == "" {
		return
	}
	report, found, err := s.jobExecutionStore().GetJobExecutionTestReport(job.ID)
	if err != nil || !found {
		return
	}
	job.TestSummary = &protocol.JobExecutionTestSummary{
		Total:   report.Total,
		Passed:  report.Passed,
		Failed:  report.Failed,
		Skipped: report.Skipped,
	}
}

func (s *stateStore) attachJobExecutionSchedulingDiagnoses(jobs []protocol.JobExecution) {
	agents := s.schedulingAgentSnapshots(time.Now().UTC())
	for i := range jobs {
		jobs[i].SchedulingDiagnosis = requirements.DiagnoseQueuedJob(jobs[i], agents)
	}
}

func (s *stateStore) attachJobExecutionSchedulingDiagnosis(job *protocol.JobExecution) {
	if job == nil {
		return
	}
	job.SchedulingDiagnosis = requirements.DiagnoseQueuedJob(*job, s.schedulingAgentSnapshots(time.Now().UTC()))
}
