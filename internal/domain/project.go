package domain

import (
	"errors"
	"time"
)

var ErrProjectNotFound = errors.New("project not found")

// Project is the transport- and persistence-neutral project summary used by
// application services.
type Project struct {
	ID             int64
	Name           string
	SourceKind     string
	ConfigPath     string
	RepoURL        string
	RepoRef        string
	ConfigFile     string
	LoadedCommit   string
	UpdatedUTC     time.Time
	Pipelines      []Pipeline
	PipelineChains []PipelineChain
}

type Pipeline struct {
	ID             int64
	PipelineID     string
	Trigger        string
	DependsOn      []string
	SourceRepo     string
	SourceRef      string
	SupportsDryRun bool
}

type PipelineChain struct {
	ID                string
	Name              string
	Pipelines         []string
	SupportsDryRun    bool
	VersionPipelineID int64
}

type ProjectDetails struct {
	Project   Project
	Pipelines []PipelineDetails
}

type PipelineDetails struct {
	ID         int64
	PipelineID string
	Trigger    string
	DependsOn  []string
	SourceRepo string
	SourceRef  string
	Jobs       []PipelineJobDetails
}

type PipelineJobDetails struct {
	ID             string
	Needs          []string
	TimeoutSeconds int
	RunsOn         map[string]string
	RequiresTools  map[string]string
	MatrixCount    int
	Steps          []PipelineStepDetails
}

type PipelineStepDetails struct {
	Index       int
	Type        string
	Name        string
	TestName    string
	Command     string
	SkipDryRun  bool
	Environment map[string]string
}
