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
}

type ExecutionCardSection struct {
	Key   string
	Label string
	Jobs  []ExecutionCardJob
}

type ExecutionCardJob struct {
	ID                  string
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
	ID          string
	Kind        string
	Name        string
	Description string
	Index       int
	Total       int
	Reached     bool
	Status      string
	StartedUTC  time.Time
	DurationMS  int64
	ExitCode    *int
	Error       string
	YAMLLiteral string
	Command     string
}

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
