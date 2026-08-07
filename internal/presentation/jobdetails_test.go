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
		Artifacts:            []domain.JobArtifact{{Path: "dist/ciwi.zip", SizeBytes: 2048}},
		TestReport: &domain.JobTestReport{
			Total: 3, Passed: 2, Failed: 1,
			Suites: []domain.JobTestSuite{{Name: "unit", Total: 3, Passed: 2, Failed: 1}},
			Coverage: &domain.JobCoverageReport{
				Format: "go-coverprofile", TotalStatements: 100, CoveredStatements: 75,
				Files: []domain.JobCoverageFile{{Path: "main.go", TotalStatements: 20, CoveredStatements: 10}},
			},
		},
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
	if len(view.Artifacts.Nodes) != 1 || len(view.Artifacts.Nodes[0].Children) != 1 || view.Artifacts.Nodes[0].Children[0].Detail != "2 KB" || !view.Artifacts.CanDownloadAll {
		t.Fatalf("artifacts = %+v", view.Artifacts)
	}
	if view.TestReport.Tone != "danger" || !strings.Contains(view.TestReport.Summary, "1 failed") || len(view.TestReport.Nodes) != 1 || len(view.TestReport.Filters) != 4 {
		t.Fatalf("test report = %+v", view.TestReport)
	}
	if !strings.Contains(view.CoverageReport.Summary, "75.00% overall") || view.CoverageReport.Nodes[0].Detail != "50.00% · 10/20" {
		t.Fatalf("coverage report = %+v", view.CoverageReport)
	}
}

func TestJobReportTreesPreserveHierarchyFiltersDownloadsAndSourceLinks(t *testing.T) {
	view := presentJobDetails(domain.JobExecutionDetails{
		ID: "job-1",
		Metadata: map[string]string{
			"pipeline_source_repo":         "git@github.com:izzyreal/ciwi.git",
			"pipeline_source_ref_resolved": "feature/reports",
		},
		Artifacts: []domain.JobArtifact{
			{Path: "dist/macos/Ciwi.app.zip", SizeBytes: 4096},
			{Path: "dist/linux/ciwi", SizeBytes: 2048},
		},
		TestReport: &domain.JobTestReport{
			Total: 2, Passed: 1, Failed: 1,
			Suites: []domain.JobTestSuite{{
				Name: "unit", Format: "junit-xml", Total: 2, Passed: 1, Failed: 1,
				Cases: []domain.JobTestCase{
					{Package: "github.com/izzyreal/ciwi/internal/presentation", Name: "TestPass", File: "jobdetails_test.go", Line: 42, Status: "pass", DurationSeconds: .125},
					{Package: "github.com/izzyreal/ciwi/internal/presentation", Name: "TestFailureWithoutFile", Status: "fail", DurationSeconds: .25},
				},
			}},
			Coverage: &domain.JobCoverageReport{
				TotalStatements: 20, CoveredStatements: 10,
				Files: []domain.JobCoverageFile{
					{Path: "internal/presentation/jobdetails.go", TotalStatements: 10, CoveredStatements: 8},
					{Path: "internal/server/server.go", TotalStatements: 10, CoveredStatements: 2},
				},
			},
		},
	})
	if len(view.Artifacts.Nodes) != 1 || view.Artifacts.Nodes[0].ActionKind != "prefix" || len(view.Artifacts.Nodes[0].Children) != 2 {
		t.Fatalf("artifact tree = %+v", view.Artifacts.Nodes)
	}
	macOS := view.Artifacts.Nodes[0].Children[1]
	if macOS.ActionPath != "dist/macos" || len(macOS.Children) != 1 || macOS.Children[0].ActionKind != "file" {
		t.Fatalf("nested artifact tree = %+v", macOS)
	}
	if view.TestReport.Filter != "all" || len(view.TestReport.Filters) != 4 || len(view.TestReport.Nodes) != 1 {
		t.Fatalf("test report = %+v", view.TestReport)
	}
	packageNode := view.TestReport.Nodes[0].Children[0]
	if len(packageNode.Children) != 2 || packageNode.Children[0].Tone != "danger" || !strings.Contains(packageNode.Children[0].Link, "/search?") {
		t.Fatalf("test cases = %+v", packageNode.Children)
	}
	if !strings.Contains(packageNode.Children[1].Link, "/blob/feature%2Freports/internal/presentation/jobdetails_test.go#L42") {
		t.Fatalf("test source link = %q", packageNode.Children[1].Link)
	}
	if len(view.CoverageReport.Nodes) != 1 || view.CoverageReport.Nodes[0].Label != "internal" || len(view.CoverageReport.Nodes[0].Children) != 2 {
		t.Fatalf("coverage tree = %+v", view.CoverageReport.Nodes)
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
