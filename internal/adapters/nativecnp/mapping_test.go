package nativecnp

import (
	"testing"
	"time"

	"github.com/izzyreal/ciwi/internal/domain"
	"github.com/izzyreal/ciwi/internal/presentation"
)

func TestSchedulingDiagnosisMappings(t *testing.T) {
	diagnosis := &domain.SchedulingDiagnosis{
		State: domain.SchedulingIncompatible, Summary: "No agent matches all requirements",
		Requirements: []string{"windows", "wix >=6.0.0"},
		Agents: []domain.SchedulingAgentAssessment{{
			AgentID: "linux", CapabilityIssues: []domain.SchedulingMatchIssue{{Message: "os expected windows, got linux"}},
		}},
	}
	cards := executionCardsToProto([]domain.ExecutionCard{{Sections: []domain.ExecutionCardSection{{Jobs: []domain.ExecutionCardJob{{
		ID: "job-1", SchedulingDiagnosis: diagnosis,
	}}}}}})
	if got := cards[0].Sections[0].Jobs[0].SchedulingDiagnosis; got == nil || got.Summary == "" || len(got.Agents) != 1 {
		t.Fatalf("card diagnosis = %+v", got)
	}
	job := jobDetailsToProto(presentation.JobDetailsView{
		ID: "job-1", SchedulingState: domain.SchedulingWaiting, SchedulingSummary: "Matching agent is busy",
		SchedulingRequirements: "windows · wix >=6.0.0",
		SchedulingAgents:       []presentation.SchedulingAgentView{{AgentID: "windows", Status: "Unavailable", Details: "busy", Tone: "warning"}},
	})
	if job.SchedulingDiagnosis == nil || job.SchedulingDiagnosis.RequirementsLabel == "" || job.SchedulingDiagnosis.Agents[0].Details != "busy" {
		t.Fatalf("job diagnosis = %+v", job.SchedulingDiagnosis)
	}
}

func TestExecutionCardMappingCarriesTableMetadata(t *testing.T) {
	created := time.Date(2026, 8, 4, 10, 11, 12, 0, time.UTC)
	cards := executionCardsToProto([]domain.ExecutionCard{{Sections: []domain.ExecutionCardSection{{Jobs: []domain.ExecutionCardJob{{
		ID: "job-1", ProjectID: 41, Label: "linux", Status: "queued", PipelineID: "build", BuildLabel: "v0.2.4 (linux-amd64)",
		AgentID: "agent-1", CreatedUTC: created, Reason: "Waiting for pipeline package", Action: "remove",
	}}}}}})
	job := cards[0].Sections[0].Jobs[0]
	if job.ProjectId != 41 || job.PipelineId != "build" || job.BuildLabel == "" || job.AgentId != "agent-1" || job.CreatedUtc != created.Format(time.RFC3339Nano) || job.Reason == "" || job.Action != "remove" {
		t.Fatalf("execution card job = %+v", job)
	}
}

func TestProgressMappingsPreserveSharedSemanticSnapshot(t *testing.T) {
	progress := domain.Progress{
		State: domain.ProgressDeterminate, Fraction: .42, SnapshotUnixMS: 1234, RatePerMS: .0002,
	}
	cards := executionCardsToProto([]domain.ExecutionCard{{
		Progress: progress,
		Sections: []domain.ExecutionCardSection{{Progress: progress, Jobs: []domain.ExecutionCardJob{{Progress: progress}}}},
	}})
	if got := cards[0].Progress; got == nil || got.State != domain.ProgressDeterminate || got.Fraction != .42 || got.SnapshotUnixMs != 1234 || got.RatePerMs != .0002 {
		t.Fatalf("card progress = %+v", got)
	}
	if cards[0].Sections[0].Progress == nil || cards[0].Sections[0].Jobs[0].Progress == nil {
		t.Fatalf("nested progress was not mapped: %+v", cards[0])
	}
	job := jobDetailsToProto(presentation.JobDetailsView{
		Progress:     progress,
		Timeline:     []presentation.JobTimelineView{{Progress: progress}},
		OutputGroups: []presentation.JobOutputGroupView{{Progress: progress}},
	})
	if job.Progress == nil || job.Timeline[0].Progress == nil || job.OutputGroups[0].Progress == nil {
		t.Fatalf("job progress was not mapped: %+v", job)
	}
}
