package server

import (
	"net/http"
	"strings"
	"testing"
)

func TestJobExecutionsViewSummaryAndPagedLists(t *testing.T) {
	ts := newTestHTTPServer(t)
	defer ts.Close()

	client := ts.Client()
	createJob := func(script string) string {
		t.Helper()
		resp := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/jobs", map[string]any{
			"script":          script,
			"timeout_seconds": 30,
		})
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("create job status=%d body=%s", resp.StatusCode, readBody(t, resp))
		}
		var payload struct {
			Job struct {
				ID string `json:"id"`
			} `json:"job_execution"`
		}
		decodeJSONBody(t, resp, &payload)
		if strings.TrimSpace(payload.Job.ID) == "" {
			t.Fatalf("missing created job id")
		}
		return payload.Job.ID
	}

	queuedID := createJob("echo queued")
	runningID := createJob("echo running")
	succeededID := createJob("echo succeeded")

	runningResp := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/jobs/"+runningID+"/status", map[string]any{
		"agent_id": "agent-a",
		"status":   "running",
		"output":   "in progress",
	})
	if runningResp.StatusCode != http.StatusOK {
		t.Fatalf("mark running status=%d body=%s", runningResp.StatusCode, readBody(t, runningResp))
	}
	_ = readBody(t, runningResp)

	succeededResp := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/jobs/"+succeededID+"/status", map[string]any{
		"agent_id": "agent-a",
		"status":   "succeeded",
		"output":   "done",
	})
	if succeededResp.StatusCode != http.StatusOK {
		t.Fatalf("mark succeeded status=%d body=%s", succeededResp.StatusCode, readBody(t, succeededResp))
	}
	_ = readBody(t, succeededResp)

	summaryResp := mustJSONRequest(t, client, http.MethodGet, ts.URL+"/api/v1/jobs?view=summary&max=100", nil)
	if summaryResp.StatusCode != http.StatusOK {
		t.Fatalf("summary status=%d body=%s", summaryResp.StatusCode, readBody(t, summaryResp))
	}
	if cacheControl := strings.ToLower(summaryResp.Header.Get("Cache-Control")); !strings.Contains(cacheControl, "no-store") {
		t.Fatalf("expected summary response to disable caching, got cache-control=%q", summaryResp.Header.Get("Cache-Control"))
	}
	var summary struct {
		Total             int `json:"total"`
		QueuedCount       int `json:"queued_count"`
		HistoryCount      int `json:"history_count"`
		QueuedGroupCount  int `json:"queued_group_count"`
		HistoryGroupCount int `json:"history_group_count"`
		QueuedGroups      []struct {
			Key         string `json:"key"`
			RunID       string `json:"run_id"`
			JobCount    int    `json:"job_count"`
			Collapsible bool   `json:"collapsible"`
		} `json:"queued_groups"`
		HistoryGroups []struct {
			Key         string `json:"key"`
			RunID       string `json:"run_id"`
			JobCount    int    `json:"job_count"`
			Collapsible bool   `json:"collapsible"`
		} `json:"history_groups"`
	}
	decodeJSONBody(t, summaryResp, &summary)
	if summary.Total != 3 {
		t.Fatalf("expected total=3, got %d", summary.Total)
	}
	if summary.QueuedCount != 2 {
		t.Fatalf("expected queued_count=2, got %d", summary.QueuedCount)
	}
	if summary.HistoryCount != 1 {
		t.Fatalf("expected history_count=1, got %d", summary.HistoryCount)
	}
	if summary.QueuedGroupCount != 2 {
		t.Fatalf("expected queued_group_count=2, got %d", summary.QueuedGroupCount)
	}
	if summary.HistoryGroupCount != 1 {
		t.Fatalf("expected history_group_count=1, got %d", summary.HistoryGroupCount)
	}
	if len(summary.QueuedGroups) != summary.QueuedGroupCount {
		t.Fatalf("expected queued_groups len=%d, got %d", summary.QueuedGroupCount, len(summary.QueuedGroups))
	}
	if len(summary.HistoryGroups) != summary.HistoryGroupCount {
		t.Fatalf("expected history_groups len=%d, got %d", summary.HistoryGroupCount, len(summary.HistoryGroups))
	}

	queuedResp := mustJSONRequest(t, client, http.MethodGet, ts.URL+"/api/v1/jobs?view=queued&max=100&offset=0&limit=1", nil)
	if queuedResp.StatusCode != http.StatusOK {
		t.Fatalf("queued view status=%d body=%s", queuedResp.StatusCode, readBody(t, queuedResp))
	}
	var queued struct {
		Total int `json:"total"`
		Jobs  []struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"job_executions"`
	}
	decodeJSONBody(t, queuedResp, &queued)
	if queued.Total != 2 {
		t.Fatalf("expected queued total=2, got %d", queued.Total)
	}
	if len(queued.Jobs) != 1 {
		t.Fatalf("expected 1 queued job in paged response, got %d", len(queued.Jobs))
	}
	status := strings.ToLower(strings.TrimSpace(queued.Jobs[0].Status))
	if status != "queued" && status != "running" {
		t.Fatalf("expected queued/running status, got %q", queued.Jobs[0].Status)
	}
	if queued.Jobs[0].ID != queuedID && queued.Jobs[0].ID != runningID {
		t.Fatalf("unexpected queued id %q", queued.Jobs[0].ID)
	}

	historyResp := mustJSONRequest(t, client, http.MethodGet, ts.URL+"/api/v1/jobs?view=history&max=100&offset=0&limit=10", nil)
	if historyResp.StatusCode != http.StatusOK {
		t.Fatalf("history view status=%d body=%s", historyResp.StatusCode, readBody(t, historyResp))
	}
	var history struct {
		Total int `json:"total"`
		Jobs  []struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"job_executions"`
	}
	decodeJSONBody(t, historyResp, &history)
	if history.Total != 1 {
		t.Fatalf("expected history total=1, got %d", history.Total)
	}
	if len(history.Jobs) != 1 {
		t.Fatalf("expected 1 history row, got %d", len(history.Jobs))
	}
	if history.Jobs[0].ID != succeededID {
		t.Fatalf("expected history job %q, got %q", succeededID, history.Jobs[0].ID)
	}
	if strings.ToLower(strings.TrimSpace(history.Jobs[0].Status)) != "succeeded" {
		t.Fatalf("expected history status succeeded, got %q", history.Jobs[0].Status)
	}
}

func TestJobExecutionsViewSummaryIncludesGroupedCounts(t *testing.T) {
	ts := newTestHTTPServer(t)
	defer ts.Close()

	client := ts.Client()
	createJob := func(script, runID string) string {
		t.Helper()
		meta := map[string]any{}
		if strings.TrimSpace(runID) != "" {
			meta["pipeline_run_id"] = runID
			meta["pipeline_id"] = "build"
		}
		resp := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/jobs", map[string]any{
			"script":          script,
			"timeout_seconds": 30,
			"metadata":        meta,
		})
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("create job status=%d body=%s", resp.StatusCode, readBody(t, resp))
		}
		var payload struct {
			Job struct {
				ID string `json:"id"`
			} `json:"job_execution"`
		}
		decodeJSONBody(t, resp, &payload)
		return payload.Job.ID
	}

	queuedRunA1 := createJob("echo queued-a1", "run-a")
	_ = createJob("echo queued-a2", "run-a")
	_ = createJob("echo queued-single", "")
	historyRunB := createJob("echo history-b", "run-b")

	doneResp := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/jobs/"+historyRunB+"/status", map[string]any{
		"agent_id": "agent-a",
		"status":   "succeeded",
		"output":   "done",
	})
	if doneResp.StatusCode != http.StatusOK {
		t.Fatalf("mark succeeded status=%d body=%s", doneResp.StatusCode, readBody(t, doneResp))
	}
	_ = readBody(t, doneResp)

	keepQueuedResp := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/jobs/"+queuedRunA1+"/status", map[string]any{
		"agent_id": "agent-a",
		"status":   "running",
		"output":   "in progress",
	})
	if keepQueuedResp.StatusCode != http.StatusOK {
		t.Fatalf("mark running status=%d body=%s", keepQueuedResp.StatusCode, readBody(t, keepQueuedResp))
	}
	_ = readBody(t, keepQueuedResp)

	summaryResp := mustJSONRequest(t, client, http.MethodGet, ts.URL+"/api/v1/jobs?view=summary&max=100", nil)
	if summaryResp.StatusCode != http.StatusOK {
		t.Fatalf("summary status=%d body=%s", summaryResp.StatusCode, readBody(t, summaryResp))
	}
	var summary struct {
		QueuedCount       int `json:"queued_count"`
		HistoryCount      int `json:"history_count"`
		QueuedGroupCount  int `json:"queued_group_count"`
		HistoryGroupCount int `json:"history_group_count"`
		QueuedGroups      []struct {
			Key         string `json:"key"`
			RunID       string `json:"run_id"`
			JobCount    int    `json:"job_count"`
			Collapsible bool   `json:"collapsible"`
		} `json:"queued_groups"`
		HistoryGroups []struct {
			Key         string `json:"key"`
			RunID       string `json:"run_id"`
			JobCount    int    `json:"job_count"`
			Collapsible bool   `json:"collapsible"`
		} `json:"history_groups"`
	}
	decodeJSONBody(t, summaryResp, &summary)
	if summary.QueuedCount != 3 {
		t.Fatalf("expected queued_count=3, got %d", summary.QueuedCount)
	}
	if summary.HistoryCount != 1 {
		t.Fatalf("expected history_count=1, got %d", summary.HistoryCount)
	}
	if summary.QueuedGroupCount != 2 {
		t.Fatalf("expected queued_group_count=2 (run-a + single), got %d", summary.QueuedGroupCount)
	}
	if summary.HistoryGroupCount != 1 {
		t.Fatalf("expected history_group_count=1, got %d", summary.HistoryGroupCount)
	}
	if len(summary.QueuedGroups) != 2 {
		t.Fatalf("expected 2 queued_groups entries, got %d", len(summary.QueuedGroups))
	}
	foundRunA := false
	foundSingle := false
	for _, g := range summary.QueuedGroups {
		if g.RunID == "run-a" {
			foundRunA = true
			if !g.Collapsible || g.JobCount != 2 {
				t.Fatalf("unexpected run-a queued group: %+v", g)
			}
			continue
		}
		if g.RunID == "" {
			foundSingle = true
			if g.Collapsible || g.JobCount != 1 {
				t.Fatalf("unexpected single queued group: %+v", g)
			}
			continue
		}
		t.Fatalf("unexpected queued group entry: %+v", g)
	}
	if !foundRunA || !foundSingle {
		t.Fatalf("missing expected queued groups, got %+v", summary.QueuedGroups)
	}
	if len(summary.HistoryGroups) != 1 || summary.HistoryGroups[0].Collapsible || summary.HistoryGroups[0].JobCount != 1 {
		t.Fatalf("unexpected history groups: %+v", summary.HistoryGroups)
	}
}
