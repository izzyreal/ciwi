package protocol

import "time"

type AgentInfo struct {
	AgentID              string            `json:"agent_id"`
	Hostname             string            `json:"hostname"`
	OS                   string            `json:"os"`
	Arch                 string            `json:"arch"`
	Version              string            `json:"version,omitempty"`
	Capabilities         map[string]string `json:"capabilities"`
	LastSeenUTC          time.Time         `json:"last_seen_utc"`
	RecentLog            []string          `json:"recent_log,omitempty"`
	NeedsUpdate          bool              `json:"needs_update,omitempty"`
	UpdateTarget         string            `json:"update_target,omitempty"`
	UpdateRequested      bool              `json:"update_requested,omitempty"`
	UpdateAttempts       int               `json:"update_attempts,omitempty"`
	UpdateLastRequestUTC time.Time         `json:"update_last_request_utc,omitempty"`
	UpdateNextRetryUTC   time.Time         `json:"update_next_retry_utc,omitempty"`
	UpdateLastError      string            `json:"update_last_error,omitempty"`
	UpdateLastErrorUTC   time.Time         `json:"update_last_error_utc,omitempty"`
}

type SourceSpec struct {
	Repo string `json:"repo"`
	Ref  string `json:"ref,omitempty"`
}

type JobCacheSpec struct {
	ID  string `json:"id"`
	Env string `json:"env,omitempty"`
}

type CreateJobExecutionRequest struct {
	Script               string            `json:"script"`
	Env                  map[string]string `json:"env,omitempty"`
	RequiredCapabilities map[string]string `json:"required_capabilities"`
	TimeoutSeconds       int               `json:"timeout_seconds"`
	ArtifactGlobs        []string          `json:"artifact_globs,omitempty"`
	Caches               []JobCacheSpec    `json:"caches,omitempty"`
	Source               *SourceSpec       `json:"source,omitempty"`
	Metadata             map[string]string `json:"metadata,omitempty"`
	StepPlan             []JobStepPlanItem `json:"step_plan,omitempty"`
}

type JobExecution struct {
	ID                   string                   `json:"id"`
	Script               string                   `json:"script"`
	Env                  map[string]string        `json:"env,omitempty"`
	RequiredCapabilities map[string]string        `json:"required_capabilities"`
	TimeoutSeconds       int                      `json:"timeout_seconds"`
	ArtifactGlobs        []string                 `json:"artifact_globs,omitempty"`
	Caches               []JobCacheSpec           `json:"caches,omitempty"`
	Source               *SourceSpec              `json:"source,omitempty"`
	Metadata             map[string]string        `json:"metadata,omitempty"`
	StepPlan             []JobStepPlanItem        `json:"step_plan,omitempty"`
	CurrentStep          string                   `json:"current_step,omitempty"`
	Status               string                   `json:"status"`
	CreatedUTC           time.Time                `json:"created_utc"`
	StartedUTC           time.Time                `json:"started_utc,omitempty"`
	FinishedUTC          time.Time                `json:"finished_utc,omitempty"`
	LeasedByAgentID      string                   `json:"leased_by_agent_id,omitempty"`
	LeasedUTC            time.Time                `json:"leased_utc,omitempty"`
	ExitCode             *int                     `json:"exit_code,omitempty"`
	Error                string                   `json:"error,omitempty"`
	Output               string                   `json:"output,omitempty"`
	TestSummary          *JobExecutionTestSummary `json:"test_summary,omitempty"`
	UnmetRequirements    []string                 `json:"unmet_requirements,omitempty"`
	SensitiveValues      []string                 `json:"sensitive_values,omitempty"`
}

type CreateJobExecutionResponse struct {
	JobExecution JobExecution `json:"job_execution"`
}

type LeaseJobExecutionRequest struct {
	AgentID      string            `json:"agent_id"`
	Capabilities map[string]string `json:"capabilities"`
}

type LeaseJobExecutionResponse struct {
	Assigned     bool          `json:"assigned"`
	JobExecution *JobExecution `json:"job_execution,omitempty"`
	Message      string        `json:"message,omitempty"`
}

type RunPipelineRequest struct {
	ConfigPath string `json:"config_path"`
	PipelineID string `json:"pipeline_id"`
}

type RunPipelineResponse struct {
	ProjectName     string   `json:"project_name"`
	PipelineID      string   `json:"pipeline_id"`
	Enqueued        int      `json:"enqueued"`
	JobExecutionIDs []string `json:"job_execution_ids"`
}

type LoadConfigRequest struct {
	ConfigPath string `json:"config_path"`
}

type LoadConfigResponse struct {
	ProjectName string `json:"project_name"`
	ConfigPath  string `json:"config_path"`
	Pipelines   int    `json:"pipelines"`
}

type ProjectSummary struct {
	ID             int64                  `json:"id"`
	Name           string                 `json:"name"`
	ConfigPath     string                 `json:"config_path,omitempty"`
	RepoURL        string                 `json:"repo_url,omitempty"`
	RepoRef        string                 `json:"repo_ref,omitempty"`
	ConfigFile     string                 `json:"config_file,omitempty"`
	Pipelines      []PipelineSummary      `json:"pipelines"`
	PipelineChains []PipelineChainSummary `json:"pipeline_chains,omitempty"`
}

type PipelineSummary struct {
	ID             int64    `json:"id"`
	PipelineID     string   `json:"pipeline_id"`
	Trigger        string   `json:"trigger,omitempty"`
	DependsOn      []string `json:"depends_on,omitempty"`
	SourceRepo     string   `json:"source_repo,omitempty"`
	SourceRef      string   `json:"source_ref,omitempty"`
	SupportsDryRun bool     `json:"supports_dry_run,omitempty"`
}

type PipelineChainSummary struct {
	ID                int64    `json:"id"`
	ChainID           string   `json:"chain_id"`
	Pipelines         []string `json:"pipelines"`
	SupportsDryRun    bool     `json:"supports_dry_run,omitempty"`
	VersionPipelineID int64    `json:"version_pipeline_id,omitempty"`
}

type MatrixInclude struct {
	Index int               `json:"index"`
	Name  string            `json:"name,omitempty"`
	Vars  map[string]string `json:"vars"`
}

type PipelineJobDetail struct {
	ID                   string            `json:"id"`
	Needs                []string          `json:"needs,omitempty"`
	TimeoutSeconds       int               `json:"timeout_seconds"`
	RunsOn               map[string]string `json:"runs_on,omitempty"`
	RequiresTools        map[string]string `json:"requires_tools,omitempty"`
	RequiresCapabilities map[string]string `json:"requires_capabilities,omitempty"`
	Artifacts            []string          `json:"artifacts,omitempty"`
	Caches               []JobCacheSpec    `json:"caches,omitempty"`
	Steps                []PipelineStep    `json:"steps,omitempty"`
	MatrixIncludes       []MatrixInclude   `json:"matrix_includes,omitempty"`
}

type PipelineStep struct {
	Type           string            `json:"type"`
	Run            string            `json:"run,omitempty"`
	TestName       string            `json:"test_name,omitempty"`
	TestCommand    string            `json:"test_command,omitempty"`
	TestFormat     string            `json:"test_format,omitempty"`
	TestReport     string            `json:"test_report,omitempty"`
	CoverageFormat string            `json:"coverage_format,omitempty"`
	CoverageReport string            `json:"coverage_report,omitempty"`
	SkipDryRun     bool              `json:"skip_dry_run,omitempty"`
	Env            map[string]string `json:"env,omitempty"`
}

type PipelineDetail struct {
	ID         int64               `json:"id"`
	PipelineID string              `json:"pipeline_id"`
	Trigger    string              `json:"trigger,omitempty"`
	DependsOn  []string            `json:"depends_on,omitempty"`
	SourceRepo string              `json:"source_repo,omitempty"`
	SourceRef  string              `json:"source_ref,omitempty"`
	Versioning PipelineVersioning  `json:"versioning,omitempty"`
	Jobs       []PipelineJobDetail `json:"jobs,omitempty"`
}

type PipelineVersioning struct {
	File      string `json:"file,omitempty"`
	TagPrefix string `json:"tag_prefix,omitempty"`
	AutoBump  string `json:"auto_bump,omitempty"`
}

type ProjectDetail struct {
	ID             int64                  `json:"id"`
	Name           string                 `json:"name"`
	RepoURL        string                 `json:"repo_url,omitempty"`
	RepoRef        string                 `json:"repo_ref,omitempty"`
	ConfigFile     string                 `json:"config_file,omitempty"`
	Pipelines      []PipelineDetail       `json:"pipelines,omitempty"`
	PipelineChains []PipelineChainSummary `json:"pipeline_chains,omitempty"`
}

type RunPersistedPipelineRequest struct {
	PipelineDBID int64 `json:"pipeline_db_id"`
}

type ImportProjectRequest struct {
	RepoURL    string `json:"repo_url"`
	RepoRef    string `json:"repo_ref,omitempty"`
	ConfigFile string `json:"config_file,omitempty"`
}

type ImportProjectResponse struct {
	ProjectName string `json:"project_name"`
	RepoURL     string `json:"repo_url"`
	RepoRef     string `json:"repo_ref,omitempty"`
	ConfigFile  string `json:"config_file"`
	Pipelines   int    `json:"pipelines"`
}

type RunPipelineSelectionRequest struct {
	PipelineJobID string `json:"pipeline_job_id,omitempty"`
	MatrixName    string `json:"matrix_name,omitempty"`
	MatrixIndex   *int   `json:"matrix_index,omitempty"`
	DryRun        bool   `json:"dry_run,omitempty"`
}

type JobExecutionArtifact struct {
	ID             int64  `json:"id"`
	JobExecutionID string `json:"job_execution_id"`
	Path           string `json:"path"`
	URL            string `json:"url"`
	SizeBytes      int64  `json:"size_bytes"`
}

type JobExecutionArtifactsResponse struct {
	Artifacts []JobExecutionArtifact `json:"artifacts"`
}

type UploadArtifact struct {
	Path       string `json:"path"`
	DataBase64 string `json:"data_base64"`
}

type UploadArtifactsRequest struct {
	AgentID   string           `json:"agent_id"`
	Artifacts []UploadArtifact `json:"artifacts"`
}

type JobExecutionStatusUpdateRequest struct {
	AgentID      string              `json:"agent_id"`
	Status       string              `json:"status"`
	ExitCode     *int                `json:"exit_code,omitempty"`
	Error        string              `json:"error,omitempty"`
	Output       string              `json:"output,omitempty"`
	CurrentStep  string              `json:"current_step,omitempty"`
	Events       []JobExecutionEvent `json:"events,omitempty"`
	TimestampUTC time.Time           `json:"timestamp_utc,omitempty"`
}

const (
	JobExecutionEventTypeStepStarted = "step.started"
)

type JobStepPlanItem struct {
	Index          int    `json:"index"`
	Total          int    `json:"total,omitempty"`
	Name           string `json:"name,omitempty"`
	Script         string `json:"script,omitempty"`
	Kind           string `json:"kind,omitempty"`
	TestName       string `json:"test_name,omitempty"`
	TestFormat     string `json:"test_format,omitempty"`
	TestReport     string `json:"test_report,omitempty"`
	CoverageFormat string `json:"coverage_format,omitempty"`
	CoverageReport string `json:"coverage_report,omitempty"`
}

type JobExecutionEvent struct {
	Type         string           `json:"type"`
	TimestampUTC time.Time        `json:"timestamp_utc,omitempty"`
	Step         *JobStepPlanItem `json:"step,omitempty"`
}

type TestCase struct {
	Package         string  `json:"package,omitempty"`
	Name            string  `json:"name,omitempty"`
	Status          string  `json:"status"`
	DurationSeconds float64 `json:"duration_seconds,omitempty"`
	Output          string  `json:"output,omitempty"`
}

type TestSuiteReport struct {
	Name    string     `json:"name,omitempty"`
	Format  string     `json:"format"`
	Total   int        `json:"total"`
	Passed  int        `json:"passed"`
	Failed  int        `json:"failed"`
	Skipped int        `json:"skipped"`
	Cases   []TestCase `json:"cases,omitempty"`
}

type CoverageFileReport struct {
	Path              string  `json:"path,omitempty"`
	TotalLines        int     `json:"total_lines,omitempty"`
	CoveredLines      int     `json:"covered_lines,omitempty"`
	TotalStatements   int     `json:"total_statements,omitempty"`
	CoveredStatements int     `json:"covered_statements,omitempty"`
	Percent           float64 `json:"percent,omitempty"`
}

type CoverageReport struct {
	Format            string               `json:"format"`
	TotalLines        int                  `json:"total_lines,omitempty"`
	CoveredLines      int                  `json:"covered_lines,omitempty"`
	TotalStatements   int                  `json:"total_statements,omitempty"`
	CoveredStatements int                  `json:"covered_statements,omitempty"`
	Percent           float64              `json:"percent,omitempty"`
	Files             []CoverageFileReport `json:"files,omitempty"`
}

type JobExecutionTestReport struct {
	Total    int               `json:"total"`
	Passed   int               `json:"passed"`
	Failed   int               `json:"failed"`
	Skipped  int               `json:"skipped"`
	Suites   []TestSuiteReport `json:"suites,omitempty"`
	Coverage *CoverageReport   `json:"coverage,omitempty"`
}

type JobExecutionTestSummary struct {
	Total   int `json:"total"`
	Passed  int `json:"passed"`
	Failed  int `json:"failed"`
	Skipped int `json:"skipped"`
}

type UploadTestReportRequest struct {
	AgentID string                 `json:"agent_id"`
	Report  JobExecutionTestReport `json:"report"`
}

type JobExecutionTestReportResponse struct {
	Report JobExecutionTestReport `json:"report"`
}
