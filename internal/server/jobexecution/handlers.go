package jobexecution

import (
	"archive/zip"
	"fmt"
	"io"
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

	parsed, ok := parseJobPath(r.URL.Path)
	if !ok {
		http.NotFound(w, r)
		return
	}
	jobID := parsed.JobID
	if parsed.IsRoot() {
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

	if parsed.IsResource("cancel") {
		handleJobCancel(w, r, deps, jobID)
		return
	}

	if parsed.IsResource("rerun") {
		handleJobRerun(w, r, deps, jobID)
		return
	}

	if parsed.IsResource("status") {
		handleJobStatus(w, r, deps, jobID)
		return
	}

	if parsed.IsResource("artifacts") {
		handleJobArtifacts(w, r, deps, jobID)
		return
	}

	if parsed.IsNestedResource("artifacts", "upload-zip") {
		handleJobArtifactsUploadZIP(w, r, deps, jobID)
		return
	}

	if parsed.IsNestedResource("artifacts", "download") {
		handleJobArtifactsDownload(w, r, deps, jobID)
		return
	}

	if parsed.IsNestedResource("artifacts", "download-all") {
		handleJobArtifactsDownloadAll(w, r, deps, jobID)
		return
	}

	if parsed.IsResource("tests") {
		handleJobTests(w, r, deps, jobID)
		return
	}

	if parsed.IsResource("blocked-by") {
		handleJobBlockedBy(w, r, deps, jobID)
		return
	}

	http.NotFound(w, r)
}

type jobPath struct {
	JobID      string
	Resource   string
	Subpath    string
	partsCount int
}

func parseJobPath(path string) (jobPath, bool) {
	rel := strings.Trim(strings.TrimPrefix(path, "/api/v1/jobs/"), "/")
	if rel == "" {
		return jobPath{}, false
	}
	parts := strings.Split(rel, "/")
	if len(parts) == 0 || strings.TrimSpace(parts[0]) == "" {
		return jobPath{}, false
	}
	if len(parts) > 3 {
		return jobPath{}, false
	}
	out := jobPath{
		JobID:      parts[0],
		partsCount: len(parts),
	}
	if len(parts) >= 2 {
		out.Resource = parts[1]
	}
	if len(parts) == 3 {
		out.Subpath = parts[2]
	}
	return out, true
}

func (p jobPath) IsRoot() bool {
	return p.partsCount == 1
}

func (p jobPath) IsResource(name string) bool {
	return p.partsCount == 2 && p.Resource == name
}

func (p jobPath) IsNestedResource(resource, subpath string) bool {
	return p.partsCount == 3 && p.Resource == resource && p.Subpath == subpath
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
	fileName := buildArtifactsZIPFileName(jobID, "")
	return writeArtifactsZIPWithFileName(w, artifactsDir, jobID, artifacts, fileName)
}

func writeArtifactsZIPWithFileName(w http.ResponseWriter, artifactsDir, jobID string, artifacts []protocol.JobExecutionArtifact, fileName string) error {
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
		rel, ok := normalizeRelativeArtifactPath(a.Path)
		if !ok {
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

	if strings.TrimSpace(fileName) == "" {
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

func normalizeRelativeArtifactPath(raw string) (string, bool) {
	rel := filepath.ToSlash(filepath.Clean(strings.TrimSpace(raw)))
	if rel == "" || rel == "." || strings.HasPrefix(rel, "/") || rel == ".." || strings.HasPrefix(rel, "../") || strings.Contains(rel, "/../") {
		return "", false
	}
	return rel, true
}

func buildArtifactsZIPFileName(jobID, prefix string) string {
	name := strings.TrimSpace(jobID)
	if strings.TrimSpace(prefix) != "" {
		name += "-" + strings.TrimSpace(prefix)
	}
	fileName := sanitizeZIPName(name) + "-artifacts.zip"
	if strings.TrimSpace(fileName) == "-artifacts.zip" {
		fileName = "job-artifacts.zip"
	}
	return fileName
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
			ID:  c.ID,
			Env: c.Env,
		})
	}
	return out
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
		var secrets []protocol.ProjectSecretSpec
		if len(step.VaultSecrets) > 0 {
			secrets = append([]protocol.ProjectSecretSpec(nil), step.VaultSecrets...)
		}
		out = append(out, protocol.JobStepPlanItem{
			Index:           step.Index,
			Total:           step.Total,
			Name:            step.Name,
			Script:          step.Script,
			Kind:            step.Kind,
			Env:             cloneStringMap(step.Env),
			VaultConnection: step.VaultConnection,
			VaultSecrets:    secrets,
			TestName:        step.TestName,
			TestFormat:      step.TestFormat,
			TestReport:      step.TestReport,
			CoverageFormat:  step.CoverageFormat,
			CoverageReport:  step.CoverageReport,
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
