package jobexecution

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/izzyreal/ciwi/internal/protocol"
)

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

func finishedJobs(jobs []protocol.JobExecution) []protocol.JobExecution {
	history := make([]protocol.JobExecution, 0, len(jobs))
	for _, job := range jobs {
		if protocol.IsActiveJobExecutionStatus(job.Status) {
			continue
		}
		history = append(history, job)
	}
	return history
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
