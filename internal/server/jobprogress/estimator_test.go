package jobprogress

import (
	"fmt"
	"testing"
	"time"

	"github.com/izzyreal/ciwi/internal/protocol"
)

type stubStore struct {
	jobs       []protocol.JobExecution
	events     map[string][]protocol.JobExecutionEvent
	listCalls  int
	batchCalls int
}

func (s *stubStore) ListJobExecutions() ([]protocol.JobExecution, error) {
	s.listCalls++
	return append([]protocol.JobExecution(nil), s.jobs...), nil
}

func (s *stubStore) ListJobExecutionEventsForJobs(jobIDs []string, eventType string) (map[string][]protocol.JobExecutionEvent, error) {
	s.batchCalls++
	out := make(map[string][]protocol.JobExecutionEvent, len(jobIDs))
	for _, id := range jobIDs {
		for _, event := range s.events[id] {
			if eventType == "" || event.Type == eventType {
				out[id] = append(out[id], event)
			}
		}
	}
	return out, nil
}

func TestAttachDetailEstimateUsesRecentMedianAndOneBatch(t *testing.T) {
	base := time.Date(2026, 7, 18, 20, 0, 0, 0, time.UTC)
	step := protocol.JobStepPlanItem{Index: 1, Total: 1, Name: "Current name", Script: "go test ./...", Kind: "run"}
	target := progressJob("target", base, "running", "agent-a", step)
	store := &stubStore{events: map[string][]protocol.JobExecutionEvent{}}
	for i := 1; i <= 12; i++ {
		candidateStep := step
		candidateStep.Name = fmt.Sprintf("Old name %d", i)
		candidate := progressJob(fmt.Sprintf("old-%02d", i), base.Add(-time.Duration(i)*time.Minute), protocol.JobExecutionStatusSucceeded, "agent-a", candidateStep)
		candidate.StartedUTC = candidate.CreatedUTC.Add(time.Second)
		candidate.FinishedUTC = candidate.StartedUTC.Add(time.Duration(i) * time.Second)
		store.jobs = append(store.jobs, candidate)
		store.events[candidate.ID] = []protocol.JobExecutionEvent{{
			Type: protocol.JobExecutionEventTypeStepFinished, Step: &candidateStep, DurationMS: int64(i * 100),
		}}
	}
	store.jobs = append(store.jobs,
		completedProgressJob("other-agent", base.Add(-30*time.Second), "agent-b", step, 99*time.Second),
		completedProgressJob("newer", base.Add(time.Minute), "agent-a", step, 99*time.Second),
	)

	estimator := New(store)
	if err := estimator.AttachDetailEstimate(&target); err != nil {
		t.Fatalf("AttachDetailEstimate: %v", err)
	}
	if target.ExpectedDurationMS != 5500 {
		t.Fatalf("expected median of ten newest runs to be 5500ms, got %d", target.ExpectedDurationMS)
	}
	if got := target.StepExpectedDuration[1]; got != 550 {
		t.Fatalf("expected renamed step history median 550ms, got %d", got)
	}
	if store.listCalls != 1 || store.batchCalls != 1 {
		t.Fatalf("expected one list and one batch call, got list=%d batch=%d", store.listCalls, store.batchCalls)
	}
	if err := estimator.AttachDetailEstimate(&target); err != nil {
		t.Fatalf("cached AttachDetailEstimate: %v", err)
	}
	if store.listCalls != 1 || store.batchCalls != 1 {
		t.Fatalf("expected cache hit, got list=%d batch=%d", store.listCalls, store.batchCalls)
	}
}

func TestAttachDetailEstimateRejectsChangedCommandsAndFailedSteps(t *testing.T) {
	base := time.Date(2026, 7, 18, 20, 0, 0, 0, time.UTC)
	step := protocol.JobStepPlanItem{Index: 1, Total: 1, Script: "go test ./...", Kind: "run"}
	target := progressJob("target", base, "running", "agent-a", step)
	matching := completedProgressJob("matching", base.Add(-time.Minute), "agent-a", step, 4*time.Second)
	changedStep := step
	changedStep.Script = "go test ./changed"
	changed := completedProgressJob("changed", base.Add(-2*time.Minute), "agent-a", changedStep, 20*time.Second)
	exitCode := 1
	store := &stubStore{
		jobs: []protocol.JobExecution{matching, changed},
		events: map[string][]protocol.JobExecutionEvent{
			matching.ID: {{Type: protocol.JobExecutionEventTypeStepFinished, Step: &step, DurationMS: 900, ExitCode: &exitCode}},
			changed.ID:  {{Type: protocol.JobExecutionEventTypeStepFinished, Step: &changedStep, DurationMS: 5000}},
		},
	}
	if err := New(store).AttachDetailEstimate(&target); err != nil {
		t.Fatalf("AttachDetailEstimate: %v", err)
	}
	if target.ExpectedDurationMS != 4000 {
		t.Fatalf("expected only matching job duration, got %d", target.ExpectedDurationMS)
	}
	if len(target.StepExpectedDuration) != 0 {
		t.Fatalf("failed step duration should be excluded, got %+v", target.StepExpectedDuration)
	}
}

func TestAttachJobEstimatesUsesProvisionalEstimateForUnleasedJobs(t *testing.T) {
	base := time.Date(2026, 7, 18, 20, 0, 0, 0, time.UTC)
	step := protocol.JobStepPlanItem{Index: 1, Script: "make"}
	history := completedProgressJob("history", base.Add(-time.Minute), "agent-a", step, 3*time.Second)
	jobs := []protocol.JobExecution{
		progressJob("leased", base, protocol.JobExecutionStatusRunning, "agent-a", step),
		progressJob("unleased", base, protocol.JobExecutionStatusQueued, "", step),
		history,
	}
	New(nil).AttachJobEstimates(jobs)
	if jobs[0].ExpectedDurationMS != 3000 {
		t.Fatalf("expected leased job estimate 3000ms, got %d", jobs[0].ExpectedDurationMS)
	}
	if jobs[1].ExpectedDurationMS != 3000 {
		t.Fatalf("expected unleased job provisional estimate 3000ms, got %d", jobs[1].ExpectedDurationMS)
	}
}

func TestAttachJobEstimatesPrefersExactAgentAndFallsBackAcrossAgents(t *testing.T) {
	base := time.Date(2026, 7, 18, 20, 0, 0, 0, time.UTC)
	step := protocol.JobStepPlanItem{Index: 1, Script: "make"}
	jobs := []protocol.JobExecution{
		progressJob("exact", base, protocol.JobExecutionStatusRunning, "agent-a", step),
		progressJob("fallback", base, protocol.JobExecutionStatusRunning, "agent-c", step),
		completedProgressJob("history-a", base.Add(-time.Minute), "agent-a", step, 3*time.Second),
		completedProgressJob("history-b", base.Add(-2*time.Minute), "agent-b", step, 9*time.Second),
	}
	New(nil).AttachJobEstimates(jobs)
	if jobs[0].ExpectedDurationMS != 3000 {
		t.Fatalf("expected same-agent estimate 3000ms, got %d", jobs[0].ExpectedDurationMS)
	}
	if jobs[1].ExpectedDurationMS != 6000 {
		t.Fatalf("expected cross-agent fallback median 6000ms, got %d", jobs[1].ExpectedDurationMS)
	}
}

func TestAttachJobEstimatesKeepsRequiredCapabilitiesSeparate(t *testing.T) {
	base := time.Date(2026, 7, 18, 20, 0, 0, 0, time.UTC)
	step := protocol.JobStepPlanItem{Index: 1, Script: "make"}
	target := progressJob("target", base, protocol.JobExecutionStatusQueued, "", step)
	target.RequiredCapabilities = map[string]string{"os": "linux", "arch": "amd64"}
	linux := completedProgressJob("linux", base.Add(-time.Minute), "agent-linux", step, 4*time.Second)
	linux.RequiredCapabilities = map[string]string{"arch": "amd64", "os": "linux"}
	windows := completedProgressJob("windows", base.Add(-2*time.Minute), "agent-windows", step, 20*time.Second)
	windows.RequiredCapabilities = map[string]string{"os": "windows", "arch": "amd64"}
	jobs := []protocol.JobExecution{target, linux, windows}
	New(nil).AttachJobEstimates(jobs)
	if jobs[0].ExpectedDurationMS != 4000 {
		t.Fatalf("expected only matching capability history, got %d", jobs[0].ExpectedDurationMS)
	}
}

func TestAttachDetailEstimateUsesProvisionalHistoryBeforeLease(t *testing.T) {
	base := time.Date(2026, 7, 18, 20, 0, 0, 0, time.UTC)
	step := protocol.JobStepPlanItem{Index: 1, Script: "make", Kind: "run"}
	target := progressJob("target", base, protocol.JobExecutionStatusQueued, "", step)
	history := completedProgressJob("history", base.Add(-time.Minute), "agent-a", step, 7*time.Second)
	store := &stubStore{
		jobs: []protocol.JobExecution{history},
		events: map[string][]protocol.JobExecutionEvent{
			history.ID: {{Type: protocol.JobExecutionEventTypeStepFinished, Step: &step, DurationMS: 6500}},
		},
	}
	if err := New(store).AttachDetailEstimate(&target); err != nil {
		t.Fatalf("AttachDetailEstimate: %v", err)
	}
	if target.ExpectedDurationMS != 7000 || target.StepExpectedDuration[1] != 6500 {
		t.Fatalf("unexpected provisional detail estimate: duration=%d steps=%v", target.ExpectedDurationMS, target.StepExpectedDuration)
	}
}

func progressJob(id string, created time.Time, status, agent string, step protocol.JobStepPlanItem) protocol.JobExecution {
	return protocol.JobExecution{
		ID: id, Script: step.Script, StepPlan: []protocol.JobStepPlanItem{step}, Status: status,
		CreatedUTC: created, LeasedByAgentID: agent,
		Metadata: map[string]string{"project": "ciwi", "pipeline_id": "release", "pipeline_job_id": "build", "matrix_name": "linux", "dry_run": "0"},
	}
}

func completedProgressJob(id string, created time.Time, agent string, step protocol.JobStepPlanItem, duration time.Duration) protocol.JobExecution {
	job := progressJob(id, created, protocol.JobExecutionStatusSucceeded, agent, step)
	job.StartedUTC = created.Add(time.Second)
	job.FinishedUTC = job.StartedUTC.Add(duration)
	return job
}
