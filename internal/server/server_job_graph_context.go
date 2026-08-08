package server

import (
	"context"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/izzyreal/ciwi/internal/domain"
	"github.com/izzyreal/ciwi/internal/protocol"
)

func (s *stateStore) jobExecutionGraphContextHandler(w http.ResponseWriter, r *http.Request, jobID string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	target, err := s.jobExecutionStore().GetJobExecution(jobID)
	if err != nil {
		http.Error(w, "job not found", http.StatusNotFound)
		return
	}
	graphContext, err := s.jobExecutionGraphContextForTarget(r.Context(), target)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, graphContext)
}

func (s *stateStore) GetJobExecutionGraphContext(ctx context.Context, jobID string) (protocol.JobExecutionGraphContext, error) {
	if err := ctx.Err(); err != nil {
		return protocol.JobExecutionGraphContext{}, err
	}
	target, err := s.jobExecutionStore().GetJobExecution(jobID)
	if err != nil {
		return protocol.JobExecutionGraphContext{}, err
	}
	return s.jobExecutionGraphContextForTarget(ctx, target)
}

func (s *stateStore) jobExecutionGraphContextForTarget(ctx context.Context, target protocol.JobExecution) (protocol.JobExecutionGraphContext, error) {
	if err := ctx.Err(); err != nil {
		return protocol.JobExecutionGraphContext{}, err
	}
	jobs, err := s.jobExecutionStore().ListJobExecutions()
	if err != nil {
		return protocol.JobExecutionGraphContext{}, err
	}
	pipelineDBIDs := map[string]int64{}
	if projectID, ok := target.Metadata.Int64(domain.ExecutionMetadataProjectID); ok && projectID > 0 {
		if detail, detailErr := s.projectStore().GetProjectDetail(projectID); detailErr == nil {
			for _, pipeline := range detail.Pipelines {
				pipelineDBIDs[strings.TrimSpace(pipeline.PipelineID)] = pipeline.ID
			}
		}
	}
	return buildJobExecutionGraphContext(target, jobs, pipelineDBIDs), nil
}

func buildJobExecutionGraphContext(target protocol.JobExecution, jobs []protocol.JobExecution, pipelineDBIDs map[string]int64) protocol.JobExecutionGraphContext {
	meta := target.Metadata
	chainRunID := meta.Value(domain.ExecutionMetadataChainRunID)
	pipelineRunID := meta.Value(domain.ExecutionMetadataPipelineRunID)
	projectID := meta.Value(domain.ExecutionMetadataProjectID)
	currentPipelineID := meta.Value(domain.ExecutionMetadataPipelineID)
	scope := "job"
	if chainRunID != "" {
		scope = "chain"
	} else if pipelineRunID != "" {
		scope = "pipeline"
	}

	selected := make([]protocol.JobExecution, 0)
	for _, job := range jobs {
		candidate := job.Metadata
		if chainRunID != "" {
			if candidate.Value(domain.ExecutionMetadataChainRunID) != chainRunID {
				continue
			}
		} else if pipelineRunID != "" {
			if candidate.Value(domain.ExecutionMetadataPipelineRunID) != pipelineRunID || candidate.Value(domain.ExecutionMetadataPipelineID) != currentPipelineID {
				continue
			}
			if candidate.Value(domain.ExecutionMetadataProjectID) != projectID {
				continue
			}
		} else if job.ID != target.ID {
			continue
		}
		selected = append(selected, job)
	}
	if len(selected) == 0 {
		selected = append(selected, target)
	}

	type pipelineGroup struct {
		id        string
		runID     string
		index     int
		dependsOn []string
		jobs      []protocol.JobExecution
	}
	groups := map[string]*pipelineGroup{}
	ordered := make([]*pipelineGroup, 0)
	for _, job := range selected {
		pipelineID := job.Metadata.Value(domain.ExecutionMetadataPipelineID)
		if pipelineID == "" {
			pipelineID = "job"
		}
		group := groups[pipelineID]
		if group == nil {
			index, _ := strconv.Atoi(job.Metadata.Value(domain.ExecutionMetadataPipelineChainIndex))
			group = &pipelineGroup{id: pipelineID, runID: job.Metadata.Value(domain.ExecutionMetadataPipelineRunID), index: index}
			groups[pipelineID] = group
			ordered = append(ordered, group)
		}
		group.jobs = append(group.jobs, job)
		group.dependsOn = appendUniqueStrings(group.dependsOn, job.Metadata.CSV(domain.ExecutionMetadataChainDependsOnPipelines)...)
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].index != ordered[j].index {
			return ordered[i].index < ordered[j].index
		}
		return ordered[i].id < ordered[j].id
	})

	context := protocol.JobExecutionGraphContext{
		Scope:                scope,
		CurrentExecutionID:   target.ID,
		CurrentPipelineID:    currentPipelineID,
		CurrentPipelineRunID: pipelineRunID,
		CurrentPipelineJobID: meta.Value(domain.ExecutionMetadataPipelineJobID),
		CurrentChainRunID:    chainRunID,
		CurrentPipelineChain: meta.Value(domain.ExecutionMetadataPipelineChainName),
		Pipelines:            make([]protocol.JobExecutionGraphPipeline, 0, len(ordered)),
	}
	for _, group := range ordered {
		pipeline := protocol.JobExecutionGraphPipeline{
			PipelineID:    group.id,
			PipelineRunID: group.runID,
			PipelineDBID:  pipelineDBIDs[group.id],
			DependsOn:     append([]string(nil), group.dependsOn...),
			Jobs:          buildJobExecutionGraphJobs(group.jobs),
		}
		statuses := make([]string, 0, len(pipeline.Jobs))
		for _, job := range pipeline.Jobs {
			statuses = append(statuses, job.Status)
		}
		pipeline.Status = aggregateJobGraphStatuses(statuses)
		context.Pipelines = append(context.Pipelines, pipeline)
	}
	return context
}

func buildJobExecutionGraphJobs(jobs []protocol.JobExecution) []protocol.JobExecutionGraphJob {
	type logicalGroup struct {
		id         string
		needs      []string
		executions []protocol.JobExecution
		createdKey string
	}
	groups := map[string]*logicalGroup{}
	ordered := make([]*logicalGroup, 0)
	for _, job := range jobs {
		id := job.Metadata.Value(domain.ExecutionMetadataPipelineJobID)
		if id == "" {
			id = strings.TrimSpace(job.ID)
		}
		group := groups[id]
		if group == nil {
			group = &logicalGroup{id: id, createdKey: job.CreatedUTC.Format(timeSortLayout)}
			groups[id] = group
			ordered = append(ordered, group)
		}
		createdKey := job.CreatedUTC.Format(timeSortLayout)
		if group.createdKey == "" || createdKey < group.createdKey {
			group.createdKey = createdKey
		}
		group.needs = appendUniqueStrings(group.needs, job.Metadata.CSV(domain.ExecutionMetadataNeedsJobIDs)...)
		group.executions = append(group.executions, job)
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].createdKey != ordered[j].createdKey {
			return ordered[i].createdKey < ordered[j].createdKey
		}
		return ordered[i].id < ordered[j].id
	})
	out := make([]protocol.JobExecutionGraphJob, 0, len(ordered))
	for _, group := range ordered {
		latest := protocol.LatestJobExecutionAttempts(group.executions)
		latestIDs := map[string]struct{}{}
		statuses := make([]string, 0, len(latest))
		for _, job := range latest {
			latestIDs[job.ID] = struct{}{}
			statuses = append(statuses, graphExecutionStatus(job))
		}
		executions := make([]protocol.JobExecutionGraphExecution, 0, len(group.executions))
		sort.SliceStable(group.executions, func(i, j int) bool {
			if !group.executions[i].CreatedUTC.Equal(group.executions[j].CreatedUTC) {
				return group.executions[i].CreatedUTC.Before(group.executions[j].CreatedUTC)
			}
			return group.executions[i].ID < group.executions[j].ID
		})
		for _, job := range group.executions {
			_, isLatest := latestIDs[job.ID]
			executions = append(executions, protocol.JobExecutionGraphExecution{
				ID:            job.ID,
				Status:        graphExecutionStatus(job),
				MatrixIndex:   job.Metadata.Value(domain.ExecutionMetadataMatrixIndex),
				MatrixName:    job.Metadata.Value(domain.ExecutionMetadataMatrixName),
				AttemptRootID: protocol.JobExecutionAttemptRootID(job),
				RerunOfJobID:  job.Metadata.Value(protocol.JobMetadataRerunOfJobID),
				LatestAttempt: isLatest,
				CreatedUTC:    job.CreatedUTC,
				StartedUTC:    job.StartedUTC,
				FinishedUTC:   job.FinishedUTC,
			})
		}
		out = append(out, protocol.JobExecutionGraphJob{
			PipelineJobID: group.id,
			Needs:         append([]string(nil), group.needs...),
			Status:        aggregateJobGraphStatuses(statuses),
			Executions:    executions,
		})
	}
	return out
}

const timeSortLayout = "2006-01-02T15:04:05.999999999Z07:00"

func graphExecutionStatus(job protocol.JobExecution) string {
	status := protocol.NormalizeJobExecutionStatus(job.Status)
	if status == protocol.JobExecutionStatusQueued && (job.Metadata.Flag(domain.ExecutionMetadataChainBlocked) || job.Metadata.Flag(domain.ExecutionMetadataNeedsBlocked) || job.Metadata.Flag(domain.ExecutionMetadataDependencyBlocked)) {
		return "waiting"
	}
	return status
}

func aggregateJobGraphStatuses(statuses []string) string {
	seen := map[string]bool{}
	for _, status := range statuses {
		seen[protocol.NormalizeJobExecutionStatus(status)] = true
	}
	if seen[protocol.JobExecutionStatusRunning] || seen[protocol.JobExecutionStatusLeased] {
		return protocol.JobExecutionStatusRunning
	}
	if seen[protocol.JobExecutionStatusQueued] {
		return protocol.JobExecutionStatusQueued
	}
	if seen[protocol.JobExecutionStatusFailed] {
		return protocol.JobExecutionStatusFailed
	}
	if seen["waiting"] {
		return "waiting"
	}
	if len(seen) > 0 && seen[protocol.JobExecutionStatusSucceeded] {
		return protocol.JobExecutionStatusSucceeded
	}
	return "unknown"
}

func splitGraphIDs(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if value := strings.TrimSpace(part); value != "" {
			out = append(out, value)
		}
	}
	return out
}

func appendUniqueStrings(dst []string, values ...string) []string {
	seen := make(map[string]struct{}, len(dst)+len(values))
	for _, value := range dst {
		seen[value] = struct{}{}
	}
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		dst = append(dst, value)
	}
	return dst
}
