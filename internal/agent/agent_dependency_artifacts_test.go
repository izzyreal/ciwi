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
	"runtime"
	"strings"
	"testing"
)

func TestDownloadDependencyArtifacts(t *testing.T) {
	t.Setenv("CIWI_DEP_ARTIFACT_LOG_LEVEL", "")
	t.Setenv("CIWI_DEP_ARTIFACT_LOG_MAX_RESTORED_LINES", "")
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
	summary, err := downloadDependencyArtifacts(context.Background(), srv.Client(), srv.URL, "job-build-1", execDir, map[string]string{}, false)
	if err != nil {
		t.Fatalf("downloadDependencyArtifacts: %v", err)
	}
	if !strings.Contains(summary, "downloading=2") {
		t.Fatalf("unexpected summary: %s", summary)
	}
	if strings.Contains(summary, "[dep-artifacts] restored=dist/") {
		t.Fatalf("default summary should not include per-file restore lines, got: %s", summary)
	}
	if !strings.Contains(summary, "[dep-artifacts] restored=2 bytes=6 skipped=0 from job=job-build-1") {
		t.Fatalf("expected compact restored summary line, got: %s", summary)
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
	got := dependencyArtifactJobIDs([]string{"job-a", "job-b", "job-a"})
	if len(got) != 2 {
		t.Fatalf("expected 2 unique ids, got %d (%v)", len(got), got)
	}
	if got[0] != "job-a" || got[1] != "job-b" {
		t.Fatalf("unexpected order/content: %v", got)
	}
}

func TestDownloadDependencyArtifactsPrefersZIP(t *testing.T) {
	t.Setenv("CIWI_DEP_ARTIFACT_LOG_LEVEL", "")
	t.Setenv("CIWI_DEP_ARTIFACT_LOG_MAX_RESTORED_LINES", "")
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
	summary, err := downloadDependencyArtifacts(context.Background(), srv.Client(), srv.URL, "job-build-1", execDir, map[string]string{}, false)
	if err != nil {
		t.Fatalf("downloadDependencyArtifacts: %v", err)
	}
	if !strings.Contains(summary, "zip_entries=2") {
		t.Fatalf("expected zip summary, got: %s", summary)
	}
	if strings.Contains(summary, "[dep-artifacts] restored=dist/") {
		t.Fatalf("default summary should not include per-file restore lines, got: %s", summary)
	}
	if !strings.Contains(summary, "[dep-artifacts] restored=2 bytes=6 skipped=0 from job=job-build-1") {
		t.Fatalf("expected compact restored summary line, got: %s", summary)
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

func TestDownloadDependencyArtifactsZIPPreservesExecutableMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not expose Unix executable mode bits")
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	header := &zip.FileHeader{Name: "dist/tool", Method: zip.Deflate}
	header.SetMode(0o755)
	w, err := zw.CreateHeader(header)
	if err != nil {
		t.Fatalf("create zip entry: %v", err)
	}
	if _, err := io.WriteString(w, "tool"); err != nil {
		t.Fatalf("write zip entry: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/zip")
		_, _ = w.Write(buf.Bytes())
	}))
	defer srv.Close()

	execDir := t.TempDir()
	if _, err := downloadDependencyArtifacts(context.Background(), srv.Client(), srv.URL, "job-build-1", execDir, map[string]string{}, false); err != nil {
		t.Fatalf("download dependency artifacts: %v", err)
	}
	info, err := os.Stat(filepath.Join(execDir, "dist", "tool"))
	if err != nil {
		t.Fatalf("stat restored executable: %v", err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("restored executable mode = %o, want 755", info.Mode().Perm())
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

func TestDownloadDependencyArtifactsZIPVerboseWithTruncation(t *testing.T) {
	t.Setenv("CIWI_DEP_ARTIFACT_LOG_LEVEL", "verbose")
	t.Setenv("CIWI_DEP_ARTIFACT_LOG_MAX_RESTORED_LINES", "1")
	zipBytes := buildTestZIP(t, map[string]string{
		"dist/a.bin": "AAA",
		"dist/b.txt": "BBB",
	})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/jobs/job-build-1/artifacts/download-all":
			w.Header().Set("Content-Type", "application/zip")
			_, _ = w.Write(zipBytes)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	execDir := t.TempDir()
	summary, err := downloadDependencyArtifacts(context.Background(), srv.Client(), srv.URL, "job-build-1", execDir, map[string]string{}, false)
	if err != nil {
		t.Fatalf("downloadDependencyArtifacts: %v", err)
	}
	hasA := strings.Contains(summary, "[dep-artifacts] restored=dist/a.bin bytes=3")
	hasB := strings.Contains(summary, "[dep-artifacts] restored=dist/b.txt bytes=3")
	if !hasA && !hasB {
		t.Fatalf("expected one restored line in verbose mode, got: %s", summary)
	}
	if hasA && hasB {
		t.Fatalf("expected restore truncation after one line, got: %s", summary)
	}
	if !strings.Contains(summary, "[dep-artifacts] restored_truncated=1 shown=1 total=2") {
		t.Fatalf("expected truncation summary, got: %s", summary)
	}
}

func TestDownloadDependencyArtifactsRejectsEmptySource(t *testing.T) {
	emptyZIP := buildTestZIP(t, map[string]string{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/jobs/job-empty/artifacts/download-all":
			w.Header().Set("Content-Type", "application/zip")
			_, _ = w.Write(emptyZIP)
		case "/api/v1/jobs/job-empty/artifacts":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"artifacts":[]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	_, err := downloadDependencyArtifacts(context.Background(), srv.Client(), srv.URL, "job-empty", t.TempDir(), map[string]string{}, false)
	if err == nil || !strings.Contains(err.Error(), "published no artifacts") {
		t.Fatalf("expected empty explicit source failure, got %v", err)
	}
}

func TestDownloadDependencyArtifactsAllowsEmptySourceDuringDryRun(t *testing.T) {
	emptyZIP := buildTestZIP(t, map[string]string{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/jobs/job-empty/artifacts/download-all":
			w.Header().Set("Content-Type", "application/zip")
			_, _ = w.Write(emptyZIP)
		case "/api/v1/jobs/job-empty/artifacts":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"artifacts":[]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	summary, err := downloadDependencyArtifacts(context.Background(), srv.Client(), srv.URL, "job-empty", t.TempDir(), map[string]string{}, true)
	if err != nil {
		t.Fatalf("expected dry run to allow empty artifact source: %v", err)
	}
	if !strings.Contains(summary, "dry_run_empty_source job=job-empty action=skip") {
		t.Fatalf("expected dry-run empty-source log, got %q", summary)
	}
}

func TestDownloadDependencyArtifactsLogsCollisionAndOverwrites(t *testing.T) {
	firstZIP := buildTestZIP(t, map[string]string{"dist/shared.txt": "first"})
	secondZIP := buildTestZIP(t, map[string]string{"dist/shared.txt": "second"})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/zip")
		switch r.URL.Path {
		case "/api/v1/jobs/job-first/artifacts/download-all":
			_, _ = w.Write(firstZIP)
		case "/api/v1/jobs/job-second/artifacts/download-all":
			_, _ = w.Write(secondZIP)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	execDir := t.TempDir()
	restoredBy := map[string]string{}
	if _, err := downloadDependencyArtifacts(context.Background(), srv.Client(), srv.URL, "job-first", execDir, restoredBy, false); err != nil {
		t.Fatalf("restore first source: %v", err)
	}
	summary, err := downloadDependencyArtifacts(context.Background(), srv.Client(), srv.URL, "job-second", execDir, restoredBy, false)
	if err != nil {
		t.Fatalf("restore second source: %v", err)
	}
	if !strings.Contains(summary, "collision path=dist/shared.txt previous_job=job-first source_job=job-second action=overwrite") {
		t.Fatalf("expected collision log, got %q", summary)
	}
	content, err := os.ReadFile(filepath.Join(execDir, "dist", "shared.txt"))
	if err != nil {
		t.Fatalf("read overwritten artifact: %v", err)
	}
	if string(content) != "second" {
		t.Fatalf("expected later source to win, got %q", string(content))
	}
}
