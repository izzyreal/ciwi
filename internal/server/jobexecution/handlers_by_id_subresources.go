package jobexecution

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/izzyreal/ciwi/internal/application"
	"github.com/izzyreal/ciwi/internal/protocol"
	"github.com/izzyreal/ciwi/internal/server/httpx"
)

func handleJobCancel(w http.ResponseWriter, r *http.Request, deps HandlerDeps, jobID string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if deps.ExecutionControls != nil {
		result, err := deps.ExecutionControls.Cancel(r.Context(), application.ExecutionControlRequest{
			JobExecutionID: jobID, IdempotencyKey: r.Header.Get("Idempotency-Key"),
		})
		if err != nil {
			writeApplicationError(w, err)
			return
		}
		updated, err := deps.Store.GetJobExecution(result.JobExecutionID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, SingleViewResponse{JobExecution: ViewFromProtocol(updated)})
		return
	}
	updated, err := CancelJobExecution(deps.Store, jobID, nowUTC(deps))
	if err != nil {
		status := http.StatusBadRequest
		if strings.Contains(err.Error(), "not found") {
			status = http.StatusNotFound
		} else if strings.Contains(err.Error(), "not active") {
			status = http.StatusConflict
		}
		http.Error(w, err.Error(), status)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, SingleViewResponse{JobExecution: ViewFromProtocol(updated)})
}

func handleJobRerun(w http.ResponseWriter, r *http.Request, deps HandlerDeps, jobID string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if deps.ExecutionControls != nil {
		result, err := deps.ExecutionControls.Rerun(r.Context(), application.ExecutionControlRequest{
			JobExecutionID: jobID, IdempotencyKey: r.Header.Get("Idempotency-Key"),
		})
		if err != nil {
			writeApplicationError(w, err)
			return
		}
		clone, err := deps.Store.GetJobExecution(result.JobExecutionID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		httpx.WriteJSON(w, http.StatusCreated, CreateViewResponse{JobExecution: ViewFromProtocol(clone)})
		return
	}
	clone, err := RerunJobExecution(deps.Store, jobID, deps.PrepareRerun)
	if err != nil {
		status := http.StatusBadRequest
		if strings.Contains(err.Error(), "not found") {
			status = http.StatusNotFound
		} else if strings.Contains(err.Error(), "has not started") || strings.Contains(err.Error(), "dependencies") || strings.Contains(err.Error(), "required job") {
			status = http.StatusConflict
		}
		http.Error(w, err.Error(), status)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, CreateViewResponse{JobExecution: ViewFromProtocol(clone)})
}

func writeApplicationError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	switch application.ErrorKindOf(err) {
	case application.ErrorInvalidArgument:
		status = http.StatusBadRequest
	case application.ErrorNotFound:
		status = http.StatusNotFound
	case application.ErrorConflict:
		status = http.StatusConflict
	case application.ErrorFailedPrecondition:
		status = http.StatusPreconditionFailed
	case application.ErrorUnavailable:
		status = http.StatusServiceUnavailable
	case application.ErrorUnsupported:
		status = http.StatusNotImplemented
	}
	http.Error(w, err.Error(), status)
}

func CancelJobExecution(store Store, jobID string, now time.Time) (protocol.JobExecution, error) {
	job, err := store.GetJobExecution(jobID)
	if err != nil {
		return protocol.JobExecution{}, fmt.Errorf("job not found: %w", err)
	}
	if !protocol.IsActiveJobExecutionStatus(job.Status) {
		return protocol.JobExecution{}, fmt.Errorf("job is not active")
	}
	agentID := strings.TrimSpace(job.LeasedByAgentID)
	if agentID == "" {
		agentID = "server-control"
	}
	updated, err := store.UpdateJobExecutionStatus(jobID, protocol.JobExecutionStatusUpdateRequest{
		AgentID: agentID, Status: protocol.JobExecutionStatusFailed,
		Error: "cancelled by user", TimestampUTC: now,
	})
	if err != nil {
		return protocol.JobExecution{}, err
	}
	if err := store.AppendJobExecutionEvents(jobID, []protocol.JobExecutionEvent{{
		Type: protocol.JobExecutionEventTypeSystemMessage, TimestampUTC: now,
		Message: "[control] job cancelled by user",
	}}); err != nil {
		return protocol.JobExecution{}, err
	}
	return updated, nil
}

func RerunJobExecution(store Store, jobID string, prepare func(protocol.JobExecution, *protocol.CreateJobExecutionRequest) error) (protocol.JobExecution, error) {
	job, err := store.GetJobExecution(jobID)
	if err != nil {
		return protocol.JobExecution{}, fmt.Errorf("job not found: %w", err)
	}
	if job.StartedUTC.IsZero() && !isDependencyBlockedJob(job) {
		return protocol.JobExecution{}, fmt.Errorf("job has not started yet")
	}
	metadata := cloneStringMap(job.Metadata)
	if metadata == nil {
		metadata = map[string]string{}
	}
	rootID := protocol.JobExecutionAttemptRootID(job)
	if rootID == "" {
		rootID = job.ID
	}
	metadata[protocol.JobMetadataAttemptRootJobID] = rootID
	metadata[protocol.JobMetadataRerunOfJobID] = job.ID
	request := protocol.CreateJobExecutionRequest{
		Script: job.Script, Env: cloneStringMap(job.Env), RequiredCapabilities: cloneStringMap(job.RequiredCapabilities),
		TimeoutSeconds: job.TimeoutSeconds, ArtifactGlobs: append([]string(nil), job.ArtifactGlobs...),
		DependencyArtifactJobIDs: append([]string(nil), job.DependencyArtifactJobIDs...), Caches: cloneJobCaches(job.Caches),
		Source: cloneSource(job.Source), Metadata: metadata, StepPlan: cloneJobStepPlan(job.StepPlan),
	}
	if prepare != nil {
		if err := prepare(job, &request); err != nil {
			return protocol.JobExecution{}, err
		}
	}
	return store.CreateJobExecution(request)
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
		if err := deps.Store.AppendJobExecutionEvents(jobID, req.Events); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	if deps.MarkAgentSeen != nil {
		deps.MarkAgentSeen(req.AgentID, nowUTC(deps))
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
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	artifacts, err := listArtifactsWithSynthetic(deps, jobID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	for i := range artifacts {
		artifacts[i].URL = "/artifacts/" + strings.TrimPrefix(filepath.ToSlash(artifacts[i].URL), "/")
	}
	httpx.WriteJSON(w, http.StatusOK, protocol.JobExecutionArtifactsResponse{Artifacts: artifacts})
}

func handleJobArtifactsUploadZIP(w http.ResponseWriter, r *http.Request, deps HandlerDeps, jobID string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	agentID := strings.TrimSpace(r.Header.Get("X-CIWI-Agent-ID"))
	if agentID == "" {
		agentID = strings.TrimSpace(r.URL.Query().Get("agent_id"))
	}
	if agentID == "" {
		http.Error(w, "agent_id is required", http.StatusBadRequest)
		return
	}
	job, err := deps.Store.GetJobExecution(jobID)
	if err != nil {
		http.Error(w, "job not found", http.StatusNotFound)
		return
	}
	if job.LeasedByAgentID != "" && job.LeasedByAgentID != agentID {
		http.Error(w, "job is leased by another agent", http.StatusConflict)
		return
	}
	payload, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed reading zip payload", http.StatusBadRequest)
		return
	}
	artifacts, err := PersistArtifactsZIP(deps.ArtifactsDir, jobID, payload)
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
		deps.MarkAgentSeen(agentID, nowUTC(deps))
	}
	httpx.WriteJSON(w, http.StatusOK, protocol.JobExecutionArtifactsResponse{Artifacts: artifacts})
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

func handleJobArtifactsDownload(w http.ResponseWriter, r *http.Request, deps HandlerDeps, jobID string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	prefix, ok := normalizeRelativeArtifactPath(r.URL.Query().Get("prefix"))
	if !ok {
		http.Error(w, "invalid prefix", http.StatusBadRequest)
		return
	}
	artifacts, err := listArtifactsWithSynthetic(deps, jobID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	filtered := make([]protocol.JobExecutionArtifact, 0, len(artifacts))
	for _, a := range artifacts {
		rel, ok := normalizeRelativeArtifactPath(a.Path)
		if !ok {
			continue
		}
		if rel == prefix || strings.HasPrefix(rel, prefix+"/") {
			filtered = append(filtered, a)
		}
	}
	if len(filtered) == 0 {
		http.Error(w, "artifact directory not found", http.StatusNotFound)
		return
	}
	fileName := buildArtifactsZIPFileName(jobID, prefix)
	if err := writeArtifactsZIPWithFileName(w, deps.ArtifactsDir, jobID, filtered, fileName); err != nil {
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
