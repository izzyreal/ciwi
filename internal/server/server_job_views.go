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

type jobsSummaryViewResponse struct {
	View              string                   `json:"view"`
	Max               int                      `json:"max"`
	Total             int                      `json:"total"`
	QueuedCount       int                      `json:"queued_count"`
	HistoryCount      int                      `json:"history_count"`
	QueuedGroupCount  int                      `json:"queued_group_count"`
	HistoryGroupCount int                      `json:"history_group_count"`
	QueuedGroups      []jobDisplayGroupSummary `json:"queued_groups"`
	HistoryGroups     []jobDisplayGroupSummary `json:"history_groups"`
}

type jobsPagedViewResponse struct {
	View          string    `json:"view"`
	Total         int       `json:"total"`
	Offset        int       `json:"offset"`
	Limit         int       `json:"limit"`
	JobExecutions []jobView `json:"job_executions"`
	Jobs          []jobView `json:"jobs"`
}

type jobsListViewResponse struct {
	JobExecutions []jobView `json:"job_executions"`
	Jobs          []jobView `json:"jobs"`
}

type jobPairViewResponse struct {
	JobExecution jobView `json:"job_execution"`
	Job          jobView `json:"job"`
}

type jobSingleViewResponse struct {
	Job jobView `json:"job"`
}

type leaseJobViewResponse struct {
	Assigned bool     `json:"assigned"`
	Job      *jobView `json:"job,omitempty"`
	Message  string   `json:"message,omitempty"`
}

type deleteJobViewResponse struct {
	Deleted bool   `json:"deleted"`
	JobID   string `json:"job_id"`
}

type clearQueueViewResponse struct {
	Cleared int64 `json:"cleared"`
}

type flushHistoryViewResponse struct {
	Flushed int64 `json:"flushed"`
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
