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
		ID: "job-1", Label: "linux", Status: "queued", PipelineID: "build", BuildLabel: "v0.2.4 (linux-amd64)",
		AgentID: "agent-1", CreatedUTC: created, Reason: "Waiting for pipeline package", Action: "remove",
	}}}}}})
	job := cards[0].Sections[0].Jobs[0]
	if job.PipelineId != "build" || job.BuildLabel == "" || job.AgentId != "agent-1" || job.CreatedUtc != created.Format(time.RFC3339Nano) || job.Reason == "" || job.Action != "remove" {
		t.Fatalf("execution card job = %+v", job)
	}
}
