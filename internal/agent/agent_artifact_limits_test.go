package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/izzyreal/ciwi/internal/protocol"
)

func TestArtifactMaxFileBytesFromEnv(t *testing.T) {
	for _, tc := range []struct {
		name    string
		value   string
		unset   bool
		want    int64
		wantErr bool
	}{
		{name: "unset", unset: true, want: 536870912},
		{name: "empty", want: 536870912},
		{name: "blank", value: " \t\n", want: 536870912},
		{name: "override", value: "1024", want: 1024},
		{name: "trimmed", value: " 2048 ", want: 2048},
		{name: "decimal", value: "010", want: 10},
		{name: "int64 maximum", value: "9223372036854775807", want: 9223372036854775807},
		{name: "zero", value: "0", wantErr: true},
		{name: "negative", value: "-1", wantErr: true},
		{name: "units", value: "512MiB", wantErr: true},
		{name: "fraction", value: "1.5", wantErr: true},
		{name: "hex", value: "0x200", wantErr: true},
		{name: "overflow", value: "9223372036854775808", wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("CIWI_ARTIFACT_MAX_FILE_BYTES", tc.value)
			if tc.unset {
				if err := os.Unsetenv("CIWI_ARTIFACT_MAX_FILE_BYTES"); err != nil {
					t.Fatal(err)
				}
			}
			got, err := artifactMaxFileBytesFromEnv()
			if tc.wantErr {
				if err == nil || !strings.Contains(err.Error(), "CIWI_ARTIFACT_MAX_FILE_BYTES must be a positive decimal byte count") {
					t.Fatalf("expected configuration error, got limit=%d err=%v", got, err)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("limit=%d err=%v, want %d", got, err, tc.want)
			}
		})
	}
}

func TestCollectArtifactsFileSizeLimit(t *testing.T) {
	t.Setenv("CIWI_ARTIFACT_MAX_FILE_BYTES", "8")
	for _, size := range []int{7, 8, 9} {
		t.Run(fmt.Sprintf("%d bytes", size), func(t *testing.T) {
			root := t.TempDir()
			if err := os.WriteFile(filepath.Join(root, "installer.pkg"), make([]byte, size), 0o644); err != nil {
				t.Fatal(err)
			}
			artifacts, _, err := collectArtifacts(root, []string{"*.pkg"})
			if size > 8 {
				if err == nil || !strings.Contains(err.Error(), `artifact "installer.pkg" size 9 bytes exceeds CIWI_ARTIFACT_MAX_FILE_BYTES limit of 8 bytes`) {
					t.Fatalf("expected oversized artifact error, got %v", err)
				}
				if len(artifacts) != 0 {
					t.Fatal("oversized collection returned artifacts")
				}
				return
			}
			if err != nil || len(artifacts) != 1 || len(artifacts[0].Content) != size {
				t.Fatalf("expected one %d-byte artifact, got count=%d err=%v", size, len(artifacts), err)
			}
		})
	}
}

func TestCollectArtifactsAcceptsReleaseInstallerSize(t *testing.T) {
	t.Setenv("CIWI_ARTIFACT_MAX_FILE_BYTES", "")
	const installerSize = 105056149
	root := t.TempDir()
	file, err := os.Create(filepath.Join(root, "VMPC2000XL-Installer-Intel-M1-v0.9.19.pkg"))
	if err != nil {
		t.Fatal(err)
	}
	truncateErr := file.Truncate(installerSize)
	closeErr := file.Close()
	if truncateErr != nil || closeErr != nil {
		t.Fatalf("create installer fixture: truncate=%v close=%v", truncateErr, closeErr)
	}
	artifacts, _, err := collectArtifacts(root, []string{"*.pkg"})
	if err != nil || len(artifacts) != 1 || len(artifacts[0].Content) != installerSize {
		t.Fatalf("expected installer to be collected, got count=%d err=%v", len(artifacts), err)
	}
}

func TestCollectAndUploadArtifactsRejectsInvalidCollection(t *testing.T) {
	for _, tc := range []struct {
		name  string
		limit string
		want  string
	}{
		{name: "oversized file after valid file", limit: "8", want: `artifact "z-installer.pkg" size 9 bytes exceeds CIWI_ARTIFACT_MAX_FILE_BYTES limit of 8 bytes`},
		{name: "invalid configuration", limit: "invalid", want: "CIWI_ARTIFACT_MAX_FILE_BYTES must be a positive decimal byte count"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("CIWI_ARTIFACT_MAX_FILE_BYTES", tc.limit)
			root := t.TempDir()
			for name, content := range map[string]string{"a-version.txt": "0.9.19", "z-installer.pkg": "123456789"} {
				if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				t.Errorf("unexpected upload request: %s", r.URL.Path)
				return jsonHTTPResponse(http.StatusOK, `{}`), nil
			})}
			summary, err := collectAndUploadArtifacts(context.Background(), client, "http://ciwi", "agent", "job", root, []string{"*"}, nil)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected collection failure containing %q, got %v", tc.want, err)
			}
			if strings.Contains(summary, "[artifacts] uploaded") {
				t.Fatal("failed collection reported uploaded artifacts")
			}
		})
	}
}

func TestCollectArtifactsWithNoMatches(t *testing.T) {
	t.Setenv("CIWI_ARTIFACT_MAX_FILE_BYTES", "8")
	root := t.TempDir()
	for _, globs := range [][]string{nil, {"missing/*.pkg"}} {
		artifacts, _, err := collectArtifacts(root, globs)
		if err != nil || len(artifacts) != 0 {
			t.Fatalf("empty collection changed for %v: count=%d err=%v", globs, len(artifacts), err)
		}
	}
}

func TestExecuteLeasedJobFailsWhenArtifactExceedsLimit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("posix shell assertion test skipped on windows")
	}
	t.Setenv("CIWI_ARTIFACT_MAX_FILE_BYTES", "8")
	var mu sync.Mutex
	var statuses []protocol.JobExecutionStatusUpdateRequest
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v1/jobs/job-1/status" {
			var status protocol.JobExecutionStatusUpdateRequest
			if err := json.NewDecoder(r.Body).Decode(&status); err != nil {
				return nil, err
			}
			mu.Lock()
			statuses = append(statuses, status)
			mu.Unlock()
			return jsonHTTPResponse(http.StatusOK, `{}`), nil
		}
		if r.URL.Path == "/api/v1/jobs/job-1/artifacts/upload-zip" {
			t.Error("oversized artifact triggered an upload")
		}
		return jsonHTTPResponse(http.StatusNotFound, `{}`), nil
	})}
	job := protocol.JobExecution{
		ID: "job-1", Script: "mkdir -p dist && printf '123456789' > dist/installer.pkg",
		TimeoutSeconds: 30, ArtifactGlobs: []string{"dist/*.pkg"},
	}
	if err := executeLeasedJob(context.Background(), client, "http://ciwi", "agent", t.TempDir(), nil, job); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(statuses) == 0 {
		t.Fatal("no job status reported")
	}
	wantError := `artifact "dist/installer.pkg" size 9 bytes exceeds CIWI_ARTIFACT_MAX_FILE_BYTES limit of 8 bytes`
	last := statuses[len(statuses)-1]
	if last.Status != protocol.JobExecutionStatusFailed || !strings.Contains(last.Error, wantError) {
		t.Fatalf("expected failed job with size details, got status=%s error=%q", last.Status, last.Error)
	}
	var failedPhase bool
	for _, status := range statuses {
		if status.Status == protocol.JobExecutionStatusSucceeded {
			t.Fatal("oversized artifact job reported success")
		}
		for _, event := range status.Events {
			if event.Type == protocol.JobExecutionEventTypePhaseFinished && event.Phase != nil && event.Phase.ID == protocol.JobExecutionPhaseArtifacts {
				if event.Error != wantError {
					t.Fatalf("artifact phase error=%q, want %q", event.Error, wantError)
				}
				failedPhase = true
			}
		}
	}
	if !failedPhase {
		t.Fatal("artifact phase did not report failure")
	}
}
