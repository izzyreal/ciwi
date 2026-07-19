package protocol

import "testing"

func TestBuildJobExecutionTimelineIncludesApplicablePhasesAndSteps(t *testing.T) {
	job := JobExecution{
		Source: &SourceSpec{Repo: "https://example.test/repo.git", Ref: "abc123"},
		Env: map[string]string{
			"CIWI_DEP_ARTIFACT_JOB_IDS": "job-a, job-b",
			"CIWI_DEP_ARTIFACT_JOB_ID":  "job-b",
		},
		Caches:        []JobCacheSpec{{ID: "fetchcontent"}},
		ArtifactGlobs: []string{"dist/**"},
		StepPlan: []JobStepPlanItem{
			{Index: 1, Name: "Build"},
			{Index: 2, Name: "Test", TestReport: "dist/results.xml"},
		},
	}

	timeline := BuildJobExecutionTimeline(job)
	wantIDs := []string{
		JobExecutionPhaseWorkspace,
		JobExecutionPhaseCheckout,
		JobExecutionPhaseDependencies,
		JobExecutionPhaseEnvironment,
		"step:1",
		"step:2",
		JobExecutionPhaseArtifacts,
		JobExecutionPhaseTests,
	}
	if len(timeline) != len(wantIDs) {
		t.Fatalf("expected %d timeline items, got %d: %+v", len(wantIDs), len(timeline), timeline)
	}
	for i, wantID := range wantIDs {
		if timeline[i].ID != wantID || timeline[i].Index != i+1 || timeline[i].Total != len(wantIDs) {
			t.Fatalf("unexpected timeline item %d: %+v", i, timeline[i])
		}
	}
	if timeline[2].Description != "Source jobs: job-a, job-b" {
		t.Fatalf("dependency IDs should be stable and deduplicated, got %q", timeline[2].Description)
	}
	if index, total := TimelineStepPosition(timeline, 2); index != 6 || total != 8 {
		t.Fatalf("expected second YAML step at 6/8, got %d/%d", index, total)
	}
}

func TestBuildJobExecutionTimelineMinimalJob(t *testing.T) {
	timeline := BuildJobExecutionTimeline(JobExecution{StepPlan: []JobStepPlanItem{{Index: 1}}})
	if len(timeline) != 3 {
		t.Fatalf("expected workspace, environment, and YAML step, got %+v", timeline)
	}
	if timeline[0].ID != JobExecutionPhaseWorkspace || timeline[1].ID != JobExecutionPhaseEnvironment || timeline[2].ID != "step:1" {
		t.Fatalf("unexpected minimal timeline: %+v", timeline)
	}
	if timeline[2].Name != "" {
		t.Fatalf("expected unnamed step to omit a redundant display name, got %q", timeline[2].Name)
	}
}
