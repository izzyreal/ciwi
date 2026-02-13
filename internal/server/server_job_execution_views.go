package server

import (
	"time"

	"github.com/izzyreal/ciwi/internal/protocol"
)

type jobExecutionView struct {
	ID                   string                            `json:"id"`
	Script               string                            `json:"script"`
	Env                  map[string]string                 `json:"env,omitempty"`
	RequiredCapabilities map[string]string                 `json:"required_capabilities"`
	TimeoutSeconds       int                               `json:"timeout_seconds"`
	ArtifactGlobs        []string                          `json:"artifact_globs,omitempty"`
	Caches               []protocol.JobCacheSpec           `json:"caches,omitempty"`
	Source               *protocol.SourceSpec              `json:"source,omitempty"`
	Metadata             map[string]string                 `json:"metadata,omitempty"`
	CurrentStep          string                            `json:"current_step,omitempty"`
	Status               string                            `json:"status"`
	CreatedUTC           time.Time                         `json:"created_utc"`
	StartedUTC           *time.Time                        `json:"started_utc,omitempty"`
	FinishedUTC          *time.Time                        `json:"finished_utc,omitempty"`
	LeasedByAgentID      string                            `json:"leased_by_agent_id,omitempty"`
	LeasedUTC            *time.Time                        `json:"leased_utc,omitempty"`
	ExitCode             *int                              `json:"exit_code,omitempty"`
	Error                string                            `json:"error,omitempty"`
	Output               string                            `json:"output,omitempty"`
	TestSummary          *protocol.JobExecutionTestSummary `json:"test_summary,omitempty"`
	UnmetRequirements    []string                          `json:"unmet_requirements,omitempty"`
	SensitiveValues      []string                          `json:"sensitive_values,omitempty"`
}

type createJobExecutionViewResponse struct {
	JobExecution jobExecutionView `json:"job_execution"`
}

type jobExecutionsSummaryViewResponse struct {
	View              string                            `json:"view"`
	Max               int                               `json:"max"`
	Total             int                               `json:"total"`
	QueuedCount       int                               `json:"queued_count"`
	HistoryCount      int                               `json:"history_count"`
	QueuedGroupCount  int                               `json:"queued_group_count"`
	HistoryGroupCount int                               `json:"history_group_count"`
	QueuedGroups      []jobExecutionDisplayGroupSummary `json:"queued_groups"`
	HistoryGroups     []jobExecutionDisplayGroupSummary `json:"history_groups"`
}

type jobExecutionsPagedViewResponse struct {
	View          string             `json:"view"`
	Total         int                `json:"total"`
	Offset        int                `json:"offset"`
	Limit         int                `json:"limit"`
	JobExecutions []jobExecutionView `json:"job_executions"`
}

type jobExecutionsListViewResponse struct {
	JobExecutions []jobExecutionView `json:"job_executions"`
}

type jobExecutionViewResponse struct {
	JobExecution jobExecutionView `json:"job_execution"`
}

type leaseJobExecutionViewResponse struct {
	Assigned     bool              `json:"assigned"`
	JobExecution *jobExecutionView `json:"job_execution,omitempty"`
	Message      string            `json:"message,omitempty"`
}

type deleteJobExecutionViewResponse struct {
	Deleted        bool   `json:"deleted"`
	JobExecutionID string `json:"job_execution_id"`
}

type clearJobExecutionQueueViewResponse struct {
	Cleared int64 `json:"cleared"`
}

type flushJobExecutionHistoryViewResponse struct {
	Flushed int64 `json:"flushed"`
}

func jobExecutionViewFromProtocol(job protocol.JobExecution) jobExecutionView {
	view := jobExecutionView{
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
		Status:               protocol.NormalizeJobExecutionStatus(job.Status),
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

func jobExecutionViewsFromProtocol(jobs []protocol.JobExecution) []jobExecutionView {
	out := make([]jobExecutionView, len(jobs))
	for i := range jobs {
		out[i] = jobExecutionViewFromProtocol(jobs[i])
	}
	return out
}
