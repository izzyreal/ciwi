package protocol

import "strings"

const (
	JobMetadataAttemptRootJobID = "attempt_root_job_id"
	JobMetadataRerunOfJobID     = "rerun_of_job_id"
)

// JobExecutionAttemptRootID identifies executions that are attempts of the
// same logical pipeline job. Executions created before attempt metadata was
// introduced are their own roots.
func JobExecutionAttemptRootID(job JobExecution) string {
	if root := strings.TrimSpace(job.Metadata[JobMetadataAttemptRootJobID]); root != "" {
		return root
	}
	return strings.TrimSpace(job.ID)
}

// LatestJobExecutionAttempts removes superseded attempts while preserving the
// input order. CreatedUTC determines recency, with the job ID as a stable tie
// breaker.
func LatestJobExecutionAttempts(jobs []JobExecution) []JobExecution {
	if len(jobs) < 2 {
		return append([]JobExecution(nil), jobs...)
	}
	latest := make(map[string]JobExecution, len(jobs))
	for _, job := range jobs {
		root := JobExecutionAttemptRootID(job)
		current, ok := latest[root]
		if !ok || job.CreatedUTC.After(current.CreatedUTC) ||
			(job.CreatedUTC.Equal(current.CreatedUTC) && strings.TrimSpace(job.ID) > strings.TrimSpace(current.ID)) {
			latest[root] = job
		}
	}
	out := make([]JobExecution, 0, len(latest))
	for _, job := range jobs {
		root := JobExecutionAttemptRootID(job)
		if selected, ok := latest[root]; ok && selected.ID == job.ID {
			out = append(out, job)
		}
	}
	return out
}
