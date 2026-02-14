package agent

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/izzyreal/ciwi/internal/protocol"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

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

func TestIsRetryableGitTransportError(t *testing.T) {
	tests := []struct {
		name   string
		output string
		err    error
		want   bool
	}{
		{
			name:   "retryable http2 stream reset",
			output: "fatal: unable to access 'https://github.com/izzyreal/ciwi.git/': HTTP/2 stream 1 was not closed cleanly before end of the underlying stream",
			err:    errors.New("exit status 128"),
			want:   true,
		},
		{
			name:   "retryable remote hung up",
			output: "fatal: the remote end hung up unexpectedly",
			err:    errors.New("exit status 128"),
			want:   true,
		},
		{
			name:   "non retryable auth failure",
			output: "remote: Invalid username or password.\nfatal: Authentication failed for 'https://github.com/izzyreal/ciwi.git/'",
			err:    errors.New("exit status 128"),
			want:   false,
		},
		{
			name:   "non retryable repo missing",
			output: "remote: Repository not found.\nfatal: repository 'https://github.com/izzyreal/ciwi.git/' not found",
			err:    errors.New("exit status 128"),
			want:   false,
		},
		{
			name:   "retryable timeout from error text",
			output: "",
			err:    errors.New("connection timed out"),
			want:   true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isRetryableGitTransportError(tt.output, tt.err)
			if got != tt.want {
				t.Fatalf("isRetryableGitTransportError()=%v want=%v output=%q err=%v", got, tt.want, tt.output, tt.err)
			}
		})
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

func TestParseSimpleEnv(t *testing.T) {
	in := strings.Join([]string{
		"# comment",
		" CIWI_SERVER_URL = http://host:8112 ",
		"CIWI_AGENT_ID=agent-win",
		"CIWI_GITHUB_TOKEN='ghp_abc=xyz'",
		"ignored",
		"=empty",
		"",
	}, "\n")
	got := parseSimpleEnv(in)
	if got["CIWI_SERVER_URL"] != "http://host:8112" {
		t.Fatalf("unexpected CIWI_SERVER_URL=%q", got["CIWI_SERVER_URL"])
	}
	if got["CIWI_AGENT_ID"] != "agent-win" {
		t.Fatalf("unexpected CIWI_AGENT_ID=%q", got["CIWI_AGENT_ID"])
	}
	if got["CIWI_GITHUB_TOKEN"] != "ghp_abc=xyz" {
		t.Fatalf("unexpected CIWI_GITHUB_TOKEN=%q", got["CIWI_GITHUB_TOKEN"])
	}
	if _, ok := got["ignored"]; ok {
		t.Fatal("expected invalid lines to be ignored")
	}
}

func TestCommandForScript(t *testing.T) {
	bin, args, err := commandForScript(shellPosix, "echo hi")
	if err != nil {
		t.Fatalf("commandForScript(posix): %v", err)
	}
	if bin != "sh" || len(args) != 2 || args[0] != "-c" || args[1] != "echo hi" {
		t.Fatalf("unexpected posix command: %s %v", bin, args)
	}

	bin, args, err = commandForScript(shellCmd, "echo hi")
	if runtime.GOOS == "windows" {
		if err != nil {
			t.Fatalf("commandForScript(cmd): %v", err)
		}
		if !strings.EqualFold(bin, "cmd.exe") || len(args) != 3 || args[0] != "/d" || args[1] != "/c" || args[2] != "echo hi" {
			t.Fatalf("unexpected cmd command: %s %v", bin, args)
		}
		return
	}
	if err == nil {
		t.Fatalf("expected cmd shell to fail on non-windows, got %s %v", bin, args)
	}

	bin, args, err = commandForScript(shellPowerShell, "Write-Host hi")
	if runtime.GOOS == "windows" {
		if err != nil {
			t.Fatalf("commandForScript(powershell): %v", err)
		}
		if bin != "powershell" || len(args) != 4 || args[0] != "-NoProfile" || args[1] != "-NonInteractive" || args[2] != "-Command" || args[3] != "Write-Host hi" {
			t.Fatalf("unexpected powershell command: %s %v", bin, args)
		}
		return
	}
	if err == nil {
		t.Fatalf("expected powershell shell to fail on non-windows, got %s %v", bin, args)
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
	client := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.Method != http.MethodPost {
				t.Fatalf("unexpected method: %s", r.Method)
			}
			if !strings.HasPrefix(r.URL.Path, "/api/v1/jobs/job-123/artifacts") {
				t.Fatalf("unexpected path: %s", r.URL.Path)
			}
			if err := json.NewDecoder(r.Body).Decode(&gotReq); err != nil {
				t.Fatalf("decode upload request: %v", err)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"artifacts":[]}`)),
				Header:     make(http.Header),
			}, nil
		}),
	}

	summary, err := collectAndUploadArtifacts(
		context.Background(),
		client,
		"http://example.local",
		"agent-1",
		"job-123",
		root,
		[]string{"dist/*"},
		nil,
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

func TestResolveJobCacheEnvMissThenHit(t *testing.T) {
	workDir := t.TempDir()
	execDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(execDir, "CMakeLists.txt"), []byte("cmake_minimum_required(VERSION 3.20)"), 0o644); err != nil {
		t.Fatalf("write CMakeLists: %v", err)
	}

	job := protocol.JobExecution{
		Caches: []protocol.JobCacheSpec{
			{
				ID:  "sharedcache",
				Env: "CACHE_DIR",
				Key: protocol.JobCacheKey{
					Prefix: "cache-v1",
					Files:  []string{"CMakeLists.txt"},
				},
			},
		},
	}

	env1, logs1 := resolveJobCacheEnv(workDir, execDir, job, nil)
	dir1 := env1["CACHE_DIR"]
	if dir1 == "" {
		t.Fatalf("expected CACHE_DIR to be set, logs=%v", logs1)
	}
	if !strings.Contains(strings.Join(logs1, "\n"), "source=miss") {
		t.Fatalf("expected miss log, logs=%v", logs1)
	}
	if _, err := os.Stat(dir1); err != nil {
		t.Fatalf("expected cache dir to exist: %v", err)
	}

	env2, logs2 := resolveJobCacheEnv(workDir, execDir, job, nil)
	dir2 := env2["CACHE_DIR"]
	if dir2 != dir1 {
		t.Fatalf("expected same cache dir on second resolve: %q vs %q", dir2, dir1)
	}
	if !strings.Contains(strings.Join(logs2, "\n"), "source=hit") {
		t.Fatalf("expected hit log, logs=%v", logs2)
	}
}

func TestResolveJobCacheEnvUsesRestoreKey(t *testing.T) {
	workDir := t.TempDir()
	execDir := t.TempDir()
	cacheBase := filepath.Join(workDir, "cache", "sharedcache")
	restored := filepath.Join(cacheBase, "cache-v1-old")
	if err := os.MkdirAll(restored, 0o755); err != nil {
		t.Fatalf("mkdir restore cache: %v", err)
	}

	job := protocol.JobExecution{
		Caches: []protocol.JobCacheSpec{
			{
				ID:          "sharedcache",
				Env:         "CACHE_DIR",
				RestoreKeys: []string{"cache-v1"},
				Key: protocol.JobCacheKey{
					Prefix: "cache-v1",
					Files:  []string{"CMakeLists.txt"},
				},
			},
		},
	}
	env, logs := resolveJobCacheEnv(workDir, execDir, job, nil)
	got := env["CACHE_DIR"]
	if !samePath(got, restored) {
		t.Fatalf("expected restore dir %q, got %q (logs=%v)", restored, got, logs)
	}
	if !strings.Contains(strings.Join(logs, "\n"), "source=restore:cache-v1-old") {
		t.Fatalf("expected restore source in logs, logs=%v", logs)
	}
}

func TestExecuteLeasedJobFailsWhenArtifactUploadFails(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell-script assertion test skipped on windows")
	}

	var (
		mu       sync.Mutex
		statuses []protocol.JobExecutionStatusUpdateRequest
	)
	client := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			switch {
			case r.Method == http.MethodPost && r.URL.Path == "/api/v1/jobs/job-1/status":
				var req protocol.JobExecutionStatusUpdateRequest
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					t.Fatalf("decode status: %v", err)
				}
				mu.Lock()
				statuses = append(statuses, req)
				mu.Unlock()
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(`{"ok":true}`)),
					Header:     make(http.Header),
				}, nil
			case r.Method == http.MethodPost && r.URL.Path == "/api/v1/jobs/job-1/artifacts":
				return &http.Response{
					StatusCode: http.StatusInternalServerError,
					Body:       io.NopCloser(strings.NewReader("upload failed")),
					Header:     make(http.Header),
				}, nil
			default:
				return &http.Response{
					StatusCode: http.StatusNotFound,
					Body:       io.NopCloser(strings.NewReader("not found")),
					Header:     make(http.Header),
				}, nil
			}
		}),
	}

	workDir := t.TempDir()
	job := protocol.JobExecution{
		ID:             "job-1",
		Script:         "mkdir -p dist && printf 'x' > dist/a.bin",
		TimeoutSeconds: 30,
		ArtifactGlobs:  []string{"dist/*"},
	}
	err := executeLeasedJob(context.Background(), client, "http://example.local", "agent-1", workDir, nil, job)
	if err != nil {
		t.Fatalf("executeLeasedJob: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(statuses) < 2 {
		t.Fatalf("expected at least running+failed status updates, got %d", len(statuses))
	}
	last := statuses[len(statuses)-1]
	if last.Status != "failed" {
		t.Fatalf("expected final status failed, got %q", last.Status)
	}
	if !strings.Contains(last.Error, "artifact upload failed") {
		t.Fatalf("expected artifact upload failure in error, got %q", last.Error)
	}
}

func TestExecuteLeasedJobCancelsWhenServerForceFails(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("posix shell assertion test skipped on windows")
	}

	var (
		stateChecks int32
		mu          sync.Mutex
		statuses    []protocol.JobExecutionStatusUpdateRequest
	)
	client := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			switch {
			case r.Method == http.MethodGet && r.URL.Path == "/api/v1/jobs/job-force-fail":
				check := atomic.AddInt32(&stateChecks, 1)
				status := "running"
				errText := ""
				if check >= 2 {
					status = "failed"
					errText = "force-failed from UI"
				}
				body := `{"job_execution":{"id":"job-force-fail","status":"` + status + `","error":"` + errText + `"}}`
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(body)),
					Header:     make(http.Header),
				}, nil
			case r.Method == http.MethodPost && r.URL.Path == "/api/v1/jobs/job-force-fail/status":
				var req protocol.JobExecutionStatusUpdateRequest
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					t.Fatalf("decode status: %v", err)
				}
				mu.Lock()
				statuses = append(statuses, req)
				mu.Unlock()
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(`{"ok":true}`)),
					Header:     make(http.Header),
				}, nil
			default:
				return &http.Response{
					StatusCode: http.StatusNotFound,
					Body:       io.NopCloser(strings.NewReader("not found")),
					Header:     make(http.Header),
				}, nil
			}
		}),
	}

	job := protocol.JobExecution{
		ID:             "job-force-fail",
		Script:         "sleep 4",
		TimeoutSeconds: 120,
		RequiredCapabilities: map[string]string{
			"shell": shellPosix,
		},
	}

	execCtx, execCancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer execCancel()
	if err := executeLeasedJob(execCtx, client, "http://example.local", "agent-1", t.TempDir(), nil, job); err != nil {
		t.Fatalf("executeLeasedJob: %v", err)
	}
	if atomic.LoadInt32(&stateChecks) < 2 {
		t.Fatalf("expected job state polling checks, got %d", atomic.LoadInt32(&stateChecks))
	}

	mu.Lock()
	defer mu.Unlock()
	if len(statuses) == 0 {
		t.Fatalf("expected status updates")
	}
	last := statuses[len(statuses)-1]
	if last.Status != "failed" {
		t.Fatalf("expected final status failed, got %q", last.Status)
	}
	if !strings.Contains(last.Output, "[control] job marked failed on server: force-failed from UI") {
		t.Fatalf("expected control cancel marker in output, got:\n%s", last.Output)
	}
}

func TestExecuteLeasedJobRunsPipelineStepsInSeparateShellProcesses(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("posix shell assertion test skipped on windows")
	}

	workDir := t.TempDir()
	otherDir := t.TempDir()
	jobID := "job-steps"
	execDir := filepath.Join(workDir, jobID)

	client := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.Method == http.MethodPost && r.URL.Path == "/api/v1/jobs/"+jobID+"/status" {
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(`{"ok":true}`)),
					Header:     make(http.Header),
				}, nil
			}
			return &http.Response{
				StatusCode: http.StatusNotFound,
				Body:       io.NopCloser(strings.NewReader("not found")),
				Header:     make(http.Header),
			}, nil
		}),
	}

	job := protocol.JobExecution{
		ID:             jobID,
		Script:         strings.Join([]string{`pwd > step1.txt`, `cd "` + strings.ReplaceAll(otherDir, `"`, `\"`) + `"`, `pwd > step2.txt`, `pwd > step3.txt`}, "\n"),
		TimeoutSeconds: 30,
		StepPlan: []protocol.JobStepPlanItem{
			{Index: 1, Total: 3, Name: "first", Script: `pwd > step1.txt`},
			{Index: 2, Total: 3, Name: "second", Script: `cd "` + strings.ReplaceAll(otherDir, `"`, `\"`) + `"` + "\n" + `pwd > step2.txt`},
			{Index: 3, Total: 3, Name: "third", Script: `pwd > step3.txt`},
		},
		RequiredCapabilities: map[string]string{
			"shell": shellPosix,
		},
	}
	if err := executeLeasedJob(context.Background(), client, "http://example.local", "agent-1", workDir, nil, job); err != nil {
		t.Fatalf("executeLeasedJob: %v", err)
	}

	if _, err := os.Stat(filepath.Join(execDir, "step1.txt")); err != nil {
		t.Fatalf("expected step1 output in exec dir: %v", err)
	}
	if _, err := os.Stat(filepath.Join(execDir, "step3.txt")); err != nil {
		t.Fatalf("expected step3 output in exec dir: %v", err)
	}
	if _, err := os.Stat(filepath.Join(otherDir, "step2.txt")); err != nil {
		t.Fatalf("expected step2 output in alternate dir: %v", err)
	}
	if _, err := os.Stat(filepath.Join(otherDir, "step3.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected no step3 output in alternate dir, got err=%v", err)
	}
}

func TestExecuteLeasedJobFailsWhenStepTestReportContainsFailures(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("posix shell assertion test skipped on windows")
	}

	var (
		mu       sync.Mutex
		statuses []protocol.JobExecutionStatusUpdateRequest
	)
	client := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.Method == http.MethodPost && r.URL.Path == "/api/v1/jobs/job-test-report/status" {
				var req protocol.JobExecutionStatusUpdateRequest
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					t.Fatalf("decode status: %v", err)
				}
				mu.Lock()
				statuses = append(statuses, req)
				mu.Unlock()
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(`{"ok":true}`)),
					Header:     make(http.Header),
				}, nil
			}
			if r.Method == http.MethodPost && r.URL.Path == "/api/v1/jobs/job-test-report/tests" {
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(`{"ok":true}`)),
					Header:     make(http.Header),
				}, nil
			}
			return &http.Response{
				StatusCode: http.StatusNotFound,
				Body:       io.NopCloser(strings.NewReader("not found")),
				Header:     make(http.Header),
			}, nil
		}),
	}

	job := protocol.JobExecution{
		ID:             "job-test-report",
		Script:         "mkdir -p out && printf '%s\\n' '{\"Action\":\"run\",\"Package\":\"p\",\"Test\":\"TestA\"}' '{\"Action\":\"fail\",\"Package\":\"p\",\"Test\":\"TestA\",\"Elapsed\":0.01}' > out/report.json",
		TimeoutSeconds: 30,
		StepPlan: []protocol.JobStepPlanItem{
			{
				Index:      1,
				Total:      1,
				Name:       "go_unit",
				Script:     "mkdir -p out && printf '%s\\n' '{\"Action\":\"run\",\"Package\":\"p\",\"Test\":\"TestA\"}' '{\"Action\":\"fail\",\"Package\":\"p\",\"Test\":\"TestA\",\"Elapsed\":0.01}' > out/report.json",
				Kind:       "test",
				TestName:   "go-unit",
				TestFormat: "go-test-json",
				TestReport: "out/report.json",
			},
		},
		RequiredCapabilities: map[string]string{
			"shell": shellPosix,
		},
	}
	if err := executeLeasedJob(context.Background(), client, "http://example.local", "agent-1", t.TempDir(), nil, job); err != nil {
		t.Fatalf("executeLeasedJob: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(statuses) == 0 {
		t.Fatalf("expected status updates")
	}
	last := statuses[len(statuses)-1]
	if last.Status != "failed" {
		t.Fatalf("expected failed status, got %q", last.Status)
	}
	if !strings.Contains(last.Error, "test report contains failures") {
		t.Fatalf("expected failure from parsed test report, got %q", last.Error)
	}
}

func TestExecuteLeasedJobRunningStepStatusCarriesOutputSnapshot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("posix shell assertion test skipped on windows")
	}

	var (
		mu       sync.Mutex
		statuses []protocol.JobExecutionStatusUpdateRequest
	)
	client := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.Method == http.MethodPost && r.URL.Path == "/api/v1/jobs/job-step-output/status" {
				var req protocol.JobExecutionStatusUpdateRequest
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					t.Fatalf("decode status: %v", err)
				}
				mu.Lock()
				statuses = append(statuses, req)
				mu.Unlock()
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(`{"ok":true}`)),
					Header:     make(http.Header),
				}, nil
			}
			return &http.Response{
				StatusCode: http.StatusNotFound,
				Body:       io.NopCloser(strings.NewReader("not found")),
				Header:     make(http.Header),
			}, nil
		}),
	}

	job := protocol.JobExecution{
		ID:             "job-step-output",
		Script:         "true",
		TimeoutSeconds: 30,
		StepPlan: []protocol.JobStepPlanItem{
			{Index: 1, Total: 1, Name: "compile", Script: "true"},
		},
		RequiredCapabilities: map[string]string{
			"shell": shellPosix,
		},
	}
	if err := executeLeasedJob(context.Background(), client, "http://example.local", "agent-1", t.TempDir(), nil, job); err != nil {
		t.Fatalf("executeLeasedJob: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(statuses) == 0 {
		t.Fatalf("expected status updates")
	}
	foundRunningStep := false
	foundStepStartedEvent := false
	for _, st := range statuses {
		if st.Status != "running" {
			continue
		}
		if !strings.HasPrefix(st.CurrentStep, "Step 1/1:") {
			continue
		}
		foundRunningStep = true
		if strings.TrimSpace(st.Output) == "" {
			t.Fatalf("expected running step status to include output snapshot")
		}
		for _, event := range st.Events {
			if event.Type == protocol.JobExecutionEventTypeStepStarted {
				foundStepStartedEvent = true
				break
			}
		}
	}
	if !foundRunningStep {
		t.Fatalf("expected running step status update")
	}
	if !foundStepStartedEvent {
		t.Fatalf("expected at least one running status update with step.started event")
	}
}

func TestExecuteLeasedJobIncludesDryRunSkippedStepNoteInOutput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("posix shell assertion test skipped on windows")
	}

	var (
		mu       sync.Mutex
		statuses []protocol.JobExecutionStatusUpdateRequest
	)
	client := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.Method == http.MethodPost && r.URL.Path == "/api/v1/jobs/job-dry-skip/status" {
				var req protocol.JobExecutionStatusUpdateRequest
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					t.Fatalf("decode status: %v", err)
				}
				mu.Lock()
				statuses = append(statuses, req)
				mu.Unlock()
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(`{"ok":true}`)),
					Header:     make(http.Header),
				}, nil
			}
			return &http.Response{
				StatusCode: http.StatusNotFound,
				Body:       io.NopCloser(strings.NewReader("not found")),
				Header:     make(http.Header),
			}, nil
		}),
	}

	job := protocol.JobExecution{
		ID:             "job-dry-skip",
		Script:         ":",
		TimeoutSeconds: 30,
		StepPlan: []protocol.JobStepPlanItem{
			{Index: 1, Total: 1, Name: "step 2", Kind: "dryrun_skip"},
		},
		RequiredCapabilities: map[string]string{
			"shell": shellPosix,
		},
	}
	if err := executeLeasedJob(context.Background(), client, "http://example.local", "agent-1", t.TempDir(), nil, job); err != nil {
		t.Fatalf("executeLeasedJob: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(statuses) == 0 {
		t.Fatalf("expected status updates")
	}
	last := statuses[len(statuses)-1]
	if !strings.Contains(last.Output, "[dry-run] skipped step: step 2") {
		t.Fatalf("expected dry-run skipped step note in output, got:\n%s", last.Output)
	}
}

func TestExecuteLeasedJobDisablesShellTraceForAdhoc(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("posix shell assertion test skipped on windows")
	}

	var (
		mu       sync.Mutex
		statuses []protocol.JobExecutionStatusUpdateRequest
	)
	client := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.Method == http.MethodPost && r.URL.Path == "/api/v1/jobs/job-adhoc/status" {
				var req protocol.JobExecutionStatusUpdateRequest
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					t.Fatalf("decode status: %v", err)
				}
				mu.Lock()
				statuses = append(statuses, req)
				mu.Unlock()
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(`{"ok":true}`)),
					Header:     make(http.Header),
				}, nil
			}
			return &http.Response{
				StatusCode: http.StatusNotFound,
				Body:       io.NopCloser(strings.NewReader("not found")),
				Header:     make(http.Header),
			}, nil
		}),
	}

	job := protocol.JobExecution{
		ID:             "job-adhoc",
		Script:         `echo "hello from adhoc"`,
		TimeoutSeconds: 30,
		Metadata: map[string]string{
			"adhoc": "1",
		},
	}
	if err := executeLeasedJob(context.Background(), client, "http://example.local", "agent-1", t.TempDir(), nil, job); err != nil {
		t.Fatalf("executeLeasedJob: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(statuses) == 0 {
		t.Fatalf("expected status updates")
	}
	last := statuses[len(statuses)-1]
	if last.Status != "succeeded" {
		t.Fatalf("expected succeeded status, got %q", last.Status)
	}
	if !strings.Contains(last.Output, "[run] shell_trace=false") {
		t.Fatalf("expected shell trace disabled in output, got:\n%s", last.Output)
	}
}

func TestStageCmdScript(t *testing.T) {
	execDir := t.TempDir()
	cmd, err := stageCmdScript(execDir, "@echo off\necho hello")
	if err != nil {
		t.Fatalf("stageCmdScript: %v", err)
	}
	wantPath := filepath.Join(execDir, "ciwi-job-script.cmd")
	if cmd != wantPath {
		t.Fatalf("unexpected staged cmd path: got=%q want=%q", cmd, wantPath)
	}
	data, err := os.ReadFile(wantPath)
	if err != nil {
		t.Fatalf("read staged script: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "\r\n") {
		t.Fatalf("expected CRLF line endings, got %q", content)
	}
	if !strings.Contains(content, "echo hello") {
		t.Fatalf("expected script body in staged file, got %q", content)
	}
}

func TestReportTerminalJobStatusWithRetry(t *testing.T) {
	var attempts int32
	client := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			n := atomic.AddInt32(&attempts, 1)
			if n < 3 {
				return nil, errors.New("temporary network error")
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"ok":true}`)),
				Header:     make(http.Header),
			}, nil
		}),
	}
	err := reportTerminalJobStatusWithRetry(client, "http://example.local", "job-xyz", protocol.JobExecutionStatusUpdateRequest{
		AgentID: "agent-1",
		Status:  "succeeded",
	})
	if err != nil {
		t.Fatalf("reportTerminalJobStatusWithRetry: %v", err)
	}
	if got := atomic.LoadInt32(&attempts); got != 3 {
		t.Fatalf("expected 3 attempts, got %d", got)
	}
}

func TestShouldRetryUpdateHTTPStatus(t *testing.T) {
	tests := []struct {
		status int
		want   bool
	}{
		{status: http.StatusTooManyRequests, want: true},
		{status: http.StatusInternalServerError, want: true},
		{status: http.StatusBadGateway, want: true},
		{status: http.StatusServiceUnavailable, want: true},
		{status: http.StatusGatewayTimeout, want: true},
		{status: http.StatusForbidden, want: false},
		{status: http.StatusNotFound, want: false},
	}
	for _, tt := range tests {
		if got := shouldRetryUpdateHTTPStatus(tt.status); got != tt.want {
			t.Fatalf("shouldRetryUpdateHTTPStatus(%d)=%v want=%v", tt.status, got, tt.want)
		}
	}
}

func TestShouldRetryUpdateError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{name: "context canceled", err: context.Canceled, want: false},
		{name: "deadline exceeded", err: context.DeadlineExceeded, want: false},
		{name: "http2 stream", err: errors.New("HTTP/2 stream 1 was not closed cleanly before end of the underlying stream"), want: true},
		{name: "temporary dns", err: &net.DNSError{IsTemporary: true}, want: true},
		{name: "timeout marker", err: errors.New("connection timed out"), want: true},
		{name: "non transient", err: errors.New("permission denied"), want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldRetryUpdateError(tt.err); got != tt.want {
				t.Fatalf("shouldRetryUpdateError(%v)=%v want=%v", tt.err, got, tt.want)
			}
		})
	}
}

func TestUpdateRetryDelay(t *testing.T) {
	tests := []struct {
		attempt int
		want    string
	}{
		{attempt: 0, want: "1s"},
		{attempt: 1, want: "1s"},
		{attempt: 2, want: "2s"},
		{attempt: 3, want: "4s"},
		{attempt: 10, want: "4s"},
	}
	for _, tt := range tests {
		if got := updateRetryDelay(tt.attempt).String(); got != tt.want {
			t.Fatalf("updateRetryDelay(%d)=%s want=%s", tt.attempt, got, tt.want)
		}
	}
}
