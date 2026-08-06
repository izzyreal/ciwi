package presentation

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/izzyreal/ciwi/internal/domain"
	"github.com/izzyreal/ciwi/internal/requirements"
)

type jobDetailsSourceStub struct {
	details domain.JobExecutionDetails
}

func (s jobDetailsSourceStub) GetJobExecutionDetails(context.Context, string) (domain.JobExecutionDetails, error) {
	return s.details, nil
}

func (s jobDetailsSourceStub) GetJobOutput(context.Context, string, int64) (domain.JobOutputBatch, error) {
	exitCode := 2
	return domain.JobOutputBatch{
		JobExecutionID: "job-1", NextEventID: 3, Terminal: true,
		Events: []domain.JobOutputEvent{
			{ID: 1, Type: domain.JobOutputEventOutput, ItemID: "step:1", Output: "\x1b[31mcompile output\x1b[0m"},
			{ID: 2, Type: domain.JobOutputEventFinished, ItemID: "step:1", ItemKind: "step", ItemName: "Compile", ItemIndex: 1, ItemTotal: 1, ExitCode: &exitCode, Error: "exit=2"},
		},
	}, nil
}

func TestJobDetailsViewFormatsExecutionSnapshot(t *testing.T) {
	started := time.Date(2026, 8, 2, 10, 0, 0, 0, time.UTC)
	exitCode := 0
	view, err := NewJobDetailsQueries(jobDetailsSourceStub{details: domain.JobExecutionDetails{
		ID: "job-1", ProjectName: "ciwi", PipelineID: "build", PipelineJobID: "macos", MatrixName: "arm64",
		Status: "succeeded", StartedUTC: started, FinishedUTC: started.Add(1500 * time.Millisecond), ExitCode: &exitCode,
		Timeline: []domain.JobTimelineItem{
			{ID: "system.workspace", Kind: "phase", Name: "Prepare workspace", Reached: true, Status: "succeeded"},
			{ID: "step:1", Kind: "step", Name: "Compile", Reached: true, Status: "succeeded", DurationMS: 1200, YAMLLiteral: "run: go build ./...", Command: "go build ./..."},
			{ID: "step:2", Kind: "step", Name: "Package", Status: "not reached"},
		},
	}}).GetJobDetailsView(t.Context(), "job-1")
	if err != nil {
		t.Fatal(err)
	}
	if view.Title != "ciwi / build / macos / arm64" || view.Context != "Status: Succeeded" || view.Duration != "1.5s" {
		t.Fatalf("view = %+v", view)
	}
	if !view.CanRerun || view.CanCancel {
		t.Fatalf("execution controls = can rerun %v, can cancel %v", view.CanRerun, view.CanCancel)
	}
	if len(view.Timeline) != 3 || view.Timeline[0].Title != "Ciwi phase 1/1: Prepare workspace" || view.Timeline[1].Title != "Job step 1/2: Compile" || view.Timeline[1].Duration != "00m 01s" {
		t.Fatalf("timeline = %+v", view.Timeline)
	}
	if len(view.OutputGroups) != 3 || view.OutputGroups[1].YAMLLiteral != "run: go build ./..." || view.OutputGroups[2].Reached {
		t.Fatalf("output groups = %+v", view.OutputGroups)
	}
}

func TestJobDetailsViewExposesEligibleControls(t *testing.T) {
	queued := presentJobDetails(domain.JobExecutionDetails{ID: "queued", Status: "queued"})
	if !queued.CanCancel || queued.CanRerun {
		t.Fatalf("queued controls = %+v", queued)
	}
	blocked := presentJobDetails(domain.JobExecutionDetails{
		ID: "blocked", Status: "failed", Error: "cancelled: upstream pipeline build failed",
	})
	if blocked.CanCancel || !blocked.CanRerun {
		t.Fatalf("blocked controls = %+v", blocked)
	}
}

func TestJobStepDurationMatchesBrowserClockFormat(t *testing.T) {
	if got := formatDurationMS(568); got != "00m 00s" {
		t.Fatalf("568ms step duration = %q, want browser format %q", got, "00m 00s")
	}
	if got := formatDurationMS(3_661_000); got != "01h 01m 01s" {
		t.Fatalf("long step duration = %q, want browser format %q", got, "01h 01m 01s")
	}
}

func TestJobDetailsViewMatchesWebExecutionCards(t *testing.T) {
	view := presentJobDetails(domain.JobExecutionDetails{
		ID: "release-1", ProjectID: 7, ProjectName: "ciwi", PipelineID: "release", PipelineJobID: "publish",
		Status: "succeeded", AgentID: "mac", Metadata: map[string]string{
			"build_version": "0.3.0", "build_target": "macos", "version": "0.3.0", "tag": "v0.3.0", "artifacts": "Ciwi.zip",
		},
		RequiredCapabilities: map[string]string{"requires.tool.go": ">=1.25", "requires.container.tool.cmake": "*"},
		RuntimeCapabilities:  map[string]string{"host.tool.go": "1.25.1", "container.tool.cmake": "4.0"},
		CacheStats:           []domain.JobCacheStatistics{{ID: "go", Environment: "host", Type: "directory", Path: "/cache/go", SizeBytes: 1536, Files: 3, Directories: 1}},
	})
	if view.ProjectID != 7 || view.Title != "ciwi / release / publish" || len(view.JobProperties) != 11 {
		t.Fatalf("job header/properties = %+v", view)
	}
	if len(view.CacheStatistics) != 1 || !strings.Contains(view.CacheStatistics[0].Value, "Size: 1.5 KB | Files: 3 | Dirs: 1") {
		t.Fatalf("cache statistics = %+v", view.CacheStatistics)
	}
	if view.HostToolRequirements.Summary != "Requirements matched" || view.ContainerToolRequirements.Summary != "Requirements matched" {
		t.Fatalf("tool requirements = %+v / %+v", view.HostToolRequirements, view.ContainerToolRequirements)
	}
	if !view.HasReleaseSummary || len(view.ReleaseSummary) < 4 {
		t.Fatalf("release summary = %+v", view.ReleaseSummary)
	}
}

func TestJobDetailsViewLimitsClosestSchedulingAgents(t *testing.T) {
	agents := []requirements.AgentAssessment{{AgentID: "matching", CapabilityMatch: true, AvailabilityIssues: []string{"busy"}}}
	for _, id := range []string{"a", "b", "c", "d"} {
		agents = append(agents, requirements.AgentAssessment{
			AgentID: id, CapabilityIssues: []requirements.MatchIssue{{Message: "os expected windows, got linux"}},
		})
	}
	view := presentJobDetails(domain.JobExecutionDetails{ID: "queued", Status: "queued", SchedulingDiagnosis: &requirements.SchedulingDiagnosis{
		State: requirements.DiagnosisWaiting, Summary: "Matching agent matching is busy",
		Requirements: []string{"windows", "wix >=6.0.0"}, Agents: agents,
	}})
	if len(view.SchedulingAgents) != 4 || view.SchedulingAdditional != "1 additional agent(s) do not match" {
		t.Fatalf("scheduling view = %+v additional=%q", view.SchedulingAgents, view.SchedulingAdditional)
	}
	if view.SchedulingAgents[0].Status != "Unavailable" || view.SchedulingRequirements == "" {
		t.Fatalf("scheduling view = %+v", view)
	}
	if view.SchedulingAgents[0].Tone != "warning" || view.SchedulingAgents[1].Tone != "danger" {
		t.Fatalf("scheduling tones = %+v", view.SchedulingAgents)
	}
}

func TestJobOutputViewRendersSanitizedIncrementalLines(t *testing.T) {
	view, err := NewJobDetailsQueries(jobDetailsSourceStub{}).GetJobOutputView(t.Context(), "job-1", 0)
	if err != nil {
		t.Fatal(err)
	}
	if view.NextEventID != 3 || !view.Terminal || len(view.Events) != 2 {
		t.Fatalf("view = %+v", view)
	}
	if view.Events[0].ItemID != "step:1" || view.Events[0].Text != "compile output\n" || view.Events[1].Type != domain.JobOutputEventFinished || view.Events[1].Error != "exit=2" || view.Events[1].ExitCode != "2" {
		t.Fatalf("events = %+v", view.Events)
	}
}
