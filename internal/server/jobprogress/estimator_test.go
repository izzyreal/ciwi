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

func (s *stubStore) ListJobExecutionDurationEventsForJobs(jobIDs []string) (map[string][]protocol.JobExecutionEvent, error) {
	s.batchCalls++
	out := make(map[string][]protocol.JobExecutionEvent, len(jobIDs))
	for _, id := range jobIDs {
		for _, event := range s.events[id] {
			if event.Type == protocol.JobExecutionEventTypeStepFinished || event.Type == protocol.JobExecutionEventTypePhaseFinished {
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

func TestAttachDetailEstimateMatchesStepEventsWithoutEnv(t *testing.T) {
	base := time.Date(2026, 7, 18, 20, 0, 0, 0, time.UTC)
	step := protocol.JobStepPlanItem{
		Index:  1,
		Total:  1,
		Name:   "Configure CMake",
		Script: "cmake -B build",
		Kind:   "run",
		Env:    map[string]string{"VMPC2000XL_DOCUMENTS_PATH": "/tmp/vmpc2000xl_documents"},
	}
	target := progressJob("target", base, "running", "agent-a", step)
	matching := completedProgressJob("matching", base.Add(-time.Minute), "agent-a", step, 4*time.Second)
	eventStep := step
	eventStep.Env = nil
	store := &stubStore{
		jobs: []protocol.JobExecution{matching},
		events: map[string][]protocol.JobExecutionEvent{
			matching.ID: {{Type: protocol.JobExecutionEventTypeStepFinished, Step: &eventStep, DurationMS: 3200}},
		},
	}
	if err := New(store).AttachDetailEstimate(&target); err != nil {
		t.Fatalf("AttachDetailEstimate: %v", err)
	}
	if got := target.StepExpectedDuration[1]; got != 3200 {
		t.Fatalf("expected step event without env to match, got %d from %+v", got, target.StepExpectedDuration)
	}
}

func TestEstimateStepsRetainsHistoryAcrossIndexShifts(t *testing.T) {
	step := func(index int, script string) protocol.JobStepPlanItem {
		return protocol.JobStepPlanItem{Index: index, Script: script, Kind: "run"}
	}
	tests := []struct {
		name       string
		historical []protocol.JobStepPlanItem
		current    []protocol.JobStepPlanItem
		want       map[int]int64
	}{
		{
			name:       "earlier step removed",
			historical: []protocol.JobStepPlanItem{step(1, "prepare"), step(2, "build"), step(3, "test")},
			current:    []protocol.JobStepPlanItem{step(1, "build"), step(2, "test")},
			want:       map[int]int64{1: 2000, 2: 3000},
		},
		{
			name:       "earlier step inserted",
			historical: []protocol.JobStepPlanItem{step(1, "build"), step(2, "test")},
			current:    []protocol.JobStepPlanItem{step(1, "prepare"), step(2, "build"), step(3, "test")},
			want:       map[int]int64{2: 1000, 3: 2000},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			history := protocol.JobExecution{ID: "history", StepPlan: tt.historical}
			events := make([]protocol.JobExecutionEvent, 0, len(tt.historical))
			for _, historicalStep := range tt.historical {
				historicalStep := historicalStep
				events = append(events, protocol.JobExecutionEvent{
					Type: protocol.JobExecutionEventTypeStepFinished, Step: &historicalStep,
					DurationMS: int64(historicalStep.Index) * 1000,
				})
			}

			got := estimateSteps(tt.current, []protocol.JobExecution{history}, nil, map[string][]protocol.JobExecutionEvent{"history": events})
			if len(got) != len(tt.want) {
				t.Fatalf("expected estimates %v, got %v", tt.want, got)
			}
			for index, duration := range tt.want {
				if got[index] != duration {
					t.Fatalf("expected step %d duration %dms, got %dms from %v", index, duration, got[index], got)
				}
			}
		})
	}
}

func TestEstimateStepsAlignsRepeatedIdenticalStepsInOrder(t *testing.T) {
	historical := []protocol.JobStepPlanItem{
		{Index: 1, Script: "prepare", Kind: "run"},
		{Index: 2, Script: "make", Kind: "run"},
		{Index: 3, Script: "make", Kind: "run"},
		{Index: 4, Script: "test", Kind: "run"},
	}
	current := []protocol.JobStepPlanItem{
		{Index: 1, Script: "make", Kind: "run"},
		{Index: 2, Script: "make", Kind: "run"},
		{Index: 3, Script: "test", Kind: "run"},
	}
	history := protocol.JobExecution{ID: "history", StepPlan: historical}
	events := []protocol.JobExecutionEvent{
		{Type: protocol.JobExecutionEventTypeStepFinished, Step: &historical[1], DurationMS: 1200},
		{Type: protocol.JobExecutionEventTypeStepFinished, Step: &historical[2], DurationMS: 3400},
		{Type: protocol.JobExecutionEventTypeStepFinished, Step: &historical[3], DurationMS: 5600},
	}

	got := estimateSteps(current, []protocol.JobExecution{history}, nil, map[string][]protocol.JobExecutionEvent{"history": events})
	want := map[int]int64{1: 1200, 2: 3400, 3: 5600}
	for index, duration := range want {
		if got[index] != duration {
			t.Fatalf("expected step %d duration %dms, got %dms from %v", index, duration, got[index], got)
		}
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

func TestAttachJobEstimatesFallsBackToSameLogicalJobAfterPlanChange(t *testing.T) {
	base := time.Date(2026, 7, 18, 20, 0, 0, 0, time.UTC)
	oldStep := protocol.JobStepPlanItem{Index: 1, Script: "make", Kind: "run"}
	newStep := protocol.JobStepPlanItem{Index: 1, Script: "make --parallel 4", Kind: "run"}
	target := progressJob("target", base, protocol.JobExecutionStatusRunning, "agent-a", newStep)
	history := completedProgressJob("history", base.Add(-time.Minute), "agent-a", oldStep, 8*time.Second)
	jobs := []protocol.JobExecution{target, history}

	New(nil).AttachJobEstimates(jobs)
	if jobs[0].ExpectedDurationMS != 8000 {
		t.Fatalf("expected logical-job fallback 8000ms, got %d", jobs[0].ExpectedDurationMS)
	}
}

func TestAttachJobEstimatesPrefersSameAgentLogicalHistoryOverOtherAgentExactPlan(t *testing.T) {
	base := time.Date(2026, 7, 18, 20, 0, 0, 0, time.UTC)
	oldStep := protocol.JobStepPlanItem{Index: 1, Script: "make", Kind: "run"}
	newStep := protocol.JobStepPlanItem{Index: 1, Script: "make --parallel 4", Kind: "run"}
	target := progressJob("target", base, protocol.JobExecutionStatusRunning, "agent-a", newStep)
	sameAgent := completedProgressJob("same-agent", base.Add(-time.Minute), "agent-a", oldStep, 6*time.Second)
	otherAgent := completedProgressJob("other-agent", base.Add(-2*time.Minute), "agent-b", newStep, 20*time.Second)
	jobs := []protocol.JobExecution{target, sameAgent, otherAgent}

	New(nil).AttachJobEstimates(jobs)
	if jobs[0].ExpectedDurationMS != 6000 {
		t.Fatalf("expected same-agent logical estimate 6000ms, got %d", jobs[0].ExpectedDurationMS)
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

func TestAttachJobEstimatesKeepsLogicalJobDimensionsSeparate(t *testing.T) {
	base := time.Date(2026, 7, 18, 20, 0, 0, 0, time.UTC)
	oldStep := protocol.JobStepPlanItem{Index: 1, Script: "make", Kind: "run"}
	newStep := protocol.JobStepPlanItem{Index: 1, Script: "make --parallel 4", Kind: "run"}
	tests := []struct {
		name string
		key  string
	}{
		{name: "project", key: "project"},
		{name: "pipeline", key: "pipeline_id"},
		{name: "pipeline job", key: "pipeline_job_id"},
		{name: "matrix entry", key: "matrix_name"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target := progressJob("target", base, protocol.JobExecutionStatusRunning, "agent-a", newStep)
			history := completedProgressJob("history", base.Add(-time.Minute), "agent-a", oldStep, 8*time.Second)
			history.Metadata[tt.key] = "different"
			jobs := []protocol.JobExecution{target, history}

			New(nil).AttachJobEstimates(jobs)
			if jobs[0].ExpectedDurationMS != 0 {
				t.Fatalf("different %s must not share history, got %d", tt.name, jobs[0].ExpectedDurationMS)
			}
		})
	}
}

func TestAttachJobEstimatesExcludesUnsuccessfulLogicalHistory(t *testing.T) {
	base := time.Date(2026, 7, 18, 20, 0, 0, 0, time.UTC)
	oldStep := protocol.JobStepPlanItem{Index: 1, Script: "make", Kind: "run"}
	newStep := protocol.JobStepPlanItem{Index: 1, Script: "make --parallel 4", Kind: "run"}
	for _, status := range []string{
		protocol.JobExecutionStatusFailed,
		"cancelled",
		protocol.JobExecutionStatusRunning,
	} {
		t.Run(status, func(t *testing.T) {
			target := progressJob("target", base, protocol.JobExecutionStatusRunning, "agent-a", newStep)
			history := completedProgressJob("history", base.Add(-time.Minute), "agent-a", oldStep, 8*time.Second)
			history.Status = status
			jobs := []protocol.JobExecution{target, history}

			New(nil).AttachJobEstimates(jobs)
			if jobs[0].ExpectedDurationMS != 0 {
				t.Fatalf("%s history must not provide a whole-job estimate, got %d", status, jobs[0].ExpectedDurationMS)
			}
		})
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

func TestAttachDetailEstimateUsesSuccessfulPhaseHistory(t *testing.T) {
	base := time.Date(2026, 7, 18, 20, 0, 0, 0, time.UTC)
	step := protocol.JobStepPlanItem{Index: 1, Script: "make", Kind: "run"}
	target := progressJob("target", base, protocol.JobExecutionStatusRunning, "agent-a", step)
	historyA := completedProgressJob("history-a", base.Add(-time.Minute), "agent-a", step, 8*time.Second)
	historyB := completedProgressJob("history-b", base.Add(-2*time.Minute), "agent-a", step, 12*time.Second)
	failedCode := 1
	phase := protocol.JobExecutionPhase{ID: protocol.JobExecutionPhaseEnvironment, Name: "Prepare execution environment"}
	store := &stubStore{
		jobs: []protocol.JobExecution{historyA, historyB},
		events: map[string][]protocol.JobExecutionEvent{
			historyA.ID: {
				{Type: protocol.JobExecutionEventTypePhaseFinished, Phase: &phase, DurationMS: 1000},
				{Type: protocol.JobExecutionEventTypePhaseFinished, Phase: &protocol.JobExecutionPhase{ID: protocol.JobExecutionPhaseWorkspace}, DurationMS: 200},
			},
			historyB.ID: {
				{Type: protocol.JobExecutionEventTypePhaseFinished, Phase: &phase, DurationMS: 3000},
				{Type: protocol.JobExecutionEventTypePhaseFinished, Phase: &protocol.JobExecutionPhase{ID: protocol.JobExecutionPhaseWorkspace}, DurationMS: 9000, ExitCode: &failedCode},
			},
		},
	}

	if err := New(store).AttachDetailEstimate(&target); err != nil {
		t.Fatalf("AttachDetailEstimate: %v", err)
	}
	if got := target.PhaseExpectedDuration[protocol.JobExecutionPhaseEnvironment]; got != 2000 {
		t.Fatalf("expected environment phase median 2000ms, got %d", got)
	}
	if got := target.PhaseExpectedDuration[protocol.JobExecutionPhaseWorkspace]; got != 200 {
		t.Fatalf("expected failed phase sample to be excluded, got %d", got)
	}
}

func TestAttachDetailEstimateSharesExecutedUnitsAcrossDryRunModes(t *testing.T) {
	base := time.Date(2026, 7, 18, 20, 0, 0, 0, time.UTC)
	step := protocol.JobStepPlanItem{Index: 1, Script: "make", Kind: "run"}
	target := progressJob("target", base, protocol.JobExecutionStatusRunning, "agent-a", step)
	target.ArtifactGlobs = []string{"dist/**"}
	dryRun := completedProgressJob("dry-run", base.Add(-time.Minute), "agent-a", step, 9*time.Second)
	dryRun.Metadata["dry_run"] = "1"
	dryRun.ArtifactGlobs = []string{"preview/**"}
	workspace := protocol.JobExecutionPhase{ID: protocol.JobExecutionPhaseWorkspace}
	artifacts := protocol.JobExecutionPhase{ID: protocol.JobExecutionPhaseArtifacts}
	store := &stubStore{
		jobs: []protocol.JobExecution{dryRun},
		events: map[string][]protocol.JobExecutionEvent{
			dryRun.ID: {
				{Type: protocol.JobExecutionEventTypeStepFinished, Step: &step, DurationMS: 1200},
				{Type: protocol.JobExecutionEventTypePhaseFinished, Phase: &workspace, DurationMS: 200},
				{Type: protocol.JobExecutionEventTypePhaseFinished, Phase: &artifacts, DurationMS: 800},
			},
		},
	}

	if err := New(store).AttachDetailEstimate(&target); err != nil {
		t.Fatalf("AttachDetailEstimate: %v", err)
	}
	if target.ExpectedDurationMS != 9000 {
		t.Fatalf("expected logical-job whole-duration fallback 9000ms, got %d", target.ExpectedDurationMS)
	}
	if got := target.StepExpectedDuration[1]; got != 1200 {
		t.Fatalf("expected executed dry-run step sample to be shared, got %d", got)
	}
	if got := target.PhaseExpectedDuration[protocol.JobExecutionPhaseWorkspace]; got != 200 {
		t.Fatalf("expected compatible dry-run phase sample to be shared, got %d", got)
	}
	if _, ok := target.PhaseExpectedDuration[protocol.JobExecutionPhaseArtifacts]; ok {
		t.Fatalf("artifact phase with different globs must not be shared: %+v", target.PhaseExpectedDuration)
	}
}

func TestAttachDetailEstimateDoesNotShareSkippedDryRunStep(t *testing.T) {
	base := time.Date(2026, 7, 18, 20, 0, 0, 0, time.UTC)
	step := protocol.JobStepPlanItem{Index: 1, Script: "publish-release", Kind: "run"}
	target := progressJob("target", base, protocol.JobExecutionStatusRunning, "agent-a", step)
	skipped := protocol.JobStepPlanItem{Index: 1, Kind: "dryrun_skip"}
	dryRun := completedProgressJob("dry-run", base.Add(-time.Minute), "agent-a", skipped, time.Second)
	dryRun.Metadata["dry_run"] = "1"
	store := &stubStore{
		jobs: []protocol.JobExecution{dryRun},
		events: map[string][]protocol.JobExecutionEvent{
			dryRun.ID: {{Type: protocol.JobExecutionEventTypeStepFinished, Step: &skipped, DurationMS: 10}},
		},
	}

	if err := New(store).AttachDetailEstimate(&target); err != nil {
		t.Fatalf("AttachDetailEstimate: %v", err)
	}
	if len(target.StepExpectedDuration) != 0 {
		t.Fatalf("dry-run-skipped step must not estimate an executed step, got %+v", target.StepExpectedDuration)
	}
}

func TestAttachDetailEstimateUsesSuccessfulUnitsFromFailedJob(t *testing.T) {
	base := time.Date(2026, 7, 18, 20, 0, 0, 0, time.UTC)
	step := protocol.JobStepPlanItem{Index: 1, Script: "make", Kind: "run"}
	target := progressJob("target", base, protocol.JobExecutionStatusRunning, "agent-a", step)
	failedJob := completedProgressJob("failed", base.Add(-time.Minute), "agent-a", step, 8*time.Second)
	failedJob.Status = protocol.JobExecutionStatusFailed
	workspace := protocol.JobExecutionPhase{ID: protocol.JobExecutionPhaseWorkspace}
	store := &stubStore{
		jobs: []protocol.JobExecution{failedJob},
		events: map[string][]protocol.JobExecutionEvent{
			failedJob.ID: {
				{Type: protocol.JobExecutionEventTypeStepFinished, Step: &step, DurationMS: 1400},
				{Type: protocol.JobExecutionEventTypePhaseFinished, Phase: &workspace, DurationMS: 300},
			},
		},
	}

	if err := New(store).AttachDetailEstimate(&target); err != nil {
		t.Fatalf("AttachDetailEstimate: %v", err)
	}
	if target.ExpectedDurationMS != 0 {
		t.Fatalf("failed job must not contribute aggregate duration, got %d", target.ExpectedDurationMS)
	}
	if target.StepExpectedDuration[1] != 1400 || target.PhaseExpectedDuration[protocol.JobExecutionPhaseWorkspace] != 300 {
		t.Fatalf("successful units from failed job were not reused: steps=%v phases=%v", target.StepExpectedDuration, target.PhaseExpectedDuration)
	}
}

func TestAttachDetailEstimateFallsBackPerUnitWhenSameAgentSampleIsUnusable(t *testing.T) {
	base := time.Date(2026, 7, 18, 20, 0, 0, 0, time.UTC)
	step := protocol.JobStepPlanItem{Index: 1, Script: "make", Kind: "run"}
	target := progressJob("target", base, protocol.JobExecutionStatusRunning, "agent-a", step)
	target.ArtifactGlobs = []string{"dist/**"}
	skipped := protocol.JobStepPlanItem{Index: 1, Kind: "dryrun_skip"}
	sameAgent := completedProgressJob("same-agent", base.Add(-time.Minute), "agent-a", skipped, time.Second)
	sameAgent.ArtifactGlobs = []string{"preview/**"}
	otherAgent := completedProgressJob("other-agent", base.Add(-2*time.Minute), "agent-b", step, 4*time.Second)
	otherAgent.ArtifactGlobs = []string{"dist/**"}
	artifactPhase := protocol.JobExecutionPhase{ID: protocol.JobExecutionPhaseArtifacts}
	store := &stubStore{
		jobs: []protocol.JobExecution{sameAgent, otherAgent},
		events: map[string][]protocol.JobExecutionEvent{
			sameAgent.ID: {
				{Type: protocol.JobExecutionEventTypeStepFinished, Step: &skipped, DurationMS: 10},
				{Type: protocol.JobExecutionEventTypePhaseFinished, Phase: &artifactPhase, DurationMS: 100},
			},
			otherAgent.ID: {
				{Type: protocol.JobExecutionEventTypeStepFinished, Step: &step, DurationMS: 2200},
				{Type: protocol.JobExecutionEventTypePhaseFinished, Phase: &artifactPhase, DurationMS: 3000},
			},
		},
	}

	if err := New(store).AttachDetailEstimate(&target); err != nil {
		t.Fatalf("AttachDetailEstimate: %v", err)
	}
	if got := target.StepExpectedDuration[1]; got != 2200 {
		t.Fatalf("expected per-step cross-agent fallback, got %d", got)
	}
	if got := target.PhaseExpectedDuration[protocol.JobExecutionPhaseArtifacts]; got != 3000 {
		t.Fatalf("expected per-phase cross-agent fallback, got %d", got)
	}
}

func TestAttachDetailEstimateDoesNotMixCrossAgentSamplesIntoSameAgentHistory(t *testing.T) {
	base := time.Date(2026, 7, 18, 20, 0, 0, 0, time.UTC)
	step := protocol.JobStepPlanItem{Index: 1, Script: "make", Kind: "run"}
	target := progressJob("target", base, protocol.JobExecutionStatusRunning, "agent-a", step)
	sameAgent := completedProgressJob("same-agent", base.Add(-time.Minute), "agent-a", step, time.Second)
	otherAgent := completedProgressJob("other-agent", base.Add(-2*time.Minute), "agent-b", step, 9*time.Second)
	store := &stubStore{
		jobs: []protocol.JobExecution{sameAgent, otherAgent},
		events: map[string][]protocol.JobExecutionEvent{
			sameAgent.ID:  {{Type: protocol.JobExecutionEventTypeStepFinished, Step: &step, DurationMS: 1000}},
			otherAgent.ID: {{Type: protocol.JobExecutionEventTypeStepFinished, Step: &step, DurationMS: 9000}},
		},
	}

	if err := New(store).AttachDetailEstimate(&target); err != nil {
		t.Fatalf("AttachDetailEstimate: %v", err)
	}
	if got := target.StepExpectedDuration[1]; got != 1000 {
		t.Fatalf("expected same-agent-only estimate, got %d", got)
	}
}

func TestAttachDetailEstimateFindsOlderValidSampleAfterUnusableCandidates(t *testing.T) {
	base := time.Date(2026, 7, 18, 20, 0, 0, 0, time.UTC)
	step := protocol.JobStepPlanItem{Index: 1, Script: "make", Kind: "run"}
	skipped := protocol.JobStepPlanItem{Index: 1, Kind: "dryrun_skip"}
	target := progressJob("target", base, protocol.JobExecutionStatusRunning, "agent-a", step)
	store := &stubStore{events: map[string][]protocol.JobExecutionEvent{}}
	for i := 1; i <= 12; i++ {
		candidate := completedProgressJob(fmt.Sprintf("skipped-%02d", i), base.Add(-time.Duration(i)*time.Minute), "agent-a", skipped, time.Second)
		store.jobs = append(store.jobs, candidate)
		store.events[candidate.ID] = []protocol.JobExecutionEvent{{Type: protocol.JobExecutionEventTypeStepFinished, Step: &skipped, DurationMS: 10}}
	}
	older := completedProgressJob("older-valid", base.Add(-13*time.Minute), "agent-a", step, 3*time.Second)
	store.jobs = append(store.jobs, older)
	store.events[older.ID] = []protocol.JobExecutionEvent{{Type: protocol.JobExecutionEventTypeStepFinished, Step: &step, DurationMS: 2800}}

	if err := New(store).AttachDetailEstimate(&target); err != nil {
		t.Fatalf("AttachDetailEstimate: %v", err)
	}
	if got := target.StepExpectedDuration[1]; got != 2800 {
		t.Fatalf("expected older valid sample after unusable candidates, got %d", got)
	}
}

func TestAttachDetailEstimateRejectsChangedStepEnvironmentAndVault(t *testing.T) {
	base := time.Date(2026, 7, 18, 20, 0, 0, 0, time.UTC)
	step := protocol.JobStepPlanItem{
		Index: 1, Script: "publish", Kind: "run",
		Env: map[string]string{"CHANNEL": "stable"}, VaultConnection: "home-vault",
		VaultSecrets: []protocol.ProjectSecretSpec{{Name: "token", Mount: "kv", Path: "github", Key: "token"}},
	}
	target := progressJob("target", base, protocol.JobExecutionStatusRunning, "agent-a", step)
	changedEnvStep := step
	changedEnvStep.Env = map[string]string{"CHANNEL": "preview"}
	changedVaultStep := step
	changedVaultStep.VaultSecrets = []protocol.ProjectSecretSpec{{Name: "token", Mount: "kv", Path: "github-preview", Key: "token"}}
	changedEnv := completedProgressJob("changed-env", base.Add(-time.Minute), "agent-a", changedEnvStep, time.Second)
	changedVault := completedProgressJob("changed-vault", base.Add(-2*time.Minute), "agent-a", changedVaultStep, time.Second)
	eventStep := step
	eventStep.Env = nil
	eventStep.VaultConnection = ""
	eventStep.VaultSecrets = nil
	store := &stubStore{
		jobs: []protocol.JobExecution{changedEnv, changedVault},
		events: map[string][]protocol.JobExecutionEvent{
			changedEnv.ID:   {{Type: protocol.JobExecutionEventTypeStepFinished, Step: &eventStep, DurationMS: 1000}},
			changedVault.ID: {{Type: protocol.JobExecutionEventTypeStepFinished, Step: &eventStep, DurationMS: 2000}},
		},
	}

	if err := New(store).AttachDetailEstimate(&target); err != nil {
		t.Fatalf("AttachDetailEstimate: %v", err)
	}
	if len(target.StepExpectedDuration) != 0 {
		t.Fatalf("changed environment or Vault configuration must not match: %+v", target.StepExpectedDuration)
	}
}

func TestAttachJobEstimatesSharesIdenticalPlanAcrossDryRunModes(t *testing.T) {
	base := time.Date(2026, 7, 18, 20, 0, 0, 0, time.UTC)
	step := protocol.JobStepPlanItem{Index: 1, Script: "make", Kind: "run"}
	target := progressJob("target", base, protocol.JobExecutionStatusRunning, "agent-a", step)
	dryRun := completedProgressJob("dry-run", base.Add(-time.Minute), "agent-a", step, 9*time.Second)
	dryRun.Metadata["dry_run"] = "1"
	jobs := []protocol.JobExecution{target, dryRun}

	New(nil).AttachJobEstimates(jobs)
	if jobs[0].ExpectedDurationMS != 9000 {
		t.Fatalf("identical executable plans should share whole-job history, got %d", jobs[0].ExpectedDurationMS)
	}
}

func TestAttachJobEstimatesUsesLogicalFallbackForDifferentDryRunPlan(t *testing.T) {
	base := time.Date(2026, 7, 18, 20, 0, 0, 0, time.UTC)
	ordinaryStep := protocol.JobStepPlanItem{Index: 1, Script: "publish-release", Kind: "run"}
	target := progressJob("target", base, protocol.JobExecutionStatusRunning, "agent-a", ordinaryStep)
	skippedStep := protocol.JobStepPlanItem{Index: 1, Kind: "dryrun_skip"}
	dryRun := completedProgressJob("dry-run", base.Add(-time.Minute), "agent-a", skippedStep, time.Second)
	dryRun.Metadata["dry_run"] = "1"
	jobs := []protocol.JobExecution{target, dryRun}

	New(nil).AttachJobEstimates(jobs)
	if jobs[0].ExpectedDurationMS != 1000 {
		t.Fatalf("expected logical-job fallback for different dry-run plan, got %d", jobs[0].ExpectedDurationMS)
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
