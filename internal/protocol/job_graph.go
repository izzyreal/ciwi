package protocol

import "time"

type JobExecutionGraphContext struct {
	Scope                string                      `json:"scope"`
	CurrentExecutionID   string                      `json:"current_execution_id"`
	CurrentPipelineID    string                      `json:"current_pipeline_id,omitempty"`
	CurrentPipelineRunID string                      `json:"current_pipeline_run_id,omitempty"`
	CurrentPipelineJobID string                      `json:"current_pipeline_job_id,omitempty"`
	CurrentChainRunID    string                      `json:"current_chain_run_id,omitempty"`
	CurrentPipelineChain string                      `json:"current_pipeline_chain_name,omitempty"`
	Pipelines            []JobExecutionGraphPipeline `json:"pipelines"`
}

type JobExecutionGraphPipeline struct {
	PipelineID    string                 `json:"pipeline_id"`
	PipelineRunID string                 `json:"pipeline_run_id,omitempty"`
	PipelineDBID  int64                  `json:"pipeline_db_id,omitempty"`
	DependsOn     []string               `json:"depends_on,omitempty"`
	Status        string                 `json:"status"`
	Jobs          []JobExecutionGraphJob `json:"jobs"`
}

type JobExecutionGraphJob struct {
	PipelineJobID string                       `json:"pipeline_job_id"`
	Needs         []string                     `json:"needs,omitempty"`
	Status        string                       `json:"status"`
	Executions    []JobExecutionGraphExecution `json:"executions"`
}

type JobExecutionGraphExecution struct {
	ID            string    `json:"id"`
	Status        string    `json:"status"`
	MatrixIndex   string    `json:"matrix_index,omitempty"`
	MatrixName    string    `json:"matrix_name,omitempty"`
	AttemptRootID string    `json:"attempt_root_id"`
	RerunOfJobID  string    `json:"rerun_of_job_id,omitempty"`
	LatestAttempt bool      `json:"latest_attempt"`
	CreatedUTC    time.Time `json:"created_utc"`
	StartedUTC    time.Time `json:"started_utc,omitempty"`
	FinishedUTC   time.Time `json:"finished_utc,omitempty"`
}
