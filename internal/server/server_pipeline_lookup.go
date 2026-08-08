package server

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/izzyreal/ciwi/internal/domain"
	"github.com/izzyreal/ciwi/internal/protocol"
	"github.com/izzyreal/ciwi/internal/store"
)

func (s *stateStore) loadPipelineChainPipelines(ch store.PersistedPipelineChain) ([]store.PersistedPipeline, error) {
	pipelines := make([]store.PersistedPipeline, 0, len(ch.Pipelines))
	for _, pipelineID := range ch.Pipelines {
		pipelineID = strings.TrimSpace(pipelineID)
		pipeline, err := s.pipelineStore().GetPipelineByProjectIDAndID(ch.ProjectID, pipelineID)
		if err != nil {
			return nil, fmt.Errorf("load pipeline %q in chain %q: %w", pipelineID, ch.ChainName, err)
		}
		pipelines = append(pipelines, pipeline)
	}
	return pipelines, nil
}

func (s *stateStore) getPipelineForJobExecution(job protocol.JobExecution) (store.PersistedPipeline, error) {
	pipelineID := job.Metadata.Value(domain.ExecutionMetadataPipelineID)
	if pipelineID == "" {
		return store.PersistedPipeline{}, fmt.Errorf("pipeline id is required")
	}
	projectID, err := jobExecutionProjectID(job)
	if err != nil {
		return store.PersistedPipeline{}, err
	}
	return s.pipelineStore().GetPipelineByProjectIDAndID(projectID, pipelineID)
}

func jobExecutionProjectID(job protocol.JobExecution) (int64, error) {
	raw := job.Metadata.Value(domain.ExecutionMetadataProjectID)
	if raw == "" {
		return 0, fmt.Errorf("project id is required")
	}
	projectID, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || projectID <= 0 {
		return 0, fmt.Errorf("invalid project id %q", raw)
	}
	return projectID, nil
}

func jobExecutionMatchesProject(job protocol.JobExecution, projectID int64) bool {
	return job.Metadata.Value(domain.ExecutionMetadataProjectID) == strconv.FormatInt(projectID, 10)
}
