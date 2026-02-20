package jobexecution

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/izzyreal/ciwi/internal/protocol"
	"github.com/izzyreal/ciwi/internal/server/httpx"
)

func handleJobCancel(w http.ResponseWriter, r *http.Request, deps HandlerDeps, jobID string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	job, err := deps.Store.GetJobExecution(jobID)
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
	output += "[control] job cancelled by user"
	updated, err := deps.Store.UpdateJobExecutionStatus(jobID, protocol.JobExecutionStatusUpdateRequest{
		AgentID:      agentID,
		Status:       protocol.JobExecutionStatusFailed,
		Error:        "cancelled by user",
		Output:       output,
		TimestampUTC: nowUTC(deps),
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, SingleViewResponse{JobExecution: ViewFromProtocol(updated)})
}

func handleJobRerun(w http.ResponseWriter, r *http.Request, deps HandlerDeps, jobID string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	job, err := deps.Store.GetJobExecution(jobID)
	if err != nil {
		http.Error(w, "job not found", http.StatusNotFound)
		return
	}
	if job.StartedUTC.IsZero() {
		http.Error(w, "job has not started yet", http.StatusConflict)
		return
	}
	clone, err := deps.Store.CreateJobExecution(protocol.CreateJobExecutionRequest{
		Script:               job.Script,
		Env:                  cloneStringMap(job.Env),
		RequiredCapabilities: cloneStringMap(job.RequiredCapabilities),
		TimeoutSeconds:       job.TimeoutSeconds,
		ArtifactGlobs:        append([]string(nil), job.ArtifactGlobs...),
		Caches:               cloneJobCaches(job.Caches),
		Source:               cloneSource(job.Source),
		Metadata:             cloneStringMap(job.Metadata),
		StepPlan:             cloneJobStepPlan(job.StepPlan),
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, CreateViewResponse{JobExecution: ViewFromProtocol(clone)})
}

func handleJobStatus(w http.ResponseWriter, r *http.Request, deps HandlerDeps, jobID string) {
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
	if !protocol.IsValidJobExecutionUpdateStatus(req.Status) {
		http.Error(w, "status must be running, succeeded or failed", http.StatusBadRequest)
		return
	}
	job, err := deps.Store.UpdateJobExecutionStatus(jobID, req)
	if err != nil {
		if strings.Contains(err.Error(), "another agent") {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if len(req.Events) > 0 {
		_ = deps.Store.AppendJobExecutionEvents(jobID, req.Events)
	}
	if deps.MarkAgentSeen != nil {
		deps.MarkAgentSeen(req.AgentID, req.TimestampUTC)
	}
	if protocol.NormalizeJobExecutionStatus(job.Status) == protocol.JobExecutionStatusRunning &&
		strings.TrimSpace(job.CurrentStep) != "" && strings.TrimSpace(job.Output) == "" {
		slog.Warn("job running without output snapshot",
			"job_execution_id", jobID,
			"agent_id", req.AgentID,
			"current_step", job.CurrentStep,
		)
	}
	if protocol.IsTerminalJobExecutionStatus(job.Status) {
		slog.Info("job terminal status recorded",
			"job_execution_id", jobID,
			"agent_id", req.AgentID,
			"status", job.Status,
			"exit_code", job.ExitCode,
			"error", strings.TrimSpace(job.Error),
		)
	}
	if deps.OnJobUpdated != nil {
		deps.OnJobUpdated(job)
	}
	httpx.WriteJSON(w, http.StatusOK, SingleViewResponse{JobExecution: ViewFromProtocol(job)})
}

func handleJobArtifacts(w http.ResponseWriter, r *http.Request, deps HandlerDeps, jobID string) {
	switch r.Method {
	case http.MethodGet:
		artifacts, err := listArtifactsWithSynthetic(deps, jobID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		for i := range artifacts {
			artifacts[i].URL = "/artifacts/" + strings.TrimPrefix(filepath.ToSlash(artifacts[i].URL), "/")
		}
		httpx.WriteJSON(w, http.StatusOK, protocol.JobExecutionArtifactsResponse{Artifacts: artifacts})
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
		job, err := deps.Store.GetJobExecution(jobID)
		if err != nil {
			http.Error(w, "job not found", http.StatusNotFound)
			return
		}
		if job.LeasedByAgentID != "" && job.LeasedByAgentID != req.AgentID {
			http.Error(w, "job is leased by another agent", http.StatusConflict)
			return
		}
		artifacts, err := PersistArtifacts(deps.ArtifactsDir, jobID, req.Artifacts)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := deps.Store.SaveJobExecutionArtifacts(jobID, artifacts); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		for i := range artifacts {
			artifacts[i].URL = "/artifacts/" + strings.TrimPrefix(filepath.ToSlash(artifacts[i].URL), "/")
		}
		if deps.MarkAgentSeen != nil {
			deps.MarkAgentSeen(req.AgentID, nowUTC(deps))
		}
		httpx.WriteJSON(w, http.StatusOK, protocol.JobExecutionArtifactsResponse{Artifacts: artifacts})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleJobArtifactsDownloadAll(w http.ResponseWriter, r *http.Request, deps HandlerDeps, jobID string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	artifacts, err := listArtifactsWithSynthetic(deps, jobID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := writeArtifactsZIP(w, deps.ArtifactsDir, jobID, artifacts); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func handleJobTests(w http.ResponseWriter, r *http.Request, deps HandlerDeps, jobID string) {
	switch r.Method {
	case http.MethodGet:
		report, found, err := deps.Store.GetJobExecutionTestReport(jobID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if !found {
			httpx.WriteJSON(w, http.StatusOK, protocol.JobExecutionTestReportResponse{})
			return
		}
		httpx.WriteJSON(w, http.StatusOK, protocol.JobExecutionTestReportResponse{Report: report})
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
		job, err := deps.Store.GetJobExecution(jobID)
		if err != nil {
			http.Error(w, "job not found", http.StatusNotFound)
			return
		}
		if job.LeasedByAgentID != "" && job.LeasedByAgentID != req.AgentID {
			http.Error(w, "job is leased by another agent", http.StatusConflict)
			return
		}
		if err := deps.Store.SaveJobExecutionTestReport(jobID, req.Report); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := PersistTestReportArtifact(deps.ArtifactsDir, jobID, req.Report); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := PersistCoverageReportArtifact(deps.ArtifactsDir, jobID, req.Report); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if deps.MarkAgentSeen != nil {
			deps.MarkAgentSeen(req.AgentID, nowUTC(deps))
		}
		httpx.WriteJSON(w, http.StatusOK, protocol.JobExecutionTestReportResponse{Report: req.Report})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
