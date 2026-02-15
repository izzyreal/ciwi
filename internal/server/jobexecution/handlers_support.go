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

func displayRunGroupID(job protocol.JobExecution) string {
	runID := strings.TrimSpace(job.Metadata["pipeline_run_id"])
	if runID == "" {
		return ""
	}
	projectID := strings.TrimSpace(job.Metadata["project_id"])
	if projectID == "" {
		projectID = strings.TrimSpace(job.Metadata["project"])
	}
	pipelineID := strings.TrimSpace(job.Metadata["pipeline_id"])
	return runID + "|" + projectID + "|" + pipelineID
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
	runSizes := map[string]int{}
	for _, job := range jobs {
		runGroupID := displayRunGroupID(job)
		if runGroupID == "" {
			continue
		}
		runSizes[runGroupID]++
	}

	includedRunIDs := map[string]struct{}{}
	includedJobIDs := map[string]struct{}{}
	size := 0
	for _, job := range jobs {
		jobID := strings.TrimSpace(job.ID)
		runGroupID := displayRunGroupID(job)
		if runGroupID == "" {
			if size >= maxJobs && size > 0 {
				break
			}
			if jobID != "" {
				if _, exists := includedJobIDs[jobID]; exists {
					continue
				}
				includedJobIDs[jobID] = struct{}{}
			}
			size++
			continue
		}

		if _, ok := includedRunIDs[runGroupID]; ok {
			continue
		}

		groupSize := runSizes[runGroupID]
		if groupSize <= 0 {
			groupSize = 1
		}
		if size > 0 && size+groupSize > maxJobs {
			break
		}
		includedRunIDs[runGroupID] = struct{}{}
		size += groupSize
	}

	out := make([]protocol.JobExecution, 0, size)
	for _, job := range jobs {
		jobID := strings.TrimSpace(job.ID)
		runGroupID := displayRunGroupID(job)
		if runGroupID != "" {
			if _, ok := includedRunIDs[runGroupID]; ok {
				out = append(out, job)
			}
			continue
		}
		if jobID == "" {
			continue
		}
		if _, ok := includedJobIDs[jobID]; ok {
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
		runGroupID := displayRunGroupID(job)
		if runGroupID == "" {
			continue
		}
		byRunCount[runGroupID]++
	}

	out := make([]DisplayGroupSummary, 0, len(jobs))
	seenRunIDs := map[string]struct{}{}
	for _, job := range jobs {
		jobID := strings.TrimSpace(job.ID)
		runGroupID := displayRunGroupID(job)
		if runGroupID == "" || byRunCount[runGroupID] <= 1 {
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
		if _, ok := seenRunIDs[runGroupID]; ok {
			continue
		}
		seenRunIDs[runGroupID] = struct{}{}
		out = append(out, DisplayGroupSummary{
			Key:         "run:" + runGroupID,
			RunID:       runGroupID,
			JobCount:    byRunCount[runGroupID],
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
