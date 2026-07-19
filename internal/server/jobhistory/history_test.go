package jobhistory

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/izzyreal/ciwi/internal/protocol"
)

type stubStore struct {
	listJobExecutionsFn func() ([]protocol.JobExecution, error)
}

func (s *stubStore) ListJobExecutions() ([]protocol.JobExecution, error) {
	return s.listJobExecutionsFn()
}

func TestHandleLayoutGroupsChainsAndPipelines(t *testing.T) {
	store := &stubStore{listJobExecutionsFn: func() ([]protocol.JobExecution, error) {
		return []protocol.JobExecution{
			job("release-1", "succeeded", "2026-03-29T10:25:36Z", map[string]string{
				"project": "ciwi", "project_id": "1", "pipeline_id": "release", "pipeline_run_id": "run-release", "chain_run_id": "chain-1", "pipeline_chain_id": "build-release",
			}),
			job("build-2", "succeeded", "2026-03-29T10:25:35Z", map[string]string{
				"project": "ciwi", "project_id": "1", "pipeline_id": "build", "pipeline_run_id": "run-build", "chain_run_id": "chain-1", "pipeline_chain_id": "build-release", "pipeline_job_id": "compile", "matrix_name": "linux-amd64", "matrix_index": "0",
			}),
			job("build-1", "succeeded", "2026-03-29T10:25:34Z", map[string]string{
				"project": "ciwi", "project_id": "1", "pipeline_id": "build", "pipeline_run_id": "run-build", "chain_run_id": "chain-1", "pipeline_chain_id": "build-release", "pipeline_job_id": "compile", "matrix_name": "windows-amd64", "matrix_index": "1",
			}),
			job("pipeline-standalone", "failed", "2026-03-29T10:20:00Z", map[string]string{
				"project": "ciwi", "project_id": "1", "pipeline_id": "lint", "pipeline_run_id": "run-lint", "pipeline_job_id": "lint",
			}),
		}, nil
	}}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/job-history/layout?offset=0&limit=10", nil)
	HandleLayout(rec, req, HandlerDeps{Store: store})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var got LayoutResponse
	mustDecode(t, rec, &got)
	if got.TotalCards != 2 || len(got.Cards) != 2 {
		t.Fatalf("unexpected card count: %+v", got)
	}
	if got.Cards[0].Kind != "chain" || got.Cards[1].Kind != "pipeline" {
		t.Fatalf("unexpected layout order: %+v", got.Cards)
	}
}

func TestHandleCardsFullBuildsSectionsAndMatrixGroups(t *testing.T) {
	store := &stubStore{listJobExecutionsFn: func() ([]protocol.JobExecution, error) {
		return []protocol.JobExecution{
			job("release-1", "succeeded", "2026-03-29T10:25:36Z", map[string]string{
				"project": "ciwi", "project_id": "1", "pipeline_id": "release", "pipeline_run_id": "run-release", "chain_run_id": "chain-1", "pipeline_chain_id": "build-release", "pipeline_job_id": "publish",
			}),
			job("build-2", "succeeded", "2026-03-29T10:25:35Z", map[string]string{
				"project": "ciwi", "project_id": "1", "pipeline_id": "build", "pipeline_run_id": "run-build", "chain_run_id": "chain-1", "pipeline_chain_id": "build-release", "pipeline_job_id": "compile", "matrix_name": "linux-amd64", "matrix_index": "0",
			}),
			job("build-1", "failed", "2026-03-29T10:25:34Z", map[string]string{
				"project": "ciwi", "project_id": "1", "pipeline_id": "build", "pipeline_run_id": "run-build", "chain_run_id": "chain-1", "pipeline_chain_id": "build-release", "pipeline_job_id": "compile", "matrix_name": "windows-amd64", "matrix_index": "1",
			}),
			job("build-0", "succeeded", "2026-03-29T10:25:33Z", map[string]string{
				"project": "ciwi", "project_id": "1", "pipeline_id": "build", "pipeline_run_id": "run-build", "chain_run_id": "chain-1", "pipeline_chain_id": "build-release", "pipeline_job_id": "unit-tests",
			}),
		}, nil
	}}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/job-history/cards?offset=0&limit=10&detail=full", nil)
	HandleCards(rec, req, HandlerDeps{Store: store})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var got CardsResponse
	mustDecode(t, rec, &got)
	if len(got.Cards) != 1 {
		t.Fatalf("expected 1 card, got %+v", got)
	}
	card := got.Cards[0]
	if card.Kind != "chain" || card.Summary.TotalJobs != 4 || card.Summary.Failed != 1 {
		t.Fatalf("unexpected card summary: %+v", card)
	}
	if len(card.Sections) != 2 {
		t.Fatalf("expected 2 pipeline sections, got %+v", card.Sections)
	}
	buildSection := card.Sections[1]
	if buildSection.Label != "build" {
		t.Fatalf("unexpected build section: %+v", buildSection)
	}
	if len(buildSection.Items) != 2 {
		t.Fatalf("expected job + matrix section, got %+v", buildSection.Items)
	}
	if buildSection.Items[0].Kind != "matrix" || len(buildSection.Items[0].Items) != 2 {
		t.Fatalf("expected leading matrix section with two jobs, got %+v", buildSection.Items)
	}
	if buildSection.Items[1].Kind != "job" || buildSection.Items[1].Job == nil || buildSection.Items[1].Job.ID != "build-0" {
		t.Fatalf("expected trailing plain job, got %+v", buildSection.Items[1])
	}
}

func TestHandleCardsPaginationAndInvalidDetail(t *testing.T) {
	store := &stubStore{listJobExecutionsFn: func() ([]protocol.JobExecution, error) {
		return []protocol.JobExecution{
			job("a", "succeeded", "2026-03-29T10:25:36Z", map[string]string{"pipeline_id": "a", "pipeline_run_id": "run-a"}),
			job("b", "succeeded", "2026-03-29T10:25:35Z", map[string]string{"pipeline_id": "b", "pipeline_run_id": "run-b"}),
			job("c", "succeeded", "2026-03-29T10:25:34Z", map[string]string{"pipeline_id": "c", "pipeline_run_id": "run-c"}),
		}, nil
	}}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/job-history/cards?offset=1&limit=1&detail=summary", nil)
	HandleCards(rec, req, HandlerDeps{Store: store})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var got CardsResponse
	mustDecode(t, rec, &got)
	if got.Offset != 1 || got.Limit != 1 || got.TotalCards != 3 || len(got.Cards) != 1 {
		t.Fatalf("unexpected pagination: %+v", got)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/v1/job-history/cards?detail=nope", nil)
	HandleCards(rec, req, HandlerDeps{Store: store})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid detail, got %d", rec.Code)
	}
}

func TestHandleQueueCardsSummarizeAllJobsButShowOnlyActiveRows(t *testing.T) {
	store := &stubStore{listJobExecutionsFn: func() ([]protocol.JobExecution, error) {
		return []protocol.JobExecution{
			job("run-q-5", "queued", "2026-03-29T10:25:36Z", map[string]string{
				"project": "ciwi", "project_id": "1", "pipeline_id": "build", "pipeline_run_id": "run-q", "pipeline_job_id": "publish",
			}),
			job("run-q-4", "running", "2026-03-29T10:25:35Z", map[string]string{
				"project": "ciwi", "project_id": "1", "pipeline_id": "build", "pipeline_run_id": "run-q", "pipeline_job_id": "package",
			}),
			job("run-q-3", "succeeded", "2026-03-29T10:25:34Z", map[string]string{
				"project": "ciwi", "project_id": "1", "pipeline_id": "build", "pipeline_run_id": "run-q", "pipeline_job_id": "test-2",
			}),
			job("run-q-2", "succeeded", "2026-03-29T10:25:33Z", map[string]string{
				"project": "ciwi", "project_id": "1", "pipeline_id": "build", "pipeline_run_id": "run-q", "pipeline_job_id": "test-1",
			}),
			job("run-q-1", "succeeded", "2026-03-29T10:25:32Z", map[string]string{
				"project": "ciwi", "project_id": "1", "pipeline_id": "build", "pipeline_run_id": "run-q", "pipeline_job_id": "prepare",
			}),
			job("old-finished", "succeeded", "2026-03-29T10:20:00Z", map[string]string{
				"project": "ciwi", "project_id": "1", "pipeline_id": "lint", "pipeline_run_id": "run-old", "pipeline_job_id": "lint",
			}),
		}, nil
	}}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/job-queue/cards?detail=full", nil)
	attachCalls := 0
	HandleQueueCards(rec, req, HandlerDeps{Store: store, AttachProgress: func(jobs []protocol.JobExecution) {
		attachCalls++
		for i := range jobs {
			if protocol.IsActiveJobExecutionStatus(jobs[i].Status) {
				jobs[i].ExpectedDurationMS = 1234
			}
		}
	}})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var got CardsResponse
	mustDecode(t, rec, &got)
	if got.TotalCards != 1 || len(got.Cards) != 1 {
		t.Fatalf("unexpected queued card count: %+v", got)
	}
	card := got.Cards[0]
	if card.Summary.TotalJobs != 5 || card.Summary.Succeeded != 3 || card.Summary.InProgress != 2 {
		t.Fatalf("unexpected queued summary: %+v", card.Summary)
	}
	if len(card.Sections) != 1 || len(card.Sections[0].Items) != 2 {
		t.Fatalf("expected only the two active rows visible, got %+v", card.Sections)
	}
	if attachCalls != 1 || len(card.ProgressJobs) != 5 || len(card.Sections[0].ProgressJobs) != 5 {
		t.Fatalf("expected one enrichment and all five progress children, got calls=%d card=%d section=%d", attachCalls, len(card.ProgressJobs), len(card.Sections[0].ProgressJobs))
	}
	if card.ProgressJobs[0].ExpectedDurationMS != 1234 {
		t.Fatalf("expected active progress estimate in lightweight child view, got %+v", card.ProgressJobs[0])
	}
}

func TestHandleQueueCardsClassifiesDependencyBlockedJobsAsWaiting(t *testing.T) {
	store := &stubStore{listJobExecutionsFn: func() ([]protocol.JobExecution, error) {
		return []protocol.JobExecution{
			job("release", "queued", "2026-03-29T10:25:36Z", map[string]string{
				"project": "ciwi", "pipeline_id": "release", "pipeline_run_id": "run-release",
				"chain_run_id": "chain-1", "chain_blocked": "1", "chain_depends_on_pipelines": "build",
			}),
			job("build", "failed", "2026-03-29T10:25:35Z", map[string]string{
				"project": "ciwi", "pipeline_id": "build", "pipeline_run_id": "run-build", "chain_run_id": "chain-1",
			}),
		}, nil
	}}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/job-queue/cards?detail=full", nil)
	HandleQueueCards(rec, req, HandlerDeps{Store: store})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var got CardsResponse
	mustDecode(t, rec, &got)
	if len(got.Cards) != 1 {
		t.Fatalf("expected one waiting chain card, got %+v", got.Cards)
	}
	card := got.Cards[0]
	if card.Summary.Waiting != 1 || card.Summary.InProgress != 0 {
		t.Fatalf("expected waiting job to be excluded from in-progress count, got %+v", card.Summary)
	}
	if len(card.ProgressJobs) != 2 || !card.ProgressJobs[0].Waiting {
		t.Fatalf("expected lightweight waiting marker, got %+v", card.ProgressJobs)
	}
}

func TestSummarizeCardCountsOnlyLatestJobAttempt(t *testing.T) {
	jobs := []protocol.JobExecution{
		job("job-retry", "succeeded", "2026-03-29T10:25:36Z", map[string]string{protocol.JobMetadataAttemptRootJobID: "job-original"}),
		job("job-original", "failed", "2026-03-29T10:25:35Z", nil),
	}
	got := summarizeCard(jobs, executionCard{Indices: []int{0, 1}})
	if got.TotalJobs != 1 || got.Succeeded != 1 || got.Failed != 0 {
		t.Fatalf("expected latest retry to replace failed attempt in summary, got %+v", got)
	}
}

func TestHandleCardsShowsActiveChainInHistoryWithFinishedRowsOnly(t *testing.T) {
	store := &stubStore{listJobExecutionsFn: func() ([]protocol.JobExecution, error) {
		return []protocol.JobExecution{
			job("active-2", "queued", "2026-03-29T10:25:36Z", map[string]string{
				"project": "ciwi", "project_id": "1", "pipeline_id": "build", "pipeline_run_id": "run-active", "chain_run_id": "chain-active", "pipeline_chain_id": "build-release", "pipeline_job_id": "publish",
			}),
			job("active-1", "succeeded", "2026-03-29T10:25:35Z", map[string]string{
				"project": "ciwi", "project_id": "1", "pipeline_id": "build", "pipeline_run_id": "run-active", "chain_run_id": "chain-active", "pipeline_chain_id": "build-release", "pipeline_job_id": "unit-tests",
			}),
			job("done-2", "succeeded", "2026-03-29T10:25:34Z", map[string]string{
				"project": "ciwi", "project_id": "1", "pipeline_id": "release", "pipeline_run_id": "run-done", "chain_run_id": "chain-done", "pipeline_chain_id": "build-release", "pipeline_job_id": "publish",
			}),
			job("done-1", "succeeded", "2026-03-29T10:25:33Z", map[string]string{
				"project": "ciwi", "project_id": "1", "pipeline_id": "build", "pipeline_run_id": "run-done-build", "chain_run_id": "chain-done", "pipeline_chain_id": "build-release", "pipeline_job_id": "unit-tests",
			}),
		}, nil
	}}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/job-history/cards?detail=summary", nil)
	HandleCards(rec, req, HandlerDeps{Store: store})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var got CardsResponse
	mustDecode(t, rec, &got)
	if got.TotalCards != 2 || len(got.Cards) != 2 {
		t.Fatalf("expected active chain card plus completed chain card, got %+v", got)
	}
	if got.Cards[0].Key != "chain:chain-active" || got.Cards[0].Summary.TotalJobs != 2 || got.Cards[0].Summary.Succeeded != 1 || got.Cards[0].Summary.InProgress != 1 {
		t.Fatalf("unexpected active chain history card: %+v", got.Cards[0])
	}
	if got.Cards[1].Key != "chain:chain-done" || got.Cards[1].Summary.TotalJobs != 2 {
		t.Fatalf("unexpected completed history chain card: %+v", got.Cards[1])
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/v1/job-history/cards?detail=full", nil)
	HandleCards(rec, req, HandlerDeps{Store: store})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	mustDecode(t, rec, &got)
	if len(got.Cards[0].Sections) != 1 || len(got.Cards[0].Sections[0].Items) != 1 {
		t.Fatalf("expected only finished rows visible for active chain card, got %+v", got.Cards[0].Sections)
	}
}

func job(id, status, created string, meta map[string]string) protocol.JobExecution {
	ts, _ := time.Parse(time.RFC3339, created)
	return protocol.JobExecution{
		ID:         id,
		Status:     status,
		CreatedUTC: ts.UTC(),
		Metadata:   meta,
	}
}

func mustDecode(t *testing.T, rec *httptest.ResponseRecorder, out any) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), out); err != nil {
		t.Fatalf("decode json: %v body=%s", err, rec.Body.String())
	}
}
