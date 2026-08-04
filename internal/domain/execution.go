package domain

import (
	"errors"
	"time"
)

var ErrJobExecutionNotFound = errors.New("job execution not found")

type ExecutionCard struct {
	Key             string
	Kind            string
	Title           string
	JobExecutionIDs []string
	Summary         ExecutionSummary
	Sections        []ExecutionCardSection
	// ProgressJobs contains the latest attempt for every job represented by
	// the card, including completed jobs that are omitted from an active-only
	// section view. It keeps aggregate progress stable across job boundaries.
	ProgressJobs []ExecutionCardJob
	Progress     Progress
}

type ExecutionCardSection struct {
	Key          string
	Label        string
	Jobs         []ExecutionCardJob
	ProgressJobs []ExecutionCardJob
	Progress     Progress
}

type ExecutionCardJob struct {
	ID                  string
	ProjectID           int64
	Label               string
	Status              string
	PipelineID          string
	BuildLabel          string
	AgentID             string
	CreatedUTC          time.Time
	StartedUTC          time.Time
	FinishedUTC         time.Time
	Reason              string
	Action              string
	CurrentStep         string
	ExpectedDurationMS  int64
	Waiting             bool
	Progress            Progress
	SchedulingDiagnosis *SchedulingDiagnosis
}

type ExecutionSummary struct {
	TotalJobs  int
	Succeeded  int
	Failed     int
	InProgress int
	Waiting    int
}

// JobExecutionDetails is a transport- and persistence-neutral snapshot of one
// execution. Output is intentionally excluded: live logs use a separate,
// incremental query so large histories do not inflate every status refresh.
type JobExecutionDetails struct {
	ID                  string
	ProjectName         string
	PipelineID          string
	PipelineJobID       string
	MatrixName          string
	Status              string
	CurrentStep         string
	AgentID             string
	DryRun              bool
	CreatedUTC          time.Time
	StartedUTC          time.Time
	FinishedUTC         time.Time
	ExitCode            *int
	Error               string
	ExpectedDurationMS  int64
	Waiting             bool
	Progress            Progress
	SchedulingDiagnosis *SchedulingDiagnosis
	Timeline            []JobTimelineItem
}

const (
	SchedulingReady        = "ready"
	SchedulingWaiting      = "waiting"
	SchedulingIncompatible = "incompatible"
)

type SchedulingMatchIssue struct {
	Code     string `json:"code"`
	Key      string `json:"key"`
	Expected string `json:"expected,omitempty"`
	Actual   string `json:"actual,omitempty"`
	Message  string `json:"message"`
}

type SchedulingAgentAssessment struct {
	AgentID            string                 `json:"agent_id"`
	CapabilityMatch    bool                   `json:"capability_match"`
	Available          bool                   `json:"available"`
	CapabilityIssues   []SchedulingMatchIssue `json:"capability_issues,omitempty"`
	AvailabilityIssues []string               `json:"availability_issues,omitempty"`
}

type SchedulingDiagnosis struct {
	State        string                      `json:"state"`
	Summary      string                      `json:"summary"`
	Requirements []string                    `json:"requirements,omitempty"`
	Agents       []SchedulingAgentAssessment `json:"agents,omitempty"`
}

type JobTimelineItem struct {
	ID                 string
	Kind               string
	Name               string
	Description        string
	Index              int
	Total              int
	Reached            bool
	Status             string
	StartedUTC         time.Time
	DurationMS         int64
	FinishedUTC        time.Time
	ExpectedDurationMS int64
	Progress           Progress
	ExitCode           *int
	Error              string
	YAMLLiteral        string
	Command            string
}

// Progress is a renderer-neutral snapshot. Fraction is the completed share at
// SnapshotUnixMS; active determinate progress advances at RatePerMS until the
// next server snapshot. Renderers decide only how to paint these semantics.
type Progress struct {
	State          string  `json:"state"`
	Fraction       float64 `json:"fraction"`
	SnapshotUnixMS int64   `json:"snapshot_unix_ms"`
	RatePerMS      float64 `json:"rate_per_ms"`
}

const (
	ProgressNone          = "none"
	ProgressWaiting       = "waiting"
	ProgressDeterminate   = "determinate"
	ProgressIndeterminate = "indeterminate"
	ProgressComplete      = "complete"
	ProgressOverrun       = "overrun"
)

type JobOutputBatch struct {
	JobExecutionID string
	Events         []JobOutputEvent
	NextEventID    int64
	HasMore        bool
	Terminal       bool
}

const (
	JobOutputEventSystemMessage = "system-message"
	JobOutputEventOutput        = "output"
	JobOutputEventFinished      = "finished"
)

type JobOutputEvent struct {
	ID        int64
	Type      string
	ItemID    string
	Message   string
	Output    string
	Error     string
	ExitCode  *int
	ItemKind  string
	ItemName  string
	ItemIndex int
	ItemTotal int
}
