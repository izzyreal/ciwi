package jobexecution

import (
	"net/http"
	"regexp"
	"strings"

	"github.com/izzyreal/ciwi/internal/domain"
	"github.com/izzyreal/ciwi/internal/protocol"
	"github.com/izzyreal/ciwi/internal/server/httpx"
)

var (
	requiredJobFailedRE    = regexp.MustCompile(`(?i)^cancelled:\s+required job\s+(.+?)\s+failed$`)
	upstreamPipelineFailRE = regexp.MustCompile(`(?i)^cancelled:\s+upstream pipeline\s+(.+?)\s+failed$`)
)

func handleJobBlockedBy(w http.ResponseWriter, r *http.Request, deps HandlerDeps, jobID string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	job, err := deps.Store.GetJobExecution(jobID)
	if err != nil {
		http.Error(w, "job not found", http.StatusNotFound)
		return
	}
	if !isDependencyBlockedJob(job) {
		httpx.WriteJSON(w, http.StatusOK, BlockedByViewResponse{Blocked: false})
		return
	}
	all, err := deps.Store.ListJobExecutions()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	dep := findBlockedDependency(job, all)
	if dep == nil {
		httpx.WriteJSON(w, http.StatusOK, BlockedByViewResponse{Blocked: true})
		return
	}
	httpx.WriteJSON(w, http.StatusOK, BlockedByViewResponse{Blocked: true, Dependency: dep})
}

func isDependencyBlockedJob(job protocol.JobExecution) bool {
	if protocol.NormalizeJobExecutionStatus(job.Status) != protocol.JobExecutionStatusFailed {
		return false
	}
	if !job.StartedUTC.IsZero() {
		return false
	}
	_, _, ok := blockedReasonFromError(job.Error)
	return ok
}

func blockedReasonFromError(errText string) (kind string, target string, ok bool) {
	errText = strings.TrimSpace(errText)
	if errText == "" {
		return "", "", false
	}
	if m := requiredJobFailedRE.FindStringSubmatch(errText); len(m) == 2 {
		return "required_job", strings.TrimSpace(m[1]), true
	}
	if m := upstreamPipelineFailRE.FindStringSubmatch(errText); len(m) == 2 {
		return "upstream_pipeline", strings.TrimSpace(m[1]), true
	}
	return "", "", false
}

func findBlockedDependency(job protocol.JobExecution, all []protocol.JobExecution) *BlockedDependencyView {
	all = protocol.LatestJobExecutionAttempts(all)
	kind, target, ok := blockedReasonFromError(job.Error)
	if !ok || target == "" {
		return nil
	}
	switch kind {
	case "required_job":
		return findRequiredJobDependency(job, all, target)
	case "upstream_pipeline":
		return findUpstreamPipelineDependency(job, all, target)
	default:
		return nil
	}
}

func findRequiredJobDependency(job protocol.JobExecution, all []protocol.JobExecution, requiredJobID string) *BlockedDependencyView {
	runID := job.Metadata.Value(domain.ExecutionMetadataPipelineRunID)
	projectID := job.Metadata.Value(domain.ExecutionMetadataProjectID)
	pipelineID := job.Metadata.Value(domain.ExecutionMetadataPipelineID)
	candidates := make([]protocol.JobExecution, 0)
	for _, candidate := range all {
		if candidate.Metadata.Value(domain.ExecutionMetadataPipelineJobID) != requiredJobID {
			continue
		}
		if projectID != "" && candidate.Metadata.Value(domain.ExecutionMetadataProjectID) != projectID {
			continue
		}
		if pipelineID != "" && candidate.Metadata.Value(domain.ExecutionMetadataPipelineID) != pipelineID {
			continue
		}
		if runID != "" && candidate.Metadata.Value(domain.ExecutionMetadataPipelineRunID) != runID {
			continue
		}
		if !protocol.IsTerminalJobExecutionStatus(candidate.Status) {
			continue
		}
		if protocol.NormalizeJobExecutionStatus(candidate.Status) == protocol.JobExecutionStatusSucceeded {
			continue
		}
		candidates = append(candidates, candidate)
	}
	return buildBlockedDependencyFromBest(candidates, "required job "+requiredJobID+" failed")
}

func findUpstreamPipelineDependency(job protocol.JobExecution, all []protocol.JobExecution, upstreamPipelineID string) *BlockedDependencyView {
	chainRunID := job.Metadata.Value(domain.ExecutionMetadataChainRunID)
	projectID := job.Metadata.Value(domain.ExecutionMetadataProjectID)
	candidates := make([]protocol.JobExecution, 0)
	for _, candidate := range all {
		if candidate.Metadata.Value(domain.ExecutionMetadataPipelineID) != upstreamPipelineID {
			continue
		}
		if projectID != "" && candidate.Metadata.Value(domain.ExecutionMetadataProjectID) != projectID {
			continue
		}
		if chainRunID != "" && candidate.Metadata.Value(domain.ExecutionMetadataChainRunID) != chainRunID {
			continue
		}
		if !protocol.IsTerminalJobExecutionStatus(candidate.Status) {
			continue
		}
		if protocol.NormalizeJobExecutionStatus(candidate.Status) == protocol.JobExecutionStatusSucceeded {
			continue
		}
		candidates = append(candidates, candidate)
	}
	return buildBlockedDependencyFromBest(candidates, "upstream pipeline "+upstreamPipelineID+" failed")
}

func buildBlockedDependencyFromBest(candidates []protocol.JobExecution, reason string) *BlockedDependencyView {
	if len(candidates) == 0 {
		return nil
	}
	best := candidates[0]
	for _, candidate := range candidates[1:] {
		if jobCreatedAfter(candidate, best) {
			best = candidate
		}
	}
	return &BlockedDependencyView{
		JobExecutionID: strings.TrimSpace(best.ID),
		PipelineID:     best.Metadata.Value(domain.ExecutionMetadataPipelineID),
		PipelineJobID:  best.Metadata.Value(domain.ExecutionMetadataPipelineJobID),
		MatrixName:     best.Metadata.Value(domain.ExecutionMetadataMatrixName),
		Reason:         reason,
	}
}

func jobCreatedAfter(a, b protocol.JobExecution) bool {
	at := a.CreatedUTC
	bt := b.CreatedUTC
	if at.Equal(bt) {
		return strings.TrimSpace(a.ID) > strings.TrimSpace(b.ID)
	}
	return at.After(bt)
}
