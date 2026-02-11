package agent

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/izzyreal/ciwi/internal/protocol"
)

func TestWithGoVerbose(t *testing.T) {
	base := []string{"PATH=/usr/bin"}
	got := withGoVerbose(base, true)
	found := false
	for _, e := range got {
		if e == "GOFLAGS=-v" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected GOFLAGS=-v in env, got %v", got)
	}

	already := []string{"PATH=/usr/bin", "GOFLAGS=-mod=mod"}
	got = withGoVerbose(already, true)
	if len(got) != len(already) {
		t.Fatalf("expected env unchanged when GOFLAGS already set")
	}

	got = withGoVerbose(base, false)
	if len(got) != len(base) {
		t.Fatalf("expected env unchanged when verbose disabled")
	}
}

func TestBoolEnv(t *testing.T) {
	t.Setenv("CIWI_BOOL_TEST", "yes")
	if !boolEnv("CIWI_BOOL_TEST", false) {
		t.Fatal("expected yes to parse as true")
	}
	t.Setenv("CIWI_BOOL_TEST", "off")
	if boolEnv("CIWI_BOOL_TEST", true) {
		t.Fatal("expected off to parse as false")
	}
	t.Setenv("CIWI_BOOL_TEST", "invalid")
	if !boolEnv("CIWI_BOOL_TEST", true) {
		t.Fatal("expected invalid to fall back to default=true")
	}
}

func TestTrimOutput(t *testing.T) {
	short := "hello"
	if trimOutput(short) != short {
		t.Fatalf("short output should remain unchanged")
	}

	long := strings.Repeat("x", maxReportedOutputBytes+128)
	trimmed := trimOutput(long)
	if len(trimmed) != maxReportedOutputBytes {
		t.Fatalf("expected trimmed len %d, got %d", maxReportedOutputBytes, len(trimmed))
	}
	if trimmed != long[len(long)-maxReportedOutputBytes:] {
		t.Fatal("expected trimOutput to keep tail of output")
	}
}

func TestCommandForScript(t *testing.T) {
	bin, args := commandForScript("echo hi")
	if runtime.GOOS == "windows" {
		if bin != "powershell" || len(args) == 0 || args[len(args)-1] != "echo hi" {
			t.Fatalf("unexpected windows command: %s %v", bin, args)
		}
		return
	}
	if bin != "sh" || len(args) != 2 || args[0] != "-c" || args[1] != "echo hi" {
		t.Fatalf("unexpected unix command: %s %v", bin, args)
	}
}

func TestCollectArtifacts(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "dist", "nested"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "dist", "a.txt"), []byte("A"), 0o644); err != nil {
		t.Fatalf("write a: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "dist", "nested", "b.txt"), []byte("B"), 0o644); err != nil {
		t.Fatalf("write b: %v", err)
	}

	uploads, summary, err := collectArtifacts(root, []string{"dist/**/*.txt"})
	if err != nil {
		t.Fatalf("collect artifacts: %v", err)
	}
	if len(uploads) != 2 {
		t.Fatalf("expected 2 uploads, got %d (summary=%s)", len(uploads), summary)
	}
	if uploads[0].Path != "dist/a.txt" || uploads[1].Path != "dist/nested/b.txt" {
		t.Fatalf("unexpected artifact paths: %+v", uploads)
	}
	a, _ := base64.StdEncoding.DecodeString(uploads[0].DataBase64)
	b, _ := base64.StdEncoding.DecodeString(uploads[1].DataBase64)
	if string(a) != "A" || string(b) != "B" {
		t.Fatalf("unexpected decoded artifact content")
	}
	if !strings.Contains(summary, "[artifacts] include=dist/a.txt") {
		t.Fatalf("expected summary to include dist/a.txt, got: %s", summary)
	}
}

func TestCollectAndUploadArtifacts(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "dist"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "dist", "ciwi.bin"), []byte("ciwi"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	var gotReq protocol.UploadArtifactsRequest
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		if !strings.HasPrefix(r.URL.Path, "/api/v1/jobs/job-123/artifacts") {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&gotReq); err != nil {
			t.Fatalf("decode upload request: %v", err)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"artifacts":[]}`))
	}))
	defer ts.Close()

	summary, err := collectAndUploadArtifacts(
		context.Background(),
		ts.Client(),
		ts.URL,
		"agent-1",
		"job-123",
		root,
		[]string{"dist/*"},
	)
	if err != nil {
		t.Fatalf("collectAndUploadArtifacts: %v", err)
	}
	if gotReq.AgentID != "agent-1" {
		t.Fatalf("unexpected agent id: %q", gotReq.AgentID)
	}
	if len(gotReq.Artifacts) != 1 {
		t.Fatalf("expected 1 uploaded artifact, got %d", len(gotReq.Artifacts))
	}
	if gotReq.Artifacts[0].Path != "dist/ciwi.bin" {
		t.Fatalf("unexpected artifact path: %q", gotReq.Artifacts[0].Path)
	}
	if !strings.Contains(summary, "[artifacts] uploaded") {
		t.Fatalf("expected uploaded marker in summary, got: %s", summary)
	}
}

func TestParseJobTestReport(t *testing.T) {
	output := strings.Join([]string{
		"random line",
		"__CIWI_TEST_BEGIN__ name=go-unit format=go-test-json",
		`{"Time":"2026-01-01T00:00:00Z","Action":"run","Package":"p","Test":"TestA"}`,
		`{"Time":"2026-01-01T00:00:00Z","Action":"pass","Package":"p","Test":"TestA","Elapsed":0.01}`,
		`{"Time":"2026-01-01T00:00:00Z","Action":"run","Package":"p","Test":"TestB"}`,
		`{"Time":"2026-01-01T00:00:00Z","Action":"fail","Package":"p","Test":"TestB","Elapsed":0.02}`,
		"__CIWI_TEST_END__",
	}, "\n")

	report := parseJobTestReport(output)
	if report.Total != 2 || report.Passed != 1 || report.Failed != 1 || report.Skipped != 0 {
		t.Fatalf("unexpected aggregate report: %+v", report)
	}
	if len(report.Suites) != 1 {
		t.Fatalf("expected 1 suite, got %d", len(report.Suites))
	}
	s := report.Suites[0]
	if s.Name != "go-unit" || s.Format != "go-test-json" {
		t.Fatalf("unexpected suite metadata: %+v", s)
	}
	if len(s.Cases) != 2 {
		t.Fatalf("expected 2 cases, got %d", len(s.Cases))
	}
}
