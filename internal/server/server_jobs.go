package server

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/izzyreal/ciwi/internal/protocol"
)

const testReportArtifactPath = "test-report.json"

func (s *stateStore) jobsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		jobs, err := s.db.ListJobs()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
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
		status := strings.ToLower(strings.TrimSpace(job.Status))
		switch status {
		case "queued", "leased", "running":
		default:
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
			Status:       "failed",
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

func (s *stateStore) attachTestSummaries(jobs []protocol.Job) {
	for i := range jobs {
		s.attachTestSummary(&jobs[i])
	}
}

func (s *stateStore) markAgentSeen(agentID string, ts time.Time) {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return
	}
	if ts.IsZero() {
		ts = time.Now().UTC()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.agents[agentID]
	if !ok {
		return
	}
	a.LastSeenUTC = ts
	s.agents[agentID] = a
}

func (s *stateStore) attachTestSummary(job *protocol.Job) {
	if job == nil || strings.TrimSpace(job.ID) == "" {
		return
	}
	report, found, err := s.db.GetJobTestReport(job.ID)
	if err != nil || !found {
		return
	}
	job.TestSummary = &protocol.JobTestSummary{
		Total:   report.Total,
		Passed:  report.Passed,
		Failed:  report.Failed,
		Skipped: report.Skipped,
	}
}

func (s *stateStore) attachUnmetRequirements(jobs []protocol.Job) {
	s.mu.Lock()
	agents := make(map[string]agentState, len(s.agents))
	for id, a := range s.agents {
		agents[id] = a
	}
	s.mu.Unlock()
	for i := range jobs {
		if strings.ToLower(strings.TrimSpace(jobs[i].Status)) != "queued" {
			continue
		}
		jobs[i].UnmetRequirements = diagnoseUnmetRequirements(jobs[i].RequiredCapabilities, agents)
	}
}

func (s *stateStore) attachUnmetRequirementsToJob(job *protocol.Job) {
	if job == nil {
		return
	}
	if strings.ToLower(strings.TrimSpace(job.Status)) != "queued" {
		return
	}
	s.mu.Lock()
	agents := make(map[string]agentState, len(s.agents))
	for id, a := range s.agents {
		agents[id] = a
	}
	s.mu.Unlock()
	job.UnmetRequirements = diagnoseUnmetRequirements(job.RequiredCapabilities, agents)
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

func (s *stateStore) persistArtifacts(jobID string, incoming []protocol.UploadArtifact) ([]protocol.JobArtifact, error) {
	if len(incoming) == 0 {
		return nil, nil
	}
	base := filepath.Join(s.artifactsDir, jobID)
	if err := os.MkdirAll(base, 0o755); err != nil {
		return nil, fmt.Errorf("create artifact dir: %w", err)
	}

	artifacts := make([]protocol.JobArtifact, 0, len(incoming))
	for _, in := range incoming {
		rel := filepath.ToSlash(filepath.Clean(in.Path))
		if rel == "." || rel == "" || strings.HasPrefix(rel, "/") || strings.Contains(rel, "..") {
			return nil, fmt.Errorf("invalid artifact path: %q", in.Path)
		}

		decoded, err := base64.StdEncoding.DecodeString(in.DataBase64)
		if err != nil {
			return nil, fmt.Errorf("decode artifact %q: %w", in.Path, err)
		}

		dst := filepath.Join(base, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return nil, fmt.Errorf("mkdir artifact parent: %w", err)
		}
		if err := os.WriteFile(dst, decoded, 0o644); err != nil {
			return nil, fmt.Errorf("write artifact %q: %w", in.Path, err)
		}

		storedRel := filepath.ToSlash(filepath.Join(jobID, filepath.FromSlash(rel)))
		artifacts = append(artifacts, protocol.JobArtifact{
			JobID:     jobID,
			Path:      rel,
			URL:       storedRel,
			SizeBytes: int64(len(decoded)),
		})
	}
	return artifacts, nil
}

func (s *stateStore) persistTestReportArtifact(jobID string, report protocol.JobTestReport) error {
	base := filepath.Join(s.artifactsDir, jobID)
	if err := os.MkdirAll(base, 0o755); err != nil {
		return fmt.Errorf("create test report artifact dir: %w", err)
	}
	dst := filepath.Join(base, filepath.FromSlash(testReportArtifactPath))
	payload, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal test report artifact: %w", err)
	}
	if err := os.WriteFile(dst, payload, 0o644); err != nil {
		return fmt.Errorf("write test report artifact: %w", err)
	}
	return nil
}

func appendSyntheticTestReportArtifact(artifactsDir, jobID string, artifacts []protocol.JobArtifact) []protocol.JobArtifact {
	testReportFull := filepath.Join(artifactsDir, jobID, filepath.FromSlash(testReportArtifactPath))
	info, err := os.Stat(testReportFull)
	if err != nil || info.IsDir() {
		return artifacts
	}
	for _, a := range artifacts {
		if a.Path == testReportArtifactPath {
			return artifacts
		}
	}
	artifacts = append(artifacts, protocol.JobArtifact{
		JobID:     jobID,
		Path:      testReportArtifactPath,
		URL:       filepath.ToSlash(filepath.Join(jobID, filepath.FromSlash(testReportArtifactPath))),
		SizeBytes: info.Size(),
	})
	sort.SliceStable(artifacts, func(i, j int) bool { return artifacts[i].Path < artifacts[j].Path })
	return artifacts
}
