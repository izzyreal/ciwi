package server

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/izzyreal/ciwi/internal/protocol"
	"github.com/izzyreal/ciwi/internal/version"
)

func TestHeartbeatDoesNotRequestAgentUpdate(t *testing.T) {
	oldVersion := version.Version
	version.Version = "v1.2.0"
	t.Cleanup(func() { version.Version = oldVersion })

	ts := newTestHTTPServer(t)
	defer ts.Close()

	hbResp := mustJSONRequest(t, ts.Client(), http.MethodPost, ts.URL+"/api/v1/heartbeat", map[string]any{
		"agent_id":      "agent-a",
		"hostname":      "host-a",
		"os":            "linux",
		"arch":          "amd64",
		"version":       "v1.1.0",
		"capabilities":  map[string]string{"executor": "script", "shells": "posix"},
		"timestamp_utc": "2026-02-11T00:00:00Z",
	})
	if hbResp.StatusCode != http.StatusOK {
		t.Fatalf("heartbeat status=%d body=%s", hbResp.StatusCode, readBody(t, hbResp))
	}
	var hbPayload struct {
		Accepted        bool   `json:"accepted"`
		UpdateRequested bool   `json:"update_requested"`
		UpdateTarget    string `json:"update_target"`
	}
	decodeJSONBody(t, hbResp, &hbPayload)
	if !hbPayload.Accepted {
		t.Fatalf("expected accepted=true")
	}
	if hbPayload.UpdateRequested {
		t.Fatalf("expected update_requested=false")
	}
	if hbPayload.UpdateTarget != "" {
		t.Fatalf("unexpected update_target: %q", hbPayload.UpdateTarget)
	}

	agentsResp := mustJSONRequest(t, ts.Client(), http.MethodGet, ts.URL+"/api/v1/agents", nil)
	if agentsResp.StatusCode != http.StatusOK {
		t.Fatalf("agents status=%d body=%s", agentsResp.StatusCode, readBody(t, agentsResp))
	}
	var agentsPayload struct {
		Agents []struct {
			AgentID string `json:"agent_id"`
			Version string `json:"version"`
		} `json:"agents"`
	}
	decodeJSONBody(t, agentsResp, &agentsPayload)
	if len(agentsPayload.Agents) != 1 {
		t.Fatalf("expected exactly one agent, got %d", len(agentsPayload.Agents))
	}
	if agentsPayload.Agents[0].AgentID != "agent-a" || agentsPayload.Agents[0].Version != "v1.1.0" {
		t.Fatalf("unexpected agent payload: %+v", agentsPayload.Agents[0])
	}
}

func TestGetAgentByIDEndpoint(t *testing.T) {
	oldVersion := version.Version
	version.Version = "v1.2.0"
	t.Cleanup(func() { version.Version = oldVersion })

	ts := newTestHTTPServer(t)
	defer ts.Close()
	client := ts.Client()

	hbResp := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/heartbeat", map[string]any{
		"agent_id":      "agent-by-id",
		"hostname":      "host-z",
		"os":            "linux",
		"arch":          "amd64",
		"version":       "v1.1.0",
		"capabilities":  map[string]string{"executor": "script", "shells": "posix"},
		"timestamp_utc": "2026-02-11T00:00:00Z",
	})
	if hbResp.StatusCode != http.StatusOK {
		t.Fatalf("heartbeat status=%d body=%s", hbResp.StatusCode, readBody(t, hbResp))
	}
	_ = readBody(t, hbResp)

	getResp := mustJSONRequest(t, client, http.MethodGet, ts.URL+"/api/v1/agents/agent-by-id", nil)
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("get agent status=%d body=%s", getResp.StatusCode, readBody(t, getResp))
	}
	var payload struct {
		Agent struct {
			AgentID     string `json:"agent_id"`
			Hostname    string `json:"hostname"`
			Version     string `json:"version"`
			NeedsUpdate bool   `json:"needs_update"`
		} `json:"agent"`
	}
	decodeJSONBody(t, getResp, &payload)
	if payload.Agent.AgentID != "agent-by-id" {
		t.Fatalf("unexpected agent id: %q", payload.Agent.AgentID)
	}
	if payload.Agent.Hostname != "host-z" {
		t.Fatalf("unexpected hostname: %q", payload.Agent.Hostname)
	}
	if payload.Agent.Version != "v1.1.0" {
		t.Fatalf("unexpected version: %q", payload.Agent.Version)
	}
	if !payload.Agent.NeedsUpdate {
		t.Fatalf("expected needs_update=true")
	}

	missingResp := mustJSONRequest(t, client, http.MethodGet, ts.URL+"/api/v1/agents/does-not-exist", nil)
	if missingResp.StatusCode != http.StatusNotFound {
		t.Fatalf("missing agent status=%d body=%s", missingResp.StatusCode, readBody(t, missingResp))
	}
	_ = readBody(t, missingResp)
}

func TestAgentListAndDetailUseConsistentViewFields(t *testing.T) {
	oldVersion := version.Version
	version.Version = "v1.2.0"
	t.Cleanup(func() { version.Version = oldVersion })

	ts := newTestHTTPServer(t)
	defer ts.Close()
	client := ts.Client()

	hbResp := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/heartbeat", map[string]any{
		"agent_id":      "agent-consistency",
		"hostname":      "host-c",
		"os":            "linux",
		"arch":          "amd64",
		"version":       "v1.1.0",
		"capabilities":  map[string]string{"executor": "script", "shells": "posix"},
		"timestamp_utc": "2026-02-11T00:00:00Z",
	})
	if hbResp.StatusCode != http.StatusOK {
		t.Fatalf("heartbeat status=%d body=%s", hbResp.StatusCode, readBody(t, hbResp))
	}
	_ = readBody(t, hbResp)

	updateResp := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/agents/agent-consistency/actions", map[string]any{"action": "update"})
	if updateResp.StatusCode != http.StatusOK {
		t.Fatalf("manual update status=%d body=%s", updateResp.StatusCode, readBody(t, updateResp))
	}
	_ = readBody(t, updateResp)

	type agentCore struct {
		AgentID         string `json:"agent_id"`
		Hostname        string `json:"hostname"`
		Version         string `json:"version"`
		Deactivated     bool   `json:"deactivated"`
		NeedsUpdate     bool   `json:"needs_update"`
		UpdateRequested bool   `json:"update_requested"`
		UpdateTarget    string `json:"update_target"`
		UpdateAttempts  int    `json:"update_attempts"`
	}

	listResp := mustJSONRequest(t, client, http.MethodGet, ts.URL+"/api/v1/agents", nil)
	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("agents list status=%d body=%s", listResp.StatusCode, readBody(t, listResp))
	}
	var listPayload struct {
		Agents []agentCore `json:"agents"`
	}
	decodeJSONBody(t, listResp, &listPayload)
	if len(listPayload.Agents) != 1 {
		t.Fatalf("expected 1 agent in list, got %d", len(listPayload.Agents))
	}

	detailResp := mustJSONRequest(t, client, http.MethodGet, ts.URL+"/api/v1/agents/agent-consistency", nil)
	if detailResp.StatusCode != http.StatusOK {
		t.Fatalf("agent detail status=%d body=%s", detailResp.StatusCode, readBody(t, detailResp))
	}
	var detailPayload struct {
		Agent agentCore `json:"agent"`
	}
	decodeJSONBody(t, detailResp, &detailPayload)

	listAgent := listPayload.Agents[0]
	detailAgent := detailPayload.Agent
	if listAgent.AgentID != detailAgent.AgentID {
		t.Fatalf("agent_id mismatch list=%q detail=%q", listAgent.AgentID, detailAgent.AgentID)
	}
	if listAgent.Hostname != detailAgent.Hostname {
		t.Fatalf("hostname mismatch list=%q detail=%q", listAgent.Hostname, detailAgent.Hostname)
	}
	if listAgent.Version != detailAgent.Version {
		t.Fatalf("version mismatch list=%q detail=%q", listAgent.Version, detailAgent.Version)
	}
	if listAgent.Deactivated != detailAgent.Deactivated {
		t.Fatalf("deactivated mismatch list=%v detail=%v", listAgent.Deactivated, detailAgent.Deactivated)
	}
	if listAgent.NeedsUpdate != detailAgent.NeedsUpdate {
		t.Fatalf("needs_update mismatch list=%v detail=%v", listAgent.NeedsUpdate, detailAgent.NeedsUpdate)
	}
	if listAgent.UpdateRequested != detailAgent.UpdateRequested {
		t.Fatalf("update_requested mismatch list=%v detail=%v", listAgent.UpdateRequested, detailAgent.UpdateRequested)
	}
	if listAgent.UpdateTarget != detailAgent.UpdateTarget {
		t.Fatalf("update_target mismatch list=%q detail=%q", listAgent.UpdateTarget, detailAgent.UpdateTarget)
	}
	if listAgent.UpdateAttempts != detailAgent.UpdateAttempts {
		t.Fatalf("update_attempts mismatch list=%d detail=%d", listAgent.UpdateAttempts, detailAgent.UpdateAttempts)
	}
}

func TestManualAgentUpdateRequestTriggersHeartbeatUpdate(t *testing.T) {
	oldVersion := version.Version
	version.Version = "v1.2.0"
	t.Cleanup(func() { version.Version = oldVersion })

	ts := newTestHTTPServer(t)
	defer ts.Close()
	client := ts.Client()

	firstHB := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/heartbeat", map[string]any{
		"agent_id":      "agent-a",
		"hostname":      "host-a",
		"os":            "darwin",
		"arch":          "arm64",
		"version":       "v1.1.0",
		"capabilities":  map[string]string{"executor": "script", "shells": "posix"},
		"timestamp_utc": "2026-02-11T00:00:00Z",
	})
	if firstHB.StatusCode != http.StatusOK {
		t.Fatalf("first heartbeat status=%d body=%s", firstHB.StatusCode, readBody(t, firstHB))
	}
	_ = readBody(t, firstHB)

	manualResp := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/agents/agent-a/actions", map[string]any{"action": "update"})
	if manualResp.StatusCode != http.StatusOK {
		t.Fatalf("manual update status=%d body=%s", manualResp.StatusCode, readBody(t, manualResp))
	}
	_ = readBody(t, manualResp)

	agentsResp := mustJSONRequest(t, client, http.MethodGet, ts.URL+"/api/v1/agents", nil)
	if agentsResp.StatusCode != http.StatusOK {
		t.Fatalf("agents status=%d body=%s", agentsResp.StatusCode, readBody(t, agentsResp))
	}
	var agentsPayload struct {
		Agents []struct {
			AgentID         string `json:"agent_id"`
			UpdateRequested bool   `json:"update_requested"`
			UpdateTarget    string `json:"update_target"`
		} `json:"agents"`
	}
	decodeJSONBody(t, agentsResp, &agentsPayload)
	if len(agentsPayload.Agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(agentsPayload.Agents))
	}
	if !agentsPayload.Agents[0].UpdateRequested {
		t.Fatalf("expected update_requested=true on agents list")
	}
	if agentsPayload.Agents[0].UpdateTarget != "v1.2.0" {
		t.Fatalf("unexpected update_target in agents list: %q", agentsPayload.Agents[0].UpdateTarget)
	}

	secondHB := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/heartbeat", map[string]any{
		"agent_id":      "agent-a",
		"hostname":      "host-a",
		"os":            "darwin",
		"arch":          "arm64",
		"version":       "v1.1.0",
		"capabilities":  map[string]string{"executor": "script", "shells": "posix"},
		"timestamp_utc": "2026-02-11T00:00:10Z",
	})
	if secondHB.StatusCode != http.StatusOK {
		t.Fatalf("second heartbeat status=%d body=%s", secondHB.StatusCode, readBody(t, secondHB))
	}
	var hbPayload struct {
		UpdateRequested bool   `json:"update_requested"`
		UpdateTarget    string `json:"update_target"`
	}
	decodeJSONBody(t, secondHB, &hbPayload)
	if !hbPayload.UpdateRequested {
		t.Fatalf("expected update_requested=true")
	}
	if hbPayload.UpdateTarget != "v1.2.0" {
		t.Fatalf("unexpected update_target: %q", hbPayload.UpdateTarget)
	}
}

func TestManualRefreshToolsRequest(t *testing.T) {
	ts := newTestHTTPServer(t)
	defer ts.Close()
	client := ts.Client()

	firstHB := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/heartbeat", map[string]any{
		"agent_id":      "agent-refresh",
		"hostname":      "host-r",
		"os":            "linux",
		"arch":          "amd64",
		"version":       "v1.0.0",
		"capabilities":  map[string]string{"executor": "script", "shells": "posix"},
		"timestamp_utc": "2026-02-11T00:00:00Z",
	})
	if firstHB.StatusCode != http.StatusOK {
		t.Fatalf("first heartbeat status=%d body=%s", firstHB.StatusCode, readBody(t, firstHB))
	}
	_ = readBody(t, firstHB)

	refreshResp := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/agents/agent-refresh/actions", map[string]any{"action": "refresh-tools"})
	if refreshResp.StatusCode != http.StatusOK {
		t.Fatalf("refresh-tools status=%d body=%s", refreshResp.StatusCode, readBody(t, refreshResp))
	}
	_ = readBody(t, refreshResp)

	secondHB := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/heartbeat", map[string]any{
		"agent_id":      "agent-refresh",
		"hostname":      "host-r",
		"os":            "linux",
		"arch":          "amd64",
		"version":       "v1.0.0",
		"capabilities":  map[string]string{"executor": "script", "shells": "posix"},
		"timestamp_utc": "2026-02-11T00:00:10Z",
	})
	if secondHB.StatusCode != http.StatusOK {
		t.Fatalf("second heartbeat status=%d body=%s", secondHB.StatusCode, readBody(t, secondHB))
	}
	var hbPayload struct {
		RefreshToolsRequested bool `json:"refresh_tools_requested"`
	}
	decodeJSONBody(t, secondHB, &hbPayload)
	if !hbPayload.RefreshToolsRequested {
		t.Fatalf("expected refresh_tools_requested=true")
	}
}

func TestManualAgentRestartRequest(t *testing.T) {
	ts := newTestHTTPServer(t)
	defer ts.Close()
	client := ts.Client()

	firstHB := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/heartbeat", map[string]any{
		"agent_id":      "agent-restart",
		"hostname":      "host-rs",
		"os":            "linux",
		"arch":          "amd64",
		"version":       "v1.0.0",
		"capabilities":  map[string]string{"executor": "script", "shells": "posix"},
		"timestamp_utc": "2026-02-11T00:00:00Z",
	})
	if firstHB.StatusCode != http.StatusOK {
		t.Fatalf("first heartbeat status=%d body=%s", firstHB.StatusCode, readBody(t, firstHB))
	}
	_ = readBody(t, firstHB)

	restartResp := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/agents/agent-restart/actions", map[string]any{"action": "restart"})
	if restartResp.StatusCode != http.StatusOK {
		t.Fatalf("restart request status=%d body=%s", restartResp.StatusCode, readBody(t, restartResp))
	}
	_ = readBody(t, restartResp)

	secondHB := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/heartbeat", map[string]any{
		"agent_id":      "agent-restart",
		"hostname":      "host-rs",
		"os":            "linux",
		"arch":          "amd64",
		"version":       "v1.0.0",
		"capabilities":  map[string]string{"executor": "script", "shells": "posix"},
		"timestamp_utc": "2026-02-11T00:00:10Z",
	})
	if secondHB.StatusCode != http.StatusOK {
		t.Fatalf("second heartbeat status=%d body=%s", secondHB.StatusCode, readBody(t, secondHB))
	}
	var secondPayload struct {
		RestartRequested bool `json:"restart_requested"`
	}
	decodeJSONBody(t, secondHB, &secondPayload)
	if !secondPayload.RestartRequested {
		t.Fatalf("expected restart_requested=true")
	}

	thirdHB := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/heartbeat", map[string]any{
		"agent_id":      "agent-restart",
		"hostname":      "host-rs",
		"os":            "linux",
		"arch":          "amd64",
		"version":       "v1.0.0",
		"capabilities":  map[string]string{"executor": "script", "shells": "posix"},
		"timestamp_utc": "2026-02-11T00:00:20Z",
	})
	if thirdHB.StatusCode != http.StatusOK {
		t.Fatalf("third heartbeat status=%d body=%s", thirdHB.StatusCode, readBody(t, thirdHB))
	}
	var thirdPayload struct {
		RestartRequested bool `json:"restart_requested"`
	}
	decodeJSONBody(t, thirdHB, &thirdPayload)
	if thirdPayload.RestartRequested {
		t.Fatalf("expected restart_requested=false after delivery")
	}
}

func TestManualAgentCacheWipeRequest(t *testing.T) {
	ts := newTestHTTPServer(t)
	defer ts.Close()
	client := ts.Client()

	firstHB := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/heartbeat", map[string]any{
		"agent_id":      "agent-wipe-cache",
		"hostname":      "host-wc",
		"os":            "linux",
		"arch":          "amd64",
		"version":       "v1.0.0",
		"capabilities":  map[string]string{"executor": "script", "shells": "posix"},
		"timestamp_utc": "2026-02-11T00:00:00Z",
	})
	if firstHB.StatusCode != http.StatusOK {
		t.Fatalf("first heartbeat status=%d body=%s", firstHB.StatusCode, readBody(t, firstHB))
	}
	_ = readBody(t, firstHB)

	wipeResp := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/agents/agent-wipe-cache/actions", map[string]any{"action": "wipe-cache"})
	if wipeResp.StatusCode != http.StatusOK {
		t.Fatalf("wipe-cache request status=%d body=%s", wipeResp.StatusCode, readBody(t, wipeResp))
	}
	_ = readBody(t, wipeResp)

	secondHB := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/heartbeat", map[string]any{
		"agent_id":      "agent-wipe-cache",
		"hostname":      "host-wc",
		"os":            "linux",
		"arch":          "amd64",
		"version":       "v1.0.0",
		"capabilities":  map[string]string{"executor": "script", "shells": "posix"},
		"timestamp_utc": "2026-02-11T00:00:10Z",
	})
	if secondHB.StatusCode != http.StatusOK {
		t.Fatalf("second heartbeat status=%d body=%s", secondHB.StatusCode, readBody(t, secondHB))
	}
	var secondPayload struct {
		WipeCacheRequested bool `json:"wipe_cache_requested"`
	}
	decodeJSONBody(t, secondHB, &secondPayload)
	if !secondPayload.WipeCacheRequested {
		t.Fatalf("expected wipe_cache_requested=true")
	}

	thirdHB := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/heartbeat", map[string]any{
		"agent_id":      "agent-wipe-cache",
		"hostname":      "host-wc",
		"os":            "linux",
		"arch":          "amd64",
		"version":       "v1.0.0",
		"capabilities":  map[string]string{"executor": "script", "shells": "posix"},
		"timestamp_utc": "2026-02-11T00:00:20Z",
	})
	if thirdHB.StatusCode != http.StatusOK {
		t.Fatalf("third heartbeat status=%d body=%s", thirdHB.StatusCode, readBody(t, thirdHB))
	}
	var thirdPayload struct {
		WipeCacheRequested bool `json:"wipe_cache_requested"`
	}
	decodeJSONBody(t, thirdHB, &thirdPayload)
	if thirdPayload.WipeCacheRequested {
		t.Fatalf("expected wipe_cache_requested=false after delivery")
	}
}

func TestManualAgentFlushJobHistory(t *testing.T) {
	ts, s := newTestHTTPServerWithState(t)
	defer ts.Close()
	client := ts.Client()

	firstHB := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/heartbeat", map[string]any{
		"agent_id":      "agent-flush-history",
		"hostname":      "host-fh",
		"os":            "linux",
		"arch":          "amd64",
		"version":       "v1.0.0",
		"capabilities":  map[string]string{"executor": "script", "shells": "posix"},
		"timestamp_utc": "2026-02-11T00:00:00Z",
	})
	if firstHB.StatusCode != http.StatusOK {
		t.Fatalf("first heartbeat status=%d body=%s", firstHB.StatusCode, readBody(t, firstHB))
	}
	_ = readBody(t, firstHB)

	createSucceededAdhocJob := func(agentID, name string) string {
		t.Helper()
		job, err := s.db.CreateJobExecution(protocol.CreateJobExecutionRequest{
			Script:         "echo " + name,
			TimeoutSeconds: 60,
			Metadata: map[string]string{
				"adhoc":          "1",
				"adhoc_agent_id": agentID,
				"adhoc_shell":    "posix",
			},
		})
		if err != nil {
			t.Fatalf("create job failed: %v", err)
		}
		jobID := job.ID
		if jobID == "" {
			t.Fatalf("created job id missing")
		}
		if _, err := s.db.UpdateJobExecutionStatus(jobID, protocol.JobExecutionStatusUpdateRequest{
			AgentID: agentID,
			Status:  protocol.JobExecutionStatusSucceeded,
		}); err != nil {
			t.Fatalf("update job status failed: %v", err)
		}
		artifactPath := filepath.Join(s.artifactsDir, jobID, "dist")
		if err := os.MkdirAll(artifactPath, 0o755); err != nil {
			t.Fatalf("mkdir artifact dir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(artifactPath, "result-"+name+".txt"), []byte("artifact:"+name), 0o644); err != nil {
			t.Fatalf("write artifact file: %v", err)
		}
		return jobID
	}

	flushJobID := createSucceededAdhocJob("agent-flush-history", "flush")
	keepJobID := createSucceededAdhocJob("agent-other", "keep")

	preFlushArtifactResp, err := client.Get(ts.URL + "/artifacts/" + flushJobID + "/dist/result-flush.txt")
	if err != nil {
		t.Fatalf("get pre-flush artifact: %v", err)
	}
	if preFlushArtifactResp.StatusCode != http.StatusOK {
		t.Fatalf("pre-flush artifact status=%d body=%s", preFlushArtifactResp.StatusCode, readBody(t, preFlushArtifactResp))
	}
	_ = readBody(t, preFlushArtifactResp)

	flushResp := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/agents/agent-flush-history/actions", map[string]any{"action": "flush-job-history"})
	if flushResp.StatusCode != http.StatusOK {
		t.Fatalf("flush-job-history status=%d body=%s", flushResp.StatusCode, readBody(t, flushResp))
	}
	_ = readBody(t, flushResp)

	flushedJobResp, err := client.Get(ts.URL + "/api/v1/jobs/" + flushJobID)
	if err != nil {
		t.Fatalf("get flushed job: %v", err)
	}
	if flushedJobResp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected flushed job to be 404, got %d body=%s", flushedJobResp.StatusCode, readBody(t, flushedJobResp))
	}
	_ = readBody(t, flushedJobResp)

	keptJobResp, err := client.Get(ts.URL + "/api/v1/jobs/" + keepJobID)
	if err != nil {
		t.Fatalf("get kept job: %v", err)
	}
	if keptJobResp.StatusCode != http.StatusOK {
		t.Fatalf("expected kept job to be 200, got %d body=%s", keptJobResp.StatusCode, readBody(t, keptJobResp))
	}
	_ = readBody(t, keptJobResp)

	flushedArtifactResp, err := client.Get(ts.URL + "/artifacts/" + flushJobID + "/dist/result-flush.txt")
	if err != nil {
		t.Fatalf("get flushed artifact: %v", err)
	}
	if flushedArtifactResp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected flushed artifact to be 404, got %d body=%s", flushedArtifactResp.StatusCode, readBody(t, flushedArtifactResp))
	}
	_ = readBody(t, flushedArtifactResp)

	keptArtifactResp, err := client.Get(ts.URL + "/artifacts/" + keepJobID + "/dist/result-keep.txt")
	if err != nil {
		t.Fatalf("get kept artifact: %v", err)
	}
	if keptArtifactResp.StatusCode != http.StatusOK {
		t.Fatalf("expected kept artifact to be 200, got %d body=%s", keptArtifactResp.StatusCode, readBody(t, keptArtifactResp))
	}
	_ = readBody(t, keptArtifactResp)

	secondHB := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/heartbeat", map[string]any{
		"agent_id":      "agent-flush-history",
		"hostname":      "host-fh",
		"os":            "linux",
		"arch":          "amd64",
		"version":       "v1.0.0",
		"capabilities":  map[string]string{"executor": "script", "shells": "posix"},
		"timestamp_utc": "2026-02-11T00:00:10Z",
	})
	if secondHB.StatusCode != http.StatusOK {
		t.Fatalf("second heartbeat status=%d body=%s", secondHB.StatusCode, readBody(t, secondHB))
	}
	var secondPayload struct {
		FlushJobHistoryRequested bool `json:"flush_job_history_requested"`
	}
	decodeJSONBody(t, secondHB, &secondPayload)
	if !secondPayload.FlushJobHistoryRequested {
		t.Fatalf("expected flush_job_history_requested=true")
	}

	thirdHB := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/heartbeat", map[string]any{
		"agent_id":      "agent-flush-history",
		"hostname":      "host-fh",
		"os":            "linux",
		"arch":          "amd64",
		"version":       "v1.0.0",
		"capabilities":  map[string]string{"executor": "script", "shells": "posix"},
		"timestamp_utc": "2026-02-11T00:00:20Z",
	})
	if thirdHB.StatusCode != http.StatusOK {
		t.Fatalf("third heartbeat status=%d body=%s", thirdHB.StatusCode, readBody(t, thirdHB))
	}
	var thirdPayload struct {
		FlushJobHistoryRequested bool `json:"flush_job_history_requested"`
	}
	decodeJSONBody(t, thirdHB, &thirdPayload)
	if thirdPayload.FlushJobHistoryRequested {
		t.Fatalf("expected flush_job_history_requested=false after delivery")
	}
}
