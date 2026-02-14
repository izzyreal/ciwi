package server

import (
	"github.com/izzyreal/ciwi/internal/config"
	"github.com/izzyreal/ciwi/internal/protocol"
	"github.com/izzyreal/ciwi/internal/store"
)

// Per-domain store interfaces are the seam for extracting handlers into
// dedicated packages without binding them to stateStore internals.
type agentJobExecutionStore interface {
	CreateJobExecution(req protocol.CreateJobExecutionRequest) (protocol.JobExecution, error)
	LeaseJobExecution(agentID string, agentCaps map[string]string) (*protocol.JobExecution, error)
	AgentHasActiveJobExecution(agentID string) (bool, error)
	UpdateJobExecutionStatus(jobID string, req protocol.JobExecutionStatusUpdateRequest) (protocol.JobExecution, error)
}

type jobExecutionStore interface {
	CreateJobExecution(req protocol.CreateJobExecutionRequest) (protocol.JobExecution, error)
	ListJobExecutions() ([]protocol.JobExecution, error)
	GetJobExecution(id string) (protocol.JobExecution, error)
	UpdateJobExecutionStatus(jobID string, req protocol.JobExecutionStatusUpdateRequest) (protocol.JobExecution, error)
	MergeJobExecutionMetadata(jobID string, patch map[string]string) (map[string]string, error)
	AppendJobExecutionEvents(jobID string, events []protocol.JobExecutionEvent) error
	ListJobExecutionEvents(jobID string) ([]protocol.JobExecutionEvent, error)
	DeleteQueuedJobExecution(jobID string) error
	ClearQueuedJobExecutions() (int64, error)
	FlushJobExecutionHistory() (int64, error)
	SaveJobExecutionArtifacts(jobID string, artifacts []protocol.JobExecutionArtifact) error
	ListJobExecutionArtifacts(jobID string) ([]protocol.JobExecutionArtifact, error)
	SaveJobExecutionTestReport(jobID string, report protocol.JobExecutionTestReport) error
	GetJobExecutionTestReport(jobID string) (protocol.JobExecutionTestReport, bool, error)
}

type pipelineStore interface {
	LoadConfig(cfg config.File, configPath, repoURL, repoRef, configFile string) error
	GetPipelineByProjectAndID(projectName, pipelineID string) (store.PersistedPipeline, error)
	GetPipelineByDBID(id int64) (store.PersistedPipeline, error)
	GetPipelineChainByDBID(id int64) (store.PersistedPipelineChain, error)
	ListJobExecutions() ([]protocol.JobExecution, error)
	CreateJobExecution(req protocol.CreateJobExecutionRequest) (protocol.JobExecution, error)
	MergeJobExecutionMetadata(jobID string, patch map[string]string) (map[string]string, error)
	UpdateJobExecutionStatus(jobID string, req protocol.JobExecutionStatusUpdateRequest) (protocol.JobExecution, error)
}

type projectStore interface {
	LoadConfig(cfg config.File, configPath, repoURL, repoRef, configFile string) error
	GetProjectByID(id int64) (protocol.ProjectSummary, error)
	GetProjectByName(name string) (protocol.ProjectSummary, error)
	GetProjectDetail(id int64) (protocol.ProjectDetail, error)
	ListProjects() ([]protocol.ProjectSummary, error)
}

type vaultStore interface {
	ListVaultConnections() ([]protocol.VaultConnection, error)
	UpsertVaultConnection(req protocol.UpsertVaultConnectionRequest) (protocol.VaultConnection, error)
	DeleteVaultConnection(id int64) error
	GetVaultConnectionByID(id int64) (protocol.VaultConnection, error)
	GetVaultConnectionByName(name string) (protocol.VaultConnection, error)
	GetProjectVaultSettings(projectID int64) (protocol.ProjectVaultSettings, error)
	UpdateProjectVaultSettings(projectID int64, req protocol.UpdateProjectVaultRequest) (protocol.ProjectVaultSettings, error)
	GetProjectByName(name string) (protocol.ProjectSummary, error)
}

type updateStateStore interface {
	SetAppState(key, value string) error
	ListAppState() (map[string]string, error)
}

func (s *stateStore) agentJobExecutionStore() agentJobExecutionStore {
	return s.db
}

func (s *stateStore) jobExecutionStore() jobExecutionStore {
	return s.db
}

func (s *stateStore) pipelineStore() pipelineStore {
	return s.db
}

func (s *stateStore) projectStore() projectStore {
	return s.db
}

func (s *stateStore) vaultStore() vaultStore {
	return s.db
}

func (s *stateStore) updateStateStore() updateStateStore {
	return s.db
}

var _ agentJobExecutionStore = (*store.Store)(nil)
var _ jobExecutionStore = (*store.Store)(nil)
var _ pipelineStore = (*store.Store)(nil)
var _ projectStore = (*store.Store)(nil)
var _ vaultStore = (*store.Store)(nil)
var _ updateStateStore = (*store.Store)(nil)
