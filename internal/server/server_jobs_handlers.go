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

type jobDisplayGroupSummary struct {
	Key         string `json:"key"`
	RunID       string `json:"run_id,omitempty"`
	JobCount    int    `json:"job_count"`
	Collapsible bool   `json:"collapsible"`
}

func (s *stateStore) jobsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		view := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("view")))
		maxJobs := parseJobsQueryInt(r, "max", 150, 1, 2000)

		jobs, err := s.db.ListJobs()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if maxJobs > 0 && len(jobs) > maxJobs {
			jobs = jobs[:maxJobs]
		}

		queuedJobs, historyJobs := splitJobsByState(jobs)
		switch view {
		case "summary":
			queuedGroups := summarizeJobDisplayGroups(queuedJobs)
			historyGroups := summarizeJobDisplayGroups(historyJobs)
			writeJSON(w, http.StatusOK, map[string]any{
				"view":                "summary",
				"max":                 maxJobs,
				"total":               len(jobs),
				"queued_count":        len(queuedJobs),
				"history_count":       len(historyJobs),
				"queued_group_count":  len(queuedGroups),
				"history_group_count": len(historyGroups),
				"queued_groups":       queuedGroups,
				"history_groups":      historyGroups,
			})
			return
		case "queued", "history":
			source := queuedJobs
			if view == "history" {
				source = historyJobs
			}
			offset := parseJobsQueryInt(r, "offset", 0, 0, 1_000_000)
			limit := parseJobsQueryInt(r, "limit", 25, 1, 200)
			page := paginateJobs(source, offset, limit)
			s.attachTestSummaries(page)
			s.attachUnmetRequirements(page)
			writeJSON(w, http.StatusOK, map[string]any{
				"view":           view,
				"total":          len(source),
				"offset":         offset,
				"limit":          limit,
				"job_executions": page,
				"jobs":           page,
			})
			return
		}

		s.attachTestSummaries(jobs)
		s.attachUnmetRequirements(jobs)
		writeJSON(w, http.StatusOK, map[string]any{"job_executions": jobs, "jobs": jobs})
	case http.MethodPost:
		var req protocol.CreateJobRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON body", http.StatusBadRequest)
			return
		}
		job, err := s.db.CreateJob(req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusCreated, protocol.CreateJobResponse{Job: job})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func parseJobsQueryInt(r *http.Request, key string, fallback, min, max int) int {
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

func splitJobsByState(jobs []protocol.Job) (queued []protocol.Job, history []protocol.Job) {
	queued = make([]protocol.Job, 0, len(jobs))
	history = make([]protocol.Job, 0, len(jobs))
	for _, job := range jobs {
		if protocol.IsActiveJobStatus(job.Status) {
			queued = append(queued, job)
			continue
		}
		history = append(history, job)
	}
	return queued, history
}

func summarizeJobDisplayGroups(jobs []protocol.Job) []jobDisplayGroupSummary {
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

	out := make([]jobDisplayGroupSummary, 0, len(jobs))
	seenRunIDs := map[string]struct{}{}
	for _, job := range jobs {
		jobID := strings.TrimSpace(job.ID)
		runID := strings.TrimSpace(job.Metadata["pipeline_run_id"])
		if runID == "" || byRunCount[runID] <= 1 {
			if jobID == "" {
				continue
			}
			out = append(out, jobDisplayGroupSummary{
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
		out = append(out, jobDisplayGroupSummary{
			Key:         "run:" + runID,
			RunID:       runID,
			JobCount:    byRunCount[runID],
			Collapsible: true,
		})
	}
	return out
}

func paginateJobs(jobs []protocol.Job, offset, limit int) []protocol.Job {
	if offset >= len(jobs) {
		return []protocol.Job{}
	}
	end := offset + limit
	if end > len(jobs) {
		end = len(jobs)
	}
	return append([]protocol.Job(nil), jobs[offset:end]...)
}

func (s *stateStore) jobByIDHandler(w http.ResponseWriter, r *http.Request) {
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
			job, err := s.db.GetJob(jobID)
			if err != nil {
				http.Error(w, "job not found", http.StatusNotFound)
				return
			}
			s.attachTestSummary(&job)
			s.attachUnmetRequirementsToJob(&job)
			writeJSON(w, http.StatusOK, map[string]any{"job_execution": job, "job": job})
		case http.MethodDelete:
			err := s.db.DeleteQueuedJob(jobID)
			if err != nil {
				if strings.Contains(err.Error(), "not found") {
					http.Error(w, err.Error(), http.StatusNotFound)
					return
				}
				http.Error(w, err.Error(), http.StatusConflict)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"deleted": true, "job_id": jobID})
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
		job, err := s.db.GetJob(jobID)
		if err != nil {
			http.Error(w, "job not found", http.StatusNotFound)
			return
		}
		if !protocol.IsActiveJobStatus(job.Status) {
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
		updated, err := s.db.UpdateJobStatus(jobID, protocol.JobStatusUpdateRequest{
			AgentID:      agentID,
			Status:       protocol.JobStatusFailed,
			Error:        "force-failed from UI",
			Output:       output,
			TimestampUTC: time.Now().UTC(),
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"job_execution": updated, "job": updated})
		return
	}

	if len(parts) == 2 && parts[1] == "status" {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req protocol.JobStatusUpdateRequest
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
		job, err := s.db.UpdateJobStatus(jobID, req)
		if err != nil {
			if strings.Contains(err.Error(), "another agent") {
				http.Error(w, err.Error(), http.StatusConflict)
				return
			}
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if patch := parseBuildMetadataFromOutput(req.Output); len(patch) > 0 {
			if merged, err := s.db.MergeJobMetadata(jobID, patch); err == nil {
				job.Metadata = merged
			}
		}
		// Status updates are a liveness signal while an agent is busy.
		s.markAgentSeen(req.AgentID, req.TimestampUTC)
		writeJSON(w, http.StatusOK, map[string]any{"job": job})
		return
	}

	if len(parts) == 2 && parts[1] == "artifacts" {
		switch r.Method {
		case http.MethodGet:
			artifacts, err := s.db.ListJobArtifacts(jobID)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			artifacts = appendSyntheticTestReportArtifact(s.artifactsDir, jobID, artifacts)
			for i := range artifacts {
				artifacts[i].URL = "/artifacts/" + strings.TrimPrefix(filepath.ToSlash(artifacts[i].URL), "/")
			}
			writeJSON(w, http.StatusOK, protocol.JobArtifactsResponse{Artifacts: artifacts})
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
			job, err := s.db.GetJob(jobID)
			if err != nil {
				http.Error(w, "job not found", http.StatusNotFound)
				return
			}
			if job.LeasedByAgentID != "" && job.LeasedByAgentID != req.AgentID {
				http.Error(w, "job is leased by another agent", http.StatusConflict)
				return
			}

			artifacts, err := s.persistArtifacts(jobID, req.Artifacts)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if err := s.db.SaveJobArtifacts(jobID, artifacts); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			for i := range artifacts {
				artifacts[i].URL = "/artifacts/" + strings.TrimPrefix(filepath.ToSlash(artifacts[i].URL), "/")
			}
			// Artifact upload traffic confirms the agent is alive.
			s.markAgentSeen(req.AgentID, time.Now().UTC())
			writeJSON(w, http.StatusOK, protocol.JobArtifactsResponse{Artifacts: artifacts})
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
		return
	}

	if len(parts) == 2 && parts[1] == "tests" {
		switch r.Method {
		case http.MethodGet:
			report, found, err := s.db.GetJobTestReport(jobID)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if !found {
				writeJSON(w, http.StatusOK, protocol.JobTestReportResponse{})
				return
			}
			writeJSON(w, http.StatusOK, protocol.JobTestReportResponse{Report: report})
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
			job, err := s.db.GetJob(jobID)
			if err != nil {
				http.Error(w, "job not found", http.StatusNotFound)
				return
			}
			if job.LeasedByAgentID != "" && job.LeasedByAgentID != req.AgentID {
				http.Error(w, "job is leased by another agent", http.StatusConflict)
				return
			}
			if err := s.db.SaveJobTestReport(jobID, req.Report); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if err := s.persistTestReportArtifact(jobID, req.Report); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			// Test report upload is also a liveness signal.
			s.markAgentSeen(req.AgentID, time.Now().UTC())
			writeJSON(w, http.StatusOK, protocol.JobTestReportResponse{Report: req.Report})
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
		return
	}

	http.NotFound(w, r)
}

func (s *stateStore) clearQueueHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	n, err := s.db.ClearQueuedJobs()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"cleared": n})
}

func (s *stateStore) flushHistoryHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	n, err := s.db.FlushJobHistory()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"flushed": n})
}
