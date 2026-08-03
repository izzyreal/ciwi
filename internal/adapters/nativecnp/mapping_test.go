package nativecnp

import (
	"testing"

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
