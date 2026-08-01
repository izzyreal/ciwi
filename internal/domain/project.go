package domain

import "time"

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
