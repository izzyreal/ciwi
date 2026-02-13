package server

import (
	"time"

	"github.com/izzyreal/ciwi/internal/protocol"
)

type jobView struct {
	ID                   string                   `json:"id"`
	Script               string                   `json:"script"`
	Env                  map[string]string        `json:"env,omitempty"`
	RequiredCapabilities map[string]string        `json:"required_capabilities"`
	TimeoutSeconds       int                      `json:"timeout_seconds"`
	ArtifactGlobs        []string                 `json:"artifact_globs,omitempty"`
	Caches               []protocol.JobCacheSpec  `json:"caches,omitempty"`
	Source               *protocol.SourceSpec     `json:"source,omitempty"`
	Metadata             map[string]string        `json:"metadata,omitempty"`
	CurrentStep          string                   `json:"current_step,omitempty"`
	Status               string                   `json:"status"`
	CreatedUTC           time.Time                `json:"created_utc"`
	StartedUTC           *time.Time               `json:"started_utc,omitempty"`
	FinishedUTC          *time.Time               `json:"finished_utc,omitempty"`
	LeasedByAgentID      string                   `json:"leased_by_agent_id,omitempty"`
	LeasedUTC            *time.Time               `json:"leased_utc,omitempty"`
	ExitCode             *int                     `json:"exit_code,omitempty"`
	Error                string                   `json:"error,omitempty"`
	Output               string                   `json:"output,omitempty"`
	TestSummary          *protocol.JobTestSummary `json:"test_summary,omitempty"`
	UnmetRequirements    []string                 `json:"unmet_requirements,omitempty"`
	SensitiveValues      []string                 `json:"sensitive_values,omitempty"`
}

type createJobViewResponse struct {
	Job jobView `json:"job"`
}

type leaseJobViewResponse struct {
	Assigned bool     `json:"assigned"`
	Job      *jobView `json:"job,omitempty"`
	Message  string   `json:"message,omitempty"`
}

func jobViewFromProtocol(job protocol.Job) jobView {
	view := jobView{
		ID:                   job.ID,
		Script:               job.Script,
		Env:                  job.Env,
		RequiredCapabilities: job.RequiredCapabilities,
		TimeoutSeconds:       job.TimeoutSeconds,
		ArtifactGlobs:        job.ArtifactGlobs,
		Caches:               job.Caches,
		Source:               job.Source,
		Metadata:             job.Metadata,
		CurrentStep:          job.CurrentStep,
		Status:               protocol.NormalizeJobStatus(job.Status),
		CreatedUTC:           job.CreatedUTC,
		LeasedByAgentID:      job.LeasedByAgentID,
		ExitCode:             job.ExitCode,
		Error:                job.Error,
		Output:               job.Output,
		TestSummary:          job.TestSummary,
		UnmetRequirements:    job.UnmetRequirements,
		SensitiveValues:      job.SensitiveValues,
	}
	if !job.StartedUTC.IsZero() {
		ts := job.StartedUTC
		view.StartedUTC = &ts
	}
	if !job.FinishedUTC.IsZero() {
		ts := job.FinishedUTC
		view.FinishedUTC = &ts
	}
	if !job.LeasedUTC.IsZero() {
		ts := job.LeasedUTC
		view.LeasedUTC = &ts
	}
	return view
}

func jobViewsFromProtocol(jobs []protocol.Job) []jobView {
	out := make([]jobView, len(jobs))
	for i := range jobs {
		out[i] = jobViewFromProtocol(jobs[i])
	}
	return out
}
