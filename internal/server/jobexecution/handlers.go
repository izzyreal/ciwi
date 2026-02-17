package jobexecution

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/izzyreal/ciwi/internal/protocol"
	"github.com/izzyreal/ciwi/internal/server/httpx"
)

type Store interface {
	ListJobExecutions() ([]protocol.JobExecution, error)
	CreateJobExecution(req protocol.CreateJobExecutionRequest) (protocol.JobExecution, error)
	GetJobExecution(id string) (protocol.JobExecution, error)
	DeleteQueuedJobExecution(id string) error
	UpdateJobExecutionStatus(id string, req protocol.JobExecutionStatusUpdateRequest) (protocol.JobExecution, error)
	AppendJobExecutionEvents(id string, events []protocol.JobExecutionEvent) error
	ListJobExecutionArtifacts(id string) ([]protocol.JobExecutionArtifact, error)
	SaveJobExecutionArtifacts(id string, artifacts []protocol.JobExecutionArtifact) error
	GetJobExecutionTestReport(id string) (protocol.JobExecutionTestReport, bool, error)
	SaveJobExecutionTestReport(id string, report protocol.JobExecutionTestReport) error
	ClearQueuedJobExecutions() (int64, error)
	FlushJobExecutionHistory() (int64, error)
}

type HandlerDeps struct {
	Store                              Store
	ArtifactsDir                       string
	AttachTestSummaries                func([]protocol.JobExecution)
	AttachUnmetRequirements            func([]protocol.JobExecution)
	AttachTestSummary                  func(*protocol.JobExecution)
	AttachUnmetRequirementsToExecution func(*protocol.JobExecution)
	MarkAgentSeen                      func(agentID string, ts time.Time)
	OnJobUpdated                       func(job protocol.JobExecution)
	Now                                func() time.Time
}

func HandleCollection(w http.ResponseWriter, r *http.Request, deps HandlerDeps) {
	if deps.Store == nil {
		http.Error(w, "job execution store unavailable", http.StatusInternalServerError)
		return
	}

	switch r.Method {
	case http.MethodGet:
		view := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("view")))
		maxJobs := ParseQueryInt(r, "max", 150, 1, 2000)

		jobs, err := deps.Store.ListJobExecutions()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		jobs = CapDisplayJobs(jobs, maxJobs)

		queuedJobs, historyJobs := SplitByState(jobs)
		switch view {
		case "summary":
			queuedGroups := SummarizeDisplayGroups(queuedJobs)
			historyGroups := SummarizeDisplayGroups(historyJobs)
			httpx.WriteJSON(w, http.StatusOK, SummaryViewResponse{
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
			offset := ParseQueryInt(r, "offset", 0, 0, 1_000_000)
			limit := ParseQueryInt(r, "limit", 25, 1, 200)
			page := Paginate(source, offset, limit)
			if deps.AttachTestSummaries != nil {
				deps.AttachTestSummaries(page)
			}
			if deps.AttachUnmetRequirements != nil {
				deps.AttachUnmetRequirements(page)
			}
			pageViews := ViewsFromProtocol(page)
			httpx.WriteJSON(w, http.StatusOK, PagedViewResponse{
				View:          view,
				Total:         len(source),
				Offset:        offset,
				Limit:         limit,
				JobExecutions: pageViews,
			})
			return
		}

		if deps.AttachTestSummaries != nil {
			deps.AttachTestSummaries(jobs)
		}
		if deps.AttachUnmetRequirements != nil {
			deps.AttachUnmetRequirements(jobs)
		}
		httpx.WriteJSON(w, http.StatusOK, ListViewResponse{JobExecutions: ViewsFromProtocol(jobs)})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func HandleByID(w http.ResponseWriter, r *http.Request, deps HandlerDeps) {
	if deps.Store == nil {
		http.Error(w, "job execution store unavailable", http.StatusInternalServerError)
		return
	}

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
			job, err := deps.Store.GetJobExecution(jobID)
			if err != nil {
				http.Error(w, "job not found", http.StatusNotFound)
				return
			}
			if deps.AttachTestSummary != nil {
				deps.AttachTestSummary(&job)
			}
			if deps.AttachUnmetRequirementsToExecution != nil {
				deps.AttachUnmetRequirementsToExecution(&job)
			}
			httpx.WriteJSON(w, http.StatusOK, SingleViewResponse{JobExecution: ViewFromProtocol(job)})
		case http.MethodDelete:
			if err := deps.Store.DeleteQueuedJobExecution(jobID); err != nil {
				if strings.Contains(err.Error(), "not found") {
					http.Error(w, err.Error(), http.StatusNotFound)
					return
				}
				http.Error(w, err.Error(), http.StatusConflict)
				return
			}
			httpx.WriteJSON(w, http.StatusOK, DeleteViewResponse{Deleted: true, JobExecutionID: jobID})
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
		output += "[control] job force-failed from UI"
		updated, err := deps.Store.UpdateJobExecutionStatus(jobID, protocol.JobExecutionStatusUpdateRequest{
			AgentID:      agentID,
			Status:       protocol.JobExecutionStatusFailed,
			Error:        "force-failed from UI",
			Output:       output,
			TimestampUTC: nowUTC(deps),
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, SingleViewResponse{JobExecution: ViewFromProtocol(updated)})
		return
	}

	if len(parts) == 2 && parts[1] == "rerun" {
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
		return
	}

	if len(parts) == 2 && parts[1] == "artifacts" {
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
		return
	}

	if len(parts) == 3 && parts[1] == "artifacts" && parts[2] == "download-all" {
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
		return
	}

	if len(parts) == 2 && parts[1] == "tests" {
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
		return
	}

	http.NotFound(w, r)
}

func listArtifactsWithSynthetic(deps HandlerDeps, jobID string) ([]protocol.JobExecutionArtifact, error) {
	artifacts, err := deps.Store.ListJobExecutionArtifacts(jobID)
	if err != nil {
		return nil, err
	}
	artifacts = AppendSyntheticTestReportArtifact(deps.ArtifactsDir, jobID, artifacts)
	artifacts = AppendSyntheticCoverageReportArtifact(deps.ArtifactsDir, jobID, artifacts)
	return artifacts, nil
}

func writeArtifactsZIP(w http.ResponseWriter, artifactsDir, jobID string, artifacts []protocol.JobExecutionArtifact) error {
	zipPrefix := sanitizeZIPName(jobID)
	if zipPrefix == "" {
		zipPrefix = "job"
	}
	tmp, err := os.CreateTemp("", "ciwi-"+zipPrefix+"-artifacts-*.zip")
	if err != nil {
		return fmt.Errorf("create temp zip: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	zw := zip.NewWriter(tmp)
	// Deterministic order yields stable archive listing in the UI.
	sort.SliceStable(artifacts, func(i, j int) bool { return artifacts[i].Path < artifacts[j].Path })
	for _, a := range artifacts {
		rel := filepath.ToSlash(filepath.Clean(strings.TrimSpace(a.Path)))
		if rel == "" || rel == "." || strings.HasPrefix(rel, "/") || strings.Contains(rel, "..") {
			continue
		}
		full := filepath.Join(artifactsDir, jobID, filepath.FromSlash(rel))
		info, err := os.Stat(full)
		if err != nil || info.IsDir() {
			continue
		}
		fh, err := zip.FileInfoHeader(info)
		if err != nil {
			_ = zw.Close()
			_ = tmp.Close()
			return fmt.Errorf("build zip header for %q: %w", rel, err)
		}
		fh.Name = rel
		fh.Method = zip.Deflate
		entry, err := zw.CreateHeader(fh)
		if err != nil {
			_ = zw.Close()
			_ = tmp.Close()
			return fmt.Errorf("create zip entry for %q: %w", rel, err)
		}
		f, err := os.Open(full)
		if err != nil {
			_ = zw.Close()
			_ = tmp.Close()
			return fmt.Errorf("open artifact %q: %w", rel, err)
		}
		_, copyErr := io.Copy(entry, f)
		closeErr := f.Close()
		if copyErr != nil {
			_ = zw.Close()
			_ = tmp.Close()
			return fmt.Errorf("zip artifact %q: %w", rel, copyErr)
		}
		if closeErr != nil {
			_ = zw.Close()
			_ = tmp.Close()
			return fmt.Errorf("close artifact %q: %w", rel, closeErr)
		}
	}
	if err := zw.Close(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("finalize zip: %w", err)
	}
	if _, err := tmp.Seek(0, 0); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("rewind zip: %w", err)
	}
	defer tmp.Close()

	fileName := sanitizeZIPName(jobID) + "-artifacts.zip"
	if strings.TrimSpace(fileName) == "-artifacts.zip" {
		fileName = "job-artifacts.zip"
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="`+fileName+`"`)
	w.WriteHeader(http.StatusOK)
	if _, err := io.Copy(w, tmp); err != nil {
		return fmt.Errorf("stream zip: %w", err)
	}
	return nil
}

func sanitizeZIPName(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range v {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_' || r == '.':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	return strings.Trim(strings.TrimSpace(b.String()), "-.")
}

func HandleClearQueue(w http.ResponseWriter, r *http.Request, deps HandlerDeps) {
	if deps.Store == nil {
		http.Error(w, "job execution store unavailable", http.StatusInternalServerError)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	n, err := deps.Store.ClearQueuedJobExecutions()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, ClearQueueViewResponse{Cleared: n})
}

func HandleFlushHistory(w http.ResponseWriter, r *http.Request, deps HandlerDeps) {
	if deps.Store == nil {
		http.Error(w, "job execution store unavailable", http.StatusInternalServerError)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	n, err := deps.Store.FlushJobExecutionHistory()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, FlushHistoryViewResponse{Flushed: n})
}

func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func cloneJobCaches(in []protocol.JobCacheSpec) []protocol.JobCacheSpec {
	if len(in) == 0 {
		return nil
	}
	out := make([]protocol.JobCacheSpec, 0, len(in))
	for _, c := range in {
		out = append(out, protocol.JobCacheSpec{
			ID:          c.ID,
			Env:         c.Env,
			Key:         cloneJobCacheKey(c.Key),
			RestoreKeys: append([]string(nil), c.RestoreKeys...),
			Policy:      c.Policy,
			TTLDays:     c.TTLDays,
			MaxSizeMB:   c.MaxSizeMB,
		})
	}
	return out
}

func cloneJobCacheKey(in protocol.JobCacheKey) protocol.JobCacheKey {
	return protocol.JobCacheKey{
		Prefix:  in.Prefix,
		Files:   append([]string(nil), in.Files...),
		Runtime: append([]string(nil), in.Runtime...),
		Tools:   append([]string(nil), in.Tools...),
		Env:     append([]string(nil), in.Env...),
		GitRefs: append([]protocol.JobCacheKeyGitRef(nil), in.GitRefs...),
	}
}

func cloneSource(in *protocol.SourceSpec) *protocol.SourceSpec {
	if in == nil {
		return nil
	}
	return &protocol.SourceSpec{
		Repo: in.Repo,
		Ref:  in.Ref,
	}
}

func cloneJobStepPlan(in []protocol.JobStepPlanItem) []protocol.JobStepPlanItem {
	if len(in) == 0 {
		return nil
	}
	out := make([]protocol.JobStepPlanItem, 0, len(in))
	for _, step := range in {
		out = append(out, protocol.JobStepPlanItem{
			Index:          step.Index,
			Total:          step.Total,
			Name:           step.Name,
			Script:         step.Script,
			Kind:           step.Kind,
			TestName:       step.TestName,
			TestFormat:     step.TestFormat,
			TestReport:     step.TestReport,
			CoverageFormat: step.CoverageFormat,
			CoverageReport: step.CoverageReport,
		})
	}
	return out
}

func nowUTC(deps HandlerDeps) time.Time {
	if deps.Now != nil {
		ts := deps.Now()
		if !ts.IsZero() {
			return ts.UTC()
		}
	}
	return time.Now().UTC()
}
