package jobexecution

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/izzyreal/ciwi/internal/protocol"
)

type DisplayGroupSummary struct {
	Key         string `json:"key"`
	RunID       string `json:"run_id,omitempty"`
	JobCount    int    `json:"job_count"`
	Collapsible bool   `json:"collapsible"`
}

func ParseQueryInt(r *http.Request, key string, fallback, min, max int) int {
	raw := strings.TrimSpace(r.URL.Query().Get(key))
	if raw == "" {
		return fallback
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

func SplitByState(jobs []protocol.JobExecution) (queued []protocol.JobExecution, history []protocol.JobExecution) {
	queued = make([]protocol.JobExecution, 0, len(jobs))
	history = make([]protocol.JobExecution, 0, len(jobs))
	for _, job := range jobs {
		if protocol.IsActiveJobExecutionStatus(job.Status) {
			queued = append(queued, job)
			continue
		}
		history = append(history, job)
	}
	return queued, history
}

func CapDisplayJobs(jobs []protocol.JobExecution, maxJobs int) []protocol.JobExecution {
	if maxJobs <= 0 || len(jobs) <= maxJobs {
		return jobs
	}
	out := make([]protocol.JobExecution, 0, maxJobs)
	includedRunIDs := map[string]struct{}{}
	for _, job := range jobs {
		runID := strings.TrimSpace(job.Metadata["pipeline_run_id"])
		if len(out) < maxJobs {
			out = append(out, job)
			if runID != "" {
				includedRunIDs[runID] = struct{}{}
			}
			continue
		}
		if runID == "" {
			continue
		}
		if _, ok := includedRunIDs[runID]; ok {
			out = append(out, job)
		}
	}
	return out
}

func SummarizeDisplayGroups(jobs []protocol.JobExecution) []DisplayGroupSummary {
	if len(jobs) == 0 {
		return nil
	}
	byRunCount := map[string]int{}
	for _, job := range jobs {
		runID := strings.TrimSpace(job.Metadata["pipeline_run_id"])
		if runID == "" {
			continue
		}
		byRunCount[runID]++
	}

	out := make([]DisplayGroupSummary, 0, len(jobs))
	seenRunIDs := map[string]struct{}{}
	for _, job := range jobs {
		jobID := strings.TrimSpace(job.ID)
		runID := strings.TrimSpace(job.Metadata["pipeline_run_id"])
		if runID == "" || byRunCount[runID] <= 1 {
			if jobID == "" {
				continue
			}
			out = append(out, DisplayGroupSummary{
				Key:         "job:" + jobID,
				JobCount:    1,
				Collapsible: false,
			})
			continue
		}
		if _, ok := seenRunIDs[runID]; ok {
			continue
		}
		seenRunIDs[runID] = struct{}{}
		out = append(out, DisplayGroupSummary{
			Key:         "run:" + runID,
			RunID:       runID,
			JobCount:    byRunCount[runID],
			Collapsible: true,
		})
	}
	return out
}

func Paginate(jobs []protocol.JobExecution, offset, limit int) []protocol.JobExecution {
	if offset >= len(jobs) {
		return []protocol.JobExecution{}
	}
	end := offset + limit
	if end > len(jobs) {
		end = len(jobs)
	}
	return append([]protocol.JobExecution(nil), jobs[offset:end]...)
}
