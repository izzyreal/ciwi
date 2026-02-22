package jobexecution

import (
	"net/http"
	"regexp"
	"strings"

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
	runID := strings.TrimSpace(job.Metadata["pipeline_run_id"])
	project := strings.TrimSpace(job.Metadata["project"])
	pipelineID := strings.TrimSpace(job.Metadata["pipeline_id"])
	candidates := make([]protocol.JobExecution, 0)
	for _, candidate := range all {
		if strings.TrimSpace(candidate.Metadata["pipeline_job_id"]) != requiredJobID {
			continue
		}
		if project != "" && strings.TrimSpace(candidate.Metadata["project"]) != project {
			continue
		}
		if pipelineID != "" && strings.TrimSpace(candidate.Metadata["pipeline_id"]) != pipelineID {
			continue
		}
		if runID != "" && strings.TrimSpace(candidate.Metadata["pipeline_run_id"]) != runID {
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
	chainRunID := strings.TrimSpace(job.Metadata["chain_run_id"])
	project := strings.TrimSpace(job.Metadata["project"])
	candidates := make([]protocol.JobExecution, 0)
	for _, candidate := range all {
		if strings.TrimSpace(candidate.Metadata["pipeline_id"]) != upstreamPipelineID {
			continue
		}
		if project != "" && strings.TrimSpace(candidate.Metadata["project"]) != project {
			continue
		}
		if chainRunID != "" && strings.TrimSpace(candidate.Metadata["chain_run_id"]) != chainRunID {
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
		PipelineID:     strings.TrimSpace(best.Metadata["pipeline_id"]),
		PipelineJobID:  strings.TrimSpace(best.Metadata["pipeline_job_id"]),
		MatrixName:     strings.TrimSpace(best.Metadata["matrix_name"]),
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
