package server

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/izzyreal/ciwi/internal/protocol"
	"github.com/izzyreal/ciwi/internal/store"
)

func (s *stateStore) loadPipelineChainPipelines(ch store.PersistedPipelineChain) ([]store.PersistedPipeline, error) {
	pipelines := make([]store.PersistedPipeline, 0, len(ch.Pipelines))
	for _, pipelineID := range ch.Pipelines {
		pipelineID = strings.TrimSpace(pipelineID)
		pipeline, err := s.pipelineStore().GetPipelineByProjectIDAndID(ch.ProjectID, pipelineID)
		if err != nil {
			return nil, fmt.Errorf("load pipeline %q in chain %q: %w", pipelineID, ch.ChainID, err)
		}
		pipelines = append(pipelines, pipeline)
	}
	return pipelines, nil
}

func (s *stateStore) getPipelineForJobExecution(job protocol.JobExecution) (store.PersistedPipeline, error) {
	pipelineID := strings.TrimSpace(job.Metadata["pipeline_id"])
	if pipelineID == "" {
		return store.PersistedPipeline{}, fmt.Errorf("pipeline id is required")
	}
	projectID, hasProjectID, err := jobExecutionProjectID(job)
	if err != nil {
		return store.PersistedPipeline{}, err
	}
	if hasProjectID {
		return s.pipelineStore().GetPipelineByProjectIDAndID(projectID, pipelineID)
	}
	projectName := strings.TrimSpace(job.Metadata["project"])
	if projectName == "" {
		return store.PersistedPipeline{}, fmt.Errorf("project id or name is required")
	}
	return s.pipelineStore().GetPipelineByProjectAndID(projectName, pipelineID)
}

func jobExecutionProjectID(job protocol.JobExecution) (int64, bool, error) {
	raw := strings.TrimSpace(job.Metadata["project_id"])
	if raw == "" {
		return 0, false, nil
	}
	projectID, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || projectID <= 0 {
		return 0, false, fmt.Errorf("invalid project id %q", raw)
	}
	return projectID, true, nil
}

func jobExecutionMatchesProject(job protocol.JobExecution, projectID int64, projectName string) bool {
	rawProjectID := strings.TrimSpace(job.Metadata["project_id"])
	if projectID > 0 && rawProjectID != "" {
		return rawProjectID == strconv.FormatInt(projectID, 10)
	}
	return strings.TrimSpace(job.Metadata["project"]) == strings.TrimSpace(projectName)
}
