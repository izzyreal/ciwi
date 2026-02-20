package jobexecution

import (
	"time"

	"github.com/izzyreal/ciwi/internal/protocol"
)

type View struct {
	ID                   string                            `json:"id"`
	Script               string                            `json:"script"`
	Env                  map[string]string                 `json:"env,omitempty"`
	RequiredCapabilities map[string]string                 `json:"required_capabilities"`
	TimeoutSeconds       int                               `json:"timeout_seconds"`
	ArtifactGlobs        []string                          `json:"artifact_globs,omitempty"`
	Caches               []protocol.JobCacheSpec           `json:"caches,omitempty"`
	Source               *protocol.SourceSpec              `json:"source,omitempty"`
	Metadata             map[string]string                 `json:"metadata,omitempty"`
	StepPlan             []protocol.JobStepPlanItem        `json:"step_plan,omitempty"`
	CurrentStep          string                            `json:"current_step,omitempty"`
	CacheStats           []protocol.JobCacheStats          `json:"cache_stats,omitempty"`
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

type CreateViewResponse struct {
	JobExecution View `json:"job_execution"`
}

type SummaryViewResponse struct {
	View              string                `json:"view"`
	Max               int                   `json:"max"`
	Total             int                   `json:"total"`
	QueuedCount       int                   `json:"queued_count"`
	HistoryCount      int                   `json:"history_count"`
	QueuedGroupCount  int                   `json:"queued_group_count"`
	HistoryGroupCount int                   `json:"history_group_count"`
	QueuedGroups      []DisplayGroupSummary `json:"queued_groups"`
	HistoryGroups     []DisplayGroupSummary `json:"history_groups"`
}

type PagedViewResponse struct {
	View          string `json:"view"`
	Total         int    `json:"total"`
	Offset        int    `json:"offset"`
	Limit         int    `json:"limit"`
	JobExecutions []View `json:"job_executions"`
}

type ListViewResponse struct {
	JobExecutions []View `json:"job_executions"`
}

type SingleViewResponse struct {
	JobExecution View `json:"job_execution"`
}

type LeaseViewResponse struct {
	Assigned     bool   `json:"assigned"`
	JobExecution *View  `json:"job_execution,omitempty"`
	Message      string `json:"message,omitempty"`
}

type DeleteViewResponse struct {
	Deleted        bool   `json:"deleted"`
	JobExecutionID string `json:"job_execution_id"`
}

type ClearQueueViewResponse struct {
	Cleared int64 `json:"cleared"`
}

type FlushHistoryViewResponse struct {
	Flushed int64 `json:"flushed"`
}

type EventsViewResponse struct {
	Events []protocol.JobExecutionEvent `json:"events"`
}

func ViewFromProtocol(job protocol.JobExecution) View {
	view := View{
		ID:                   job.ID,
		Script:               job.Script,
		Env:                  job.Env,
		RequiredCapabilities: job.RequiredCapabilities,
		TimeoutSeconds:       job.TimeoutSeconds,
		ArtifactGlobs:        job.ArtifactGlobs,
		Caches:               job.Caches,
		Source:               job.Source,
		Metadata:             job.Metadata,
		StepPlan:             job.StepPlan,
		CurrentStep:          job.CurrentStep,
		CacheStats:           job.CacheStats,
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

func ViewsFromProtocol(jobs []protocol.JobExecution) []View {
	out := make([]View, len(jobs))
	for i := range jobs {
		out[i] = ViewFromProtocol(jobs[i])
	}
	return out
}
