package server

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/izzyreal/ciwi/internal/protocol"
)

type jobExecutionDisplayGroupSummary struct {
	Key         string `json:"key"`
	RunID       string `json:"run_id,omitempty"`
	JobCount    int    `json:"job_count"`
	Collapsible bool   `json:"collapsible"`
}

func (s *stateStore) jobExecutionsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		view := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("view")))
		maxJobs := parseJobExecutionsQueryInt(r, "max", 150, 1, 2000)

		jobs, err := s.db.ListJobExecutions()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if maxJobs > 0 && len(jobs) > maxJobs {
			jobs = jobs[:maxJobs]
		}

		queuedJobs, historyJobs := splitJobExecutionsByState(jobs)
		switch view {
		case "summary":
			queuedGroups := summarizeJobExecutionDisplayGroups(queuedJobs)
			historyGroups := summarizeJobExecutionDisplayGroups(historyJobs)
			writeJSON(w, http.StatusOK, jobExecutionsSummaryViewResponse{
				View:              "summary",
				Max:               maxJobs,
				Total:             len(jobs),
				QueuedCount:       len(queuedJobs),
				HistoryCount:      len(historyJobs),
				QueuedGroupCount:  len(queuedGroups),
				HistoryGroupCount: len(historyGroups),
				QueuedGroups:      queuedGroups,
				HistoryGroups:     historyGroups,
			})
			return
		case "queued", "history":
			source := queuedJobs
			if view == "history" {
				source = historyJobs
			}
			offset := parseJobExecutionsQueryInt(r, "offset", 0, 0, 1_000_000)
			limit := parseJobExecutionsQueryInt(r, "limit", 25, 1, 200)
			page := paginateJobExecutions(source, offset, limit)
			s.attachJobExecutionTestSummaries(page)
			s.attachJobExecutionUnmetRequirements(page)
			pageViews := jobExecutionViewsFromProtocol(page)
			writeJSON(w, http.StatusOK, jobExecutionsPagedViewResponse{
				View:          view,
				Total:         len(source),
				Offset:        offset,
				Limit:         limit,
				JobExecutions: pageViews,
			})
			return
		}

		s.attachJobExecutionTestSummaries(jobs)
		s.attachJobExecutionUnmetRequirements(jobs)
		jobsViews := jobExecutionViewsFromProtocol(jobs)
		writeJSON(w, http.StatusOK, jobExecutionsListViewResponse{JobExecutions: jobsViews})
	case http.MethodPost:
		var req protocol.CreateJobExecutionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON body", http.StatusBadRequest)
			return
		}
		job, err := s.db.CreateJobExecution(req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusCreated, createJobExecutionViewResponse{JobExecution: jobExecutionViewFromProtocol(job)})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func parseJobExecutionsQueryInt(r *http.Request, key string, fallback, min, max int) int {
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

func splitJobExecutionsByState(jobs []protocol.JobExecution) (queued []protocol.JobExecution, history []protocol.JobExecution) {
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

func summarizeJobExecutionDisplayGroups(jobs []protocol.JobExecution) []jobExecutionDisplayGroupSummary {
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

	out := make([]jobExecutionDisplayGroupSummary, 0, len(jobs))
	seenRunIDs := map[string]struct{}{}
	for _, job := range jobs {
		jobID := strings.TrimSpace(job.ID)
		runID := strings.TrimSpace(job.Metadata["pipeline_run_id"])
		if runID == "" || byRunCount[runID] <= 1 {
			if jobID == "" {
				continue
			}
			out = append(out, jobExecutionDisplayGroupSummary{
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
		out = append(out, jobExecutionDisplayGroupSummary{
			Key:         "run:" + runID,
			RunID:       runID,
			JobCount:    byRunCount[runID],
			Collapsible: true,
		})
	}
	return out
}

func paginateJobExecutions(jobs []protocol.JobExecution, offset, limit int) []protocol.JobExecution {
	if offset >= len(jobs) {
		return []protocol.JobExecution{}
	}
	end := offset + limit
	if end > len(jobs) {
		end = len(jobs)
	}
	return append([]protocol.JobExecution(nil), jobs[offset:end]...)
}

func (s *stateStore) jobExecutionByIDHandler(w http.ResponseWriter, r *http.Request) {
	rel := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/jobs/"), "/")
	if rel == "" {
		http.NotFound(w, r)
		return
	}
	parts := strings.Split(rel, "/")
	jobID := parts[0]

	if len(parts) == 1 {
		switch r.Method {
		case http.MethodGet:
			job, err := s.db.GetJobExecution(jobID)
			if err != nil {
				http.Error(w, "job not found", http.StatusNotFound)
				return
			}
			s.attachJobExecutionTestSummary(&job)
			s.attachJobExecutionUnmetRequirementsToJobExecution(&job)
			jobResponse := jobExecutionViewFromProtocol(job)
			writeJSON(w, http.StatusOK, jobExecutionViewResponse{JobExecution: jobResponse})
		case http.MethodDelete:
			err := s.db.DeleteQueuedJobExecution(jobID)
			if err != nil {
				if strings.Contains(err.Error(), "not found") {
					http.Error(w, err.Error(), http.StatusNotFound)
					return
				}
				http.Error(w, err.Error(), http.StatusConflict)
				return
			}
			writeJSON(w, http.StatusOK, deleteJobExecutionViewResponse{Deleted: true, JobExecutionID: jobID})
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
		return
	}

	if len(parts) == 2 && parts[1] == "force-fail" {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		job, err := s.db.GetJobExecution(jobID)
		if err != nil {
			http.Error(w, "job not found", http.StatusNotFound)
			return
		}
		if !protocol.IsActiveJobExecutionStatus(job.Status) {
			http.Error(w, "job is not active", http.StatusConflict)
			return
		}
		agentID := strings.TrimSpace(job.LeasedByAgentID)
		if agentID == "" {
			agentID = "server-control"
		}
		output := strings.TrimSpace(job.Output)
		if output != "" {
			output += "\n"
		}
		output += "[control] job force-failed from UI"
		updated, err := s.db.UpdateJobExecutionStatus(jobID, protocol.JobExecutionStatusUpdateRequest{
			AgentID:      agentID,
			Status:       protocol.JobExecutionStatusFailed,
			Error:        "force-failed from UI",
			Output:       output,
			TimestampUTC: time.Now().UTC(),
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		updatedResponse := jobExecutionViewFromProtocol(updated)
		writeJSON(w, http.StatusOK, jobExecutionViewResponse{JobExecution: updatedResponse})
		return
	}

	if len(parts) == 2 && parts[1] == "status" {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req protocol.JobExecutionStatusUpdateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON body", http.StatusBadRequest)
			return
		}
		if req.AgentID == "" {
			http.Error(w, "agent_id is required", http.StatusBadRequest)
			return
		}
		if !isValidUpdateStatus(req.Status) {
			http.Error(w, "status must be running, succeeded or failed", http.StatusBadRequest)
			return
		}
		job, err := s.db.UpdateJobExecutionStatus(jobID, req)
		if err != nil {
			if strings.Contains(err.Error(), "another agent") {
				http.Error(w, err.Error(), http.StatusConflict)
				return
			}
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if patch := parseJobExecutionBuildMetadataFromOutput(req.Output); len(patch) > 0 {
			if merged, err := s.db.MergeJobExecutionMetadata(jobID, patch); err == nil {
				job.Metadata = merged
			}
		}
		// Status updates are a liveness signal while an agent is busy.
		s.markAgentSeen(req.AgentID, req.TimestampUTC)
		writeJSON(w, http.StatusOK, jobExecutionViewResponse{JobExecution: jobExecutionViewFromProtocol(job)})
		return
	}

	if len(parts) == 2 && parts[1] == "artifacts" {
		switch r.Method {
		case http.MethodGet:
			artifacts, err := s.db.ListJobExecutionArtifacts(jobID)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			artifacts = appendSyntheticJobExecutionTestReportArtifact(s.artifactsDir, jobID, artifacts)
			for i := range artifacts {
				artifacts[i].URL = "/artifacts/" + strings.TrimPrefix(filepath.ToSlash(artifacts[i].URL), "/")
			}
			writeJSON(w, http.StatusOK, protocol.JobExecutionArtifactsResponse{Artifacts: artifacts})
		case http.MethodPost:
			var req protocol.UploadArtifactsRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "invalid JSON body", http.StatusBadRequest)
				return
			}
			if req.AgentID == "" {
				http.Error(w, "agent_id is required", http.StatusBadRequest)
				return
			}
			job, err := s.db.GetJobExecution(jobID)
			if err != nil {
				http.Error(w, "job not found", http.StatusNotFound)
				return
			}
			if job.LeasedByAgentID != "" && job.LeasedByAgentID != req.AgentID {
				http.Error(w, "job is leased by another agent", http.StatusConflict)
				return
			}

			artifacts, err := s.persistJobExecutionArtifacts(jobID, req.Artifacts)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if err := s.db.SaveJobExecutionArtifacts(jobID, artifacts); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			for i := range artifacts {
				artifacts[i].URL = "/artifacts/" + strings.TrimPrefix(filepath.ToSlash(artifacts[i].URL), "/")
			}
			// Artifact upload traffic confirms the agent is alive.
			s.markAgentSeen(req.AgentID, time.Now().UTC())
			writeJSON(w, http.StatusOK, protocol.JobExecutionArtifactsResponse{Artifacts: artifacts})
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
		return
	}

	if len(parts) == 2 && parts[1] == "tests" {
		switch r.Method {
		case http.MethodGet:
			report, found, err := s.db.GetJobExecutionTestReport(jobID)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if !found {
				writeJSON(w, http.StatusOK, protocol.JobExecutionTestReportResponse{})
				return
			}
			writeJSON(w, http.StatusOK, protocol.JobExecutionTestReportResponse{Report: report})
		case http.MethodPost:
			var req protocol.UploadTestReportRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "invalid JSON body", http.StatusBadRequest)
				return
			}
			if strings.TrimSpace(req.AgentID) == "" {
				http.Error(w, "agent_id is required", http.StatusBadRequest)
				return
			}
			job, err := s.db.GetJobExecution(jobID)
			if err != nil {
				http.Error(w, "job not found", http.StatusNotFound)
				return
			}
			if job.LeasedByAgentID != "" && job.LeasedByAgentID != req.AgentID {
				http.Error(w, "job is leased by another agent", http.StatusConflict)
				return
			}
			if err := s.db.SaveJobExecutionTestReport(jobID, req.Report); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if err := s.persistJobExecutionTestReportArtifact(jobID, req.Report); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			// Test report upload is also a liveness signal.
			s.markAgentSeen(req.AgentID, time.Now().UTC())
			writeJSON(w, http.StatusOK, protocol.JobExecutionTestReportResponse{Report: req.Report})
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
		return
	}

	http.NotFound(w, r)
}

func (s *stateStore) clearJobExecutionQueueHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	n, err := s.db.ClearQueuedJobExecutions()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, clearJobExecutionQueueViewResponse{Cleared: n})
}

func (s *stateStore) flushJobExecutionHistoryHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	n, err := s.db.FlushJobExecutionHistory()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, flushJobExecutionHistoryViewResponse{Flushed: n})
}
