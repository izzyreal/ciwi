package server

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"io"
	"net/http"
	"sort"
	"strings"
	"testing"
)

func TestJobArtifactsDownloadAllZip(t *testing.T) {
	ts := newTestHTTPServer(t)
	defer ts.Close()
	client := ts.Client()

	hbResp := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/heartbeat", map[string]any{
		"agent_id":      "agent-zip",
		"hostname":      "host-zip",
		"os":            "linux",
		"arch":          "amd64",
		"version":       "v1.0.0",
		"capabilities":  map[string]string{"executor": "script", "shells": "posix"},
		"timestamp_utc": "2026-02-12T00:00:00Z",
	})
	if hbResp.StatusCode != http.StatusOK {
		t.Fatalf("heartbeat status=%d body=%s", hbResp.StatusCode, readBody(t, hbResp))
	}
	_ = readBody(t, hbResp)

	runResp := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/agents/agent-zip/actions", map[string]any{
		"action": "run-script",
		"shell":  "posix",
		"script": "echo zip",
	})
	if runResp.StatusCode != http.StatusCreated {
		t.Fatalf("run-script status=%d body=%s", runResp.StatusCode, readBody(t, runResp))
	}
	var runPayload struct {
		JobExecutionID string `json:"job_execution_id"`
	}
	decodeJSONBody(t, runResp, &runPayload)
	if strings.TrimSpace(runPayload.JobExecutionID) == "" {
		t.Fatalf("missing job execution id in run-script response")
	}

	uploadResp := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/jobs/"+runPayload.JobExecutionID+"/artifacts", map[string]any{
		"agent_id": "agent-zip",
		"artifacts": []map[string]any{
			{
				"path":        "dist/a.txt",
				"data_base64": base64.StdEncoding.EncodeToString([]byte("alpha")),
			},
			{
				"path":        "dist/nested/b.txt",
				"data_base64": base64.StdEncoding.EncodeToString([]byte("bravo")),
			},
		},
	})
	if uploadResp.StatusCode != http.StatusOK {
		t.Fatalf("upload artifacts status=%d body=%s", uploadResp.StatusCode, readBody(t, uploadResp))
	}
	_ = readBody(t, uploadResp)

	zipResp, err := client.Get(ts.URL + "/api/v1/jobs/" + runPayload.JobExecutionID + "/artifacts/download-all")
	if err != nil {
		t.Fatalf("download-all request: %v", err)
	}
	defer zipResp.Body.Close()
	if zipResp.StatusCode != http.StatusOK {
		t.Fatalf("download-all status=%d body=%s", zipResp.StatusCode, readBody(t, zipResp))
	}
	if ct := strings.ToLower(strings.TrimSpace(zipResp.Header.Get("Content-Type"))); !strings.Contains(ct, "application/zip") {
		t.Fatalf("expected application/zip content-type, got %q", ct)
	}
	if cd := zipResp.Header.Get("Content-Disposition"); !strings.Contains(cd, "attachment;") || !strings.Contains(cd, ".zip") {
		t.Fatalf("expected zip attachment content-disposition, got %q", cd)
	}

	body, err := io.ReadAll(zipResp.Body)
	if err != nil {
		t.Fatalf("read zip body: %v", err)
	}
	reader, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		t.Fatalf("parse zip: %v", err)
	}
	names := make([]string, 0, len(reader.File))
	for _, f := range reader.File {
		names = append(names, f.Name)
	}
	sort.Strings(names)
	want := []string{"dist/a.txt", "dist/nested/b.txt"}
	if len(names) != len(want) {
		t.Fatalf("unexpected zip file count got=%d want=%d names=%v", len(names), len(want), names)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("unexpected zip entries got=%v want=%v", names, want)
		}
	}
}
