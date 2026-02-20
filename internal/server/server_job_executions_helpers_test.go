package server

import (
	"testing"
	"time"

	"github.com/izzyreal/ciwi/internal/protocol"
)

func TestJobExecutionEnrichmentHelpers(t *testing.T) {
	ts, s := newTestHTTPServerWithState(t)
	defer ts.Close()

	job, err := s.db.CreateJobExecution(protocol.CreateJobExecutionRequest{
		Script:               "echo hi",
		TimeoutSeconds:       30,
		RequiredCapabilities: map[string]string{"os": "linux", "requires.tool.git": ">=2.40"},
	})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}
	report := protocol.JobExecutionTestReport{Total: 3, Passed: 2, Failed: 1}
	if err := s.db.SaveJobExecutionTestReport(job.ID, report); err != nil {
		t.Fatalf("save test report: %v", err)
	}

	jobs := []protocol.JobExecution{{ID: job.ID, Status: protocol.JobExecutionStatusQueued, RequiredCapabilities: job.RequiredCapabilities}}
	s.attachJobExecutionTestSummaries(jobs)
	if jobs[0].TestSummary == nil || jobs[0].TestSummary.Total != 3 {
		t.Fatalf("expected attached test summary, got %+v", jobs[0].TestSummary)
	}

	s.agents = map[string]agentState{
		"agent-1": {OS: "linux", Arch: "amd64", Capabilities: map[string]string{"tool.git": "2.41.0"}},
	}
	s.attachJobExecutionUnmetRequirements(jobs)
	if len(jobs[0].UnmetRequirements) != 0 {
		t.Fatalf("expected requirements to be met, got %+v", jobs[0].UnmetRequirements)
	}

	queued := protocol.JobExecution{ID: "q", Status: protocol.JobExecutionStatusQueued, RequiredCapabilities: map[string]string{"os": "darwin"}}
	s.attachJobExecutionUnmetRequirementsToJobExecution(&queued)
	if len(queued.UnmetRequirements) == 0 {
		t.Fatalf("expected unmet requirements for darwin requirement")
	}

	s.agents["agent-2"] = agentState{}
	now := time.Now().UTC().Add(-time.Minute)
	s.markAgentSeen("agent-2", now)
	if got := s.agents["agent-2"].LastSeenUTC; !got.Equal(now) {
		t.Fatalf("expected LastSeenUTC update, got %s want %s", got, now)
	}

	before := s.agents["agent-2"].LastSeenUTC
	s.markAgentSeen("", time.Time{})
	s.markAgentSeen("missing-agent", time.Time{})
	if got := s.agents["agent-2"].LastSeenUTC; !got.Equal(before) {
		t.Fatalf("unexpected LastSeenUTC mutation for ignored updates")
	}

	s.attachJobExecutionTestSummary(nil)
	s.attachJobExecutionUnmetRequirementsToJobExecution(nil)
}
