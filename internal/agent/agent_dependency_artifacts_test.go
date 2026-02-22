package agent

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDownloadDependencyArtifacts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/jobs/job-build-1/artifacts":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"artifacts":[{"path":"dist/a.bin","url":"/artifacts/job-build-1/dist/a.bin"},{"path":"dist/b.txt","url":"/artifacts/job-build-1/dist/b.txt"}]}`))
		case "/artifacts/job-build-1/dist/a.bin":
			_, _ = w.Write([]byte("AAA"))
		case "/artifacts/job-build-1/dist/b.txt":
			_, _ = w.Write([]byte("BBB"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	execDir := t.TempDir()
	summary, err := downloadDependencyArtifacts(context.Background(), srv.Client(), srv.URL, "job-build-1", execDir)
	if err != nil {
		t.Fatalf("downloadDependencyArtifacts: %v", err)
	}
	if !strings.Contains(summary, "downloading=2") {
		t.Fatalf("unexpected summary: %s", summary)
	}
	a, err := os.ReadFile(filepath.Join(execDir, "dist", "a.bin"))
	if err != nil {
		t.Fatalf("read restored a.bin: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(execDir, "dist", "b.txt"))
	if err != nil {
		t.Fatalf("read restored b.txt: %v", err)
	}
	if string(a) != "AAA" || string(b) != "BBB" {
		t.Fatalf("unexpected restored content a=%q b=%q", string(a), string(b))
	}
}

func TestDependencyArtifactJobIDs(t *testing.T) {
	got := dependencyArtifactJobIDs(map[string]string{
		"CIWI_DEP_ARTIFACT_JOB_IDS": "job-a, job-b ,job-a",
		"CIWI_DEP_ARTIFACT_JOB_ID":  "job-c",
	})
	if len(got) != 3 {
		t.Fatalf("expected 3 unique ids, got %d (%v)", len(got), got)
	}
	if got[0] != "job-a" || got[1] != "job-b" || got[2] != "job-c" {
		t.Fatalf("unexpected order/content: %v", got)
	}
}

func TestDownloadDependencyArtifactsPrefersZIP(t *testing.T) {
	zipBytes := buildTestZIP(t, map[string]string{
		"dist/a.bin": "AAA",
		"dist/b.txt": "BBB",
	})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/jobs/job-build-1/artifacts/download-all":
			w.Header().Set("Content-Type", "application/zip")
			_, _ = w.Write(zipBytes)
		case "/api/v1/jobs/job-build-1/artifacts":
			t.Fatalf("fallback list endpoint should not be called when zip download succeeds")
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	execDir := t.TempDir()
	summary, err := downloadDependencyArtifacts(context.Background(), srv.Client(), srv.URL, "job-build-1", execDir)
	if err != nil {
		t.Fatalf("downloadDependencyArtifacts: %v", err)
	}
	if !strings.Contains(summary, "zip_entries=2") {
		t.Fatalf("expected zip summary, got: %s", summary)
	}
	a, err := os.ReadFile(filepath.Join(execDir, "dist", "a.bin"))
	if err != nil {
		t.Fatalf("read restored a.bin: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(execDir, "dist", "b.txt"))
	if err != nil {
		t.Fatalf("read restored b.txt: %v", err)
	}
	if string(a) != "AAA" || string(b) != "BBB" {
		t.Fatalf("unexpected restored content a=%q b=%q", string(a), string(b))
	}
}

func buildTestZIP(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("create zip entry: %v", err)
		}
		if _, err := io.WriteString(w, content); err != nil {
			t.Fatalf("write zip entry: %v", err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip writer: %v", err)
	}
	return buf.Bytes()
}
