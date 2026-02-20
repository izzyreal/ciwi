package jobexecution

import (
	"archive/zip"
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/izzyreal/ciwi/internal/protocol"
)

func TestSanitizeZIPName(t *testing.T) {
	if got := sanitizeZIPName(" ciwi/main "); got != "ciwi-main" {
		t.Fatalf("unexpected sanitizeZIPName: %q", got)
	}
	if got := sanitizeZIPName("..."); got != "" {
		t.Fatalf("expected empty sanitize for dots-only input, got %q", got)
	}
}

func TestWriteArtifactsZIP(t *testing.T) {
	artifactsDir := t.TempDir()
	jobID := "job-1"
	base := filepath.Join(artifactsDir, jobID)
	if err := os.MkdirAll(filepath.Join(base, "dist"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(base, "dist", "a.txt"), []byte("A"), 0o644); err != nil {
		t.Fatalf("write a.txt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(base, "dist", "b.txt"), []byte("B"), 0o644); err != nil {
		t.Fatalf("write b.txt: %v", err)
	}

	rec := httptest.NewRecorder()
	err := writeArtifactsZIP(rec, artifactsDir, jobID, []protocol.JobExecutionArtifact{
		{Path: "dist/b.txt"},
		{Path: "../invalid"},
		{Path: "dist/a.txt"},
	})
	if err != nil {
		t.Fatalf("writeArtifactsZIP: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/zip" {
		t.Fatalf("unexpected content-type %q", ct)
	}
	if !strings.Contains(rec.Header().Get("Content-Disposition"), "job-1-artifacts.zip") {
		t.Fatalf("unexpected content-disposition %q", rec.Header().Get("Content-Disposition"))
	}

	zr, err := zip.NewReader(bytes.NewReader(rec.Body.Bytes()), int64(rec.Body.Len()))
	if err != nil {
		t.Fatalf("read zip: %v", err)
	}
	if len(zr.File) != 2 || zr.File[0].Name != "dist/a.txt" || zr.File[1].Name != "dist/b.txt" {
		t.Fatalf("unexpected zip entries: %+v", zr.File)
	}

	rc, err := zr.File[0].Open()
	if err != nil {
		t.Fatalf("open first zip entry: %v", err)
	}
	raw, err := io.ReadAll(rc)
	_ = rc.Close()
	if err != nil {
		t.Fatalf("read first zip entry: %v", err)
	}
	if string(raw) != "A" {
		t.Fatalf("unexpected first zip entry content: %q", string(raw))
	}
}

func TestHandleJobArtifactsDownloadAll(t *testing.T) {
	artifactsDir := t.TempDir()
	jobID := "job-1"
	base := filepath.Join(artifactsDir, jobID)
	if err := os.MkdirAll(filepath.Join(base, "dist"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(base, "dist", "a.txt"), []byte("A"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	store := &stubStore{}
	store.listJobExecutionArtifactsFn = func(id string) ([]protocol.JobExecutionArtifact, error) {
		return []protocol.JobExecutionArtifact{{JobExecutionID: id, Path: "dist/a.txt", URL: id + "/dist/a.txt", SizeBytes: 1}}, nil
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/jobs/job-1/artifacts/download-all", nil)
	handleJobArtifactsDownloadAll(rec, req, HandlerDeps{Store: store, ArtifactsDir: artifactsDir}, jobID)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/v1/jobs/job-1/artifacts/download-all", nil)
	handleJobArtifactsDownloadAll(rec, req, HandlerDeps{Store: store, ArtifactsDir: artifactsDir}, jobID)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if rec.Body.Len() == 0 {
		t.Fatalf("expected zip bytes in response body")
	}
}
