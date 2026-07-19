package jobexecution

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/izzyreal/ciwi/internal/protocol"
)

type stubStore struct {
	listJobExecutionsFn           func() ([]protocol.JobExecution, error)
	createJobExecutionFn          func(req protocol.CreateJobExecutionRequest) (protocol.JobExecution, error)
	getJobExecutionFn             func(id string) (protocol.JobExecution, error)
	deleteQueuedJobExecutionFn    func(id string) error
	updateJobExecutionStatusFn    func(id string, req protocol.JobExecutionStatusUpdateRequest) (protocol.JobExecution, error)
	appendJobExecutionEventsFn    func(id string, events []protocol.JobExecutionEvent) error
	listJobExecutionEventsFn      func(id string) ([]protocol.JobExecutionEvent, error)
	listJobExecutionEventsAfterFn func(id string, afterID int64) ([]protocol.JobExecutionEvent, error)
	listJobExecutionArtifactsFn   func(id string) ([]protocol.JobExecutionArtifact, error)
	saveJobExecutionArtifactsFn   func(id string, artifacts []protocol.JobExecutionArtifact) error
	getJobExecutionTestReportFn   func(id string) (protocol.JobExecutionTestReport, bool, error)
	saveJobExecutionTestReportFn  func(id string, report protocol.JobExecutionTestReport) error
	clearQueuedJobExecutionsFn    func() (int64, error)
	flushJobExecutionHistoryFn    func() ([]string, error)
}

func (s *stubStore) ListJobExecutions() ([]protocol.JobExecution, error) {
	if s.listJobExecutionsFn != nil {
		return s.listJobExecutionsFn()
	}
	return nil, fmt.Errorf("unexpected ListJobExecutions call")
}

func (s *stubStore) CreateJobExecution(req protocol.CreateJobExecutionRequest) (protocol.JobExecution, error) {
	if s.createJobExecutionFn != nil {
		return s.createJobExecutionFn(req)
	}
	return protocol.JobExecution{}, fmt.Errorf("unexpected CreateJobExecution call")
}

func (s *stubStore) GetJobExecution(id string) (protocol.JobExecution, error) {
	if s.getJobExecutionFn != nil {
		return s.getJobExecutionFn(id)
	}
	return protocol.JobExecution{}, fmt.Errorf("unexpected GetJobExecution call")
}

func (s *stubStore) DeleteQueuedJobExecution(id string) error {
	if s.deleteQueuedJobExecutionFn != nil {
		return s.deleteQueuedJobExecutionFn(id)
	}
	return fmt.Errorf("unexpected DeleteQueuedJobExecution call")
}

func (s *stubStore) UpdateJobExecutionStatus(id string, req protocol.JobExecutionStatusUpdateRequest) (protocol.JobExecution, error) {
	if s.updateJobExecutionStatusFn != nil {
		return s.updateJobExecutionStatusFn(id, req)
	}
	return protocol.JobExecution{}, fmt.Errorf("unexpected UpdateJobExecutionStatus call")
}

func (s *stubStore) AppendJobExecutionEvents(id string, events []protocol.JobExecutionEvent) error {
	if s.appendJobExecutionEventsFn != nil {
		return s.appendJobExecutionEventsFn(id, events)
	}
	return fmt.Errorf("unexpected AppendJobExecutionEvents call")
}

func (s *stubStore) ListJobExecutionEvents(id string) ([]protocol.JobExecutionEvent, error) {
	if s.listJobExecutionEventsFn != nil {
		return s.listJobExecutionEventsFn(id)
	}
	return nil, fmt.Errorf("unexpected ListJobExecutionEvents call")
}

func (s *stubStore) ListJobExecutionEventsAfter(id string, afterID int64) ([]protocol.JobExecutionEvent, error) {
	if s.listJobExecutionEventsAfterFn != nil {
		return s.listJobExecutionEventsAfterFn(id, afterID)
	}
	if afterID == 0 {
		return s.ListJobExecutionEvents(id)
	}
	return nil, fmt.Errorf("unexpected ListJobExecutionEventsAfter call")
}

func (s *stubStore) ListJobExecutionArtifacts(id string) ([]protocol.JobExecutionArtifact, error) {
	if s.listJobExecutionArtifactsFn != nil {
		return s.listJobExecutionArtifactsFn(id)
	}
	return nil, fmt.Errorf("unexpected ListJobExecutionArtifacts call")
}

func (s *stubStore) SaveJobExecutionArtifacts(id string, artifacts []protocol.JobExecutionArtifact) error {
	if s.saveJobExecutionArtifactsFn != nil {
		return s.saveJobExecutionArtifactsFn(id, artifacts)
	}
	return fmt.Errorf("unexpected SaveJobExecutionArtifacts call")
}

func (s *stubStore) GetJobExecutionTestReport(id string) (protocol.JobExecutionTestReport, bool, error) {
	if s.getJobExecutionTestReportFn != nil {
		return s.getJobExecutionTestReportFn(id)
	}
	return protocol.JobExecutionTestReport{}, false, fmt.Errorf("unexpected GetJobExecutionTestReport call")
}

func (s *stubStore) SaveJobExecutionTestReport(id string, report protocol.JobExecutionTestReport) error {
	if s.saveJobExecutionTestReportFn != nil {
		return s.saveJobExecutionTestReportFn(id, report)
	}
	return fmt.Errorf("unexpected SaveJobExecutionTestReport call")
}

func (s *stubStore) ClearQueuedJobExecutions() (int64, error) {
	if s.clearQueuedJobExecutionsFn != nil {
		return s.clearQueuedJobExecutionsFn()
	}
	return 0, fmt.Errorf("unexpected ClearQueuedJobExecutions call")
}

func (s *stubStore) FlushJobExecutionHistory() ([]string, error) {
	if s.flushJobExecutionHistoryFn != nil {
		return s.flushJobExecutionHistoryFn()
	}
	return nil, fmt.Errorf("unexpected FlushJobExecutionHistory call")
}

func TestHandleByIDCancelActiveJob(t *testing.T) {
	store := &stubStore{}
	store.getJobExecutionFn = func(id string) (protocol.JobExecution, error) {
		return protocol.JobExecution{
			ID:              id,
			Status:          protocol.JobExecutionStatusRunning,
			LeasedByAgentID: "agent-1",
		}, nil
	}
	store.updateJobExecutionStatusFn = func(id string, req protocol.JobExecutionStatusUpdateRequest) (protocol.JobExecution, error) {
		if req.AgentID != "agent-1" || req.Status != protocol.JobExecutionStatusFailed {
			t.Fatalf("unexpected update request: %+v", req)
		}
		return protocol.JobExecution{
			ID:     id,
			Status: protocol.JobExecutionStatusFailed,
			Error:  req.Error,
		}, nil
	}
	store.appendJobExecutionEventsFn = func(id string, events []protocol.JobExecutionEvent) error {
		if len(events) != 1 || events[0].Type != protocol.JobExecutionEventTypeSystemMessage || !strings.Contains(events[0].Message, "job cancelled by user") {
			t.Fatalf("unexpected cancellation events: %+v", events)
		}
		return nil
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/jobs/job-1/cancel", nil)
	HandleByID(rec, req, HandlerDeps{Store: store, ArtifactsDir: t.TempDir()})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleByIDCancelRejectsNonActive(t *testing.T) {
	store := &stubStore{}
	store.getJobExecutionFn = func(id string) (protocol.JobExecution, error) {
		return protocol.JobExecution{ID: id, Status: protocol.JobExecutionStatusSucceeded}, nil
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/jobs/job-1/cancel", nil)
	HandleByID(rec, req, HandlerDeps{Store: store, ArtifactsDir: t.TempDir()})
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleByIDRerunClonesStartedJob(t *testing.T) {
	store := &stubStore{}
	started := time.Now().UTC()
	store.getJobExecutionFn = func(id string) (protocol.JobExecution, error) {
		return protocol.JobExecution{
			ID:                   id,
			Script:               "echo hi",
			Env:                  map[string]string{"A": "B"},
			RequiredCapabilities: map[string]string{"os": "linux"},
			TimeoutSeconds:       30,
			ArtifactGlobs:        []string{"dist/**"},
			Caches:               []protocol.JobCacheSpec{{ID: "ccache", Env: "CCACHE_DIR"}},
			Source:               &protocol.SourceSpec{Repo: "https://example/repo.git", Ref: "main"},
			Metadata:             map[string]string{"pipeline_id": "build"},
			StepPlan:             []protocol.JobStepPlanItem{{Index: 1, Total: 1, Name: "step"}},
			StartedUTC:           started,
		}, nil
	}
	store.createJobExecutionFn = func(req protocol.CreateJobExecutionRequest) (protocol.JobExecution, error) {
		if req.Script != "echo hi" || req.TimeoutSeconds != 30 {
			t.Fatalf("unexpected clone request: %+v", req)
		}
		if req.Metadata[protocol.JobMetadataAttemptRootJobID] != "job-1" || req.Metadata[protocol.JobMetadataRerunOfJobID] != "job-1" {
			t.Fatalf("expected rerun attempt metadata, got %+v", req.Metadata)
		}
		return protocol.JobExecution{ID: "job-clone", Status: protocol.JobExecutionStatusQueued}, nil
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/jobs/job-1/rerun", nil)
	HandleByID(rec, req, HandlerDeps{Store: store, ArtifactsDir: t.TempDir()})
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleByIDRerunAllowsDependencyBlockedJob(t *testing.T) {
	store := &stubStore{}
	store.getJobExecutionFn = func(id string) (protocol.JobExecution, error) {
		return protocol.JobExecution{
			ID: id, Script: "echo package", Status: protocol.JobExecutionStatusFailed,
			Error:    "cancelled: upstream pipeline build failed",
			Metadata: map[string]string{"pipeline_id": "package"},
		}, nil
	}
	store.createJobExecutionFn = func(req protocol.CreateJobExecutionRequest) (protocol.JobExecution, error) {
		return protocol.JobExecution{ID: "job-clone", Status: protocol.JobExecutionStatusQueued}, nil
	}
	prepared := false
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/jobs/job-1/rerun", nil)
	HandleByID(rec, req, HandlerDeps{
		Store: store,
		PrepareRerun: func(original protocol.JobExecution, request *protocol.CreateJobExecutionRequest) error {
			prepared = true
			return nil
		},
	})
	if rec.Code != http.StatusCreated || !prepared {
		t.Fatalf("expected blocked job rerun to be prepared and created, status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandleByIDStatusUpdatesAndCallbacks(t *testing.T) {
	store := &stubStore{}
	var appendCalled bool
	var seenCalled bool
	var updatedCalled bool
	store.updateJobExecutionStatusFn = func(id string, req protocol.JobExecutionStatusUpdateRequest) (protocol.JobExecution, error) {
		return protocol.JobExecution{
			ID:              id,
			Status:          protocol.JobExecutionStatusRunning,
			CurrentStep:     "step 1",
			LeasedByAgentID: req.AgentID,
		}, nil
	}
	store.appendJobExecutionEventsFn = func(id string, events []protocol.JobExecutionEvent) error {
		appendCalled = true
		return nil
	}

	body := `{"agent_id":"agent-1","status":"running","events":[{"type":"step.started"}]}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/jobs/job-1/status", strings.NewReader(body))
	HandleByID(rec, req, HandlerDeps{
		Store:        store,
		ArtifactsDir: t.TempDir(),
		MarkAgentSeen: func(agentID string, ts time.Time) {
			seenCalled = true
		},
		OnJobUpdated: func(job protocol.JobExecution) {
			updatedCalled = true
		},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !appendCalled || !seenCalled || !updatedCalled {
		t.Fatalf("expected callbacks called append=%v seen=%v updated=%v", appendCalled, seenCalled, updatedCalled)
	}
}

func TestHandleByIDLogDownloadCleanAndRaw(t *testing.T) {
	store := &stubStore{}
	exitCode := 2
	started := time.Date(2026, 7, 18, 10, 0, 0, 0, time.UTC)
	finished := started.Add(2 * time.Second)
	store.getJobExecutionFn = func(id string) (protocol.JobExecution, error) {
		return protocol.JobExecution{
			ID:          id,
			Status:      protocol.JobExecutionStatusFailed,
			StepPlan:    []protocol.JobStepPlanItem{{Index: 1, Total: 1, Name: "build", YAMLLiteral: "run: go test {{pkg}}", Script: "go test ./..."}},
			StartedUTC:  started,
			FinishedUTC: finished,
			ExitCode:    &exitCode,
			Error:       "failed",
		}, nil
	}
	store.listJobExecutionEventsFn = func(id string) ([]protocol.JobExecutionEvent, error) {
		workspace := protocol.JobExecutionPhase{ID: protocol.JobExecutionPhaseWorkspace, Name: "Prepare workspace", Index: 1, Total: 3}
		return []protocol.JobExecutionEvent{
			{
				Type:         protocol.JobExecutionEventTypeSystemMessage,
				TimestampUTC: started.Add(-time.Second),
				Message:      "[meta] agent=agent-1\n",
			},
			{Type: protocol.JobExecutionEventTypePhaseStarted, TimestampUTC: started, Phase: &workspace},
			{Type: protocol.JobExecutionEventTypePhaseOutput, TimestampUTC: started, Phase: &workspace, Output: "workspace ready\n"},
			{Type: protocol.JobExecutionEventTypePhaseFinished, TimestampUTC: started, Phase: &workspace, DurationMS: 100},
			{
				Type:         protocol.JobExecutionEventTypeStepStarted,
				TimestampUTC: started,
				Step:         &protocol.JobStepPlanItem{Index: 1, Total: 1, Name: "build", YAMLLiteral: "run: go test {{pkg}}", Script: "go test ./..."},
			},
			{
				Type:         protocol.JobExecutionEventTypeStepOutput,
				TimestampUTC: started.Add(time.Second),
				Step:         &protocol.JobStepPlanItem{Index: 1, Total: 1, Name: "build", YAMLLiteral: "run: go test {{pkg}}", Script: "go test ./..."},
				Output:       "\x1b[31mFAIL\x1b[0m\r\n",
			},
			{
				Type:         protocol.JobExecutionEventTypeStepFinished,
				TimestampUTC: finished,
				Step:         &protocol.JobStepPlanItem{Index: 1, Total: 1, Name: "build", YAMLLiteral: "run: go test {{pkg}}", Script: "go test ./..."},
				Error:        "exit=2",
				ExitCode:     &exitCode,
				DurationMS:   2000,
			},
		}, nil
	}

	recClean := httptest.NewRecorder()
	reqClean := httptest.NewRequest(http.MethodGet, "/api/v1/jobs/job-1/log?format=clean", nil)
	HandleByID(recClean, reqClean, HandlerDeps{Store: store, ArtifactsDir: t.TempDir()})
	if recClean.Code != http.StatusOK {
		t.Fatalf("expected clean 200, got %d: %s", recClean.Code, recClean.Body.String())
	}
	clean := recClean.Body.String()
	if !strings.Contains(clean, "[meta] agent=agent-1") || !strings.Contains(clean, "Step 1/3: Prepare workspace") || !strings.Contains(clean, "workspace ready") || !strings.Contains(clean, "Step 3/3: build") || !strings.Contains(clean, "run: go test {{pkg}}") || !strings.Contains(clean, "go test ./...") || !strings.Contains(clean, "FAIL") || strings.Contains(clean, "\x1b[31m") {
		t.Fatalf("unexpected clean log:\n%s", clean)
	}
	if got := recClean.Header().Get("Content-Disposition"); !strings.Contains(got, "ciwi-job-1-clean.log") {
		t.Fatalf("unexpected content disposition %q", got)
	}

	recRaw := httptest.NewRecorder()
	reqRaw := httptest.NewRequest(http.MethodGet, "/api/v1/jobs/job-1/log?format=raw", nil)
	HandleByID(recRaw, reqRaw, HandlerDeps{Store: store, ArtifactsDir: t.TempDir()})
	if recRaw.Code != http.StatusOK {
		t.Fatalf("expected raw 200, got %d: %s", recRaw.Code, recRaw.Body.String())
	}
	if raw := recRaw.Body.String(); !strings.Contains(raw, "[meta] agent=agent-1") || !strings.Contains(raw, "workspace ready") || !strings.Contains(raw, "\x1b[31mFAIL\x1b[0m") {
		t.Fatalf("expected raw log to preserve ANSI output, got:\n%s", raw)
	}
}

func TestHandleByIDArtifactsGetAndRejectPost(t *testing.T) {
	artifactsDir := t.TempDir()
	store := &stubStore{}
	store.listJobExecutionArtifactsFn = func(id string) ([]protocol.JobExecutionArtifact, error) {
		return []protocol.JobExecutionArtifact{{JobExecutionID: id, Path: "dist/app.bin", URL: "job-1/dist/app.bin", SizeBytes: 5}}, nil
	}

	recGet := httptest.NewRecorder()
	reqGet := httptest.NewRequest(http.MethodGet, "/api/v1/jobs/job-1/artifacts", nil)
	HandleByID(recGet, reqGet, HandlerDeps{Store: store, ArtifactsDir: artifactsDir})
	if recGet.Code != http.StatusOK {
		t.Fatalf("expected GET 200, got %d: %s", recGet.Code, recGet.Body.String())
	}

	recPost := httptest.NewRecorder()
	reqPost := httptest.NewRequest(http.MethodPost, "/api/v1/jobs/job-1/artifacts", strings.NewReader(`{}`))
	HandleByID(recPost, reqPost, HandlerDeps{Store: store, ArtifactsDir: artifactsDir})
	if recPost.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected POST 405, got %d: %s", recPost.Code, recPost.Body.String())
	}
}

func TestHandleByIDArtifactsUploadZIP(t *testing.T) {
	artifactsDir := t.TempDir()
	store := &stubStore{}
	var saveCalled bool
	store.getJobExecutionFn = func(id string) (protocol.JobExecution, error) {
		return protocol.JobExecution{ID: id, LeasedByAgentID: "agent-1"}, nil
	}
	store.saveJobExecutionArtifactsFn = func(id string, artifacts []protocol.JobExecutionArtifact) error {
		saveCalled = true
		if len(artifacts) != 1 || artifacts[0].Path != "dist/app.bin" {
			t.Fatalf("unexpected persisted artifacts: %+v", artifacts)
		}
		return nil
	}

	var payload bytes.Buffer
	zw := zip.NewWriter(&payload)
	w, err := zw.Create("dist/app.bin")
	if err != nil {
		t.Fatalf("create zip entry: %v", err)
	}
	if _, err := io.WriteString(w, "hello"); err != nil {
		t.Fatalf("write zip entry: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip writer: %v", err)
	}

	recPost := httptest.NewRecorder()
	reqPost := httptest.NewRequest(http.MethodPost, "/api/v1/jobs/job-1/artifacts/upload-zip", bytes.NewReader(payload.Bytes()))
	reqPost.Header.Set("Content-Type", "application/zip")
	reqPost.Header.Set("X-CIWI-Agent-ID", "agent-1")
	HandleByID(recPost, reqPost, HandlerDeps{Store: store, ArtifactsDir: artifactsDir})
	if recPost.Code != http.StatusOK {
		t.Fatalf("expected POST 200, got %d: %s", recPost.Code, recPost.Body.String())
	}
	if !saveCalled {
		t.Fatalf("expected SaveJobExecutionArtifacts to be called")
	}
}

func TestHandleByIDTestsGetAndPost(t *testing.T) {
	artifactsDir := t.TempDir()
	store := &stubStore{}
	var saveReportCalled bool
	report := protocol.JobExecutionTestReport{
		Total:  1,
		Passed: 1,
	}
	store.getJobExecutionTestReportFn = func(id string) (protocol.JobExecutionTestReport, bool, error) {
		return report, true, nil
	}
	store.getJobExecutionFn = func(id string) (protocol.JobExecution, error) {
		return protocol.JobExecution{ID: id, LeasedByAgentID: "agent-1"}, nil
	}
	store.saveJobExecutionTestReportFn = func(id string, r protocol.JobExecutionTestReport) error {
		saveReportCalled = true
		return nil
	}

	recGet := httptest.NewRecorder()
	reqGet := httptest.NewRequest(http.MethodGet, "/api/v1/jobs/job-1/tests", nil)
	HandleByID(recGet, reqGet, HandlerDeps{Store: store, ArtifactsDir: artifactsDir})
	if recGet.Code != http.StatusOK {
		t.Fatalf("expected GET 200, got %d: %s", recGet.Code, recGet.Body.String())
	}

	postReqBody, _ := json.Marshal(protocol.UploadTestReportRequest{
		AgentID: "agent-1",
		Report:  report,
	})
	recPost := httptest.NewRecorder()
	reqPost := httptest.NewRequest(http.MethodPost, "/api/v1/jobs/job-1/tests", strings.NewReader(string(postReqBody)))
	HandleByID(recPost, reqPost, HandlerDeps{Store: store, ArtifactsDir: artifactsDir})
	if recPost.Code != http.StatusOK {
		t.Fatalf("expected POST 200, got %d: %s", recPost.Code, recPost.Body.String())
	}
	if !saveReportCalled {
		t.Fatalf("expected SaveJobExecutionTestReport to be called")
	}
}
