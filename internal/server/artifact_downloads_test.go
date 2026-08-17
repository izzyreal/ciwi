package server

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/izzyreal/ciwi/internal/application"
	"github.com/izzyreal/ciwi/internal/protocol"
)

type artifactDownloadStoreStub struct {
	artifacts []protocol.JobExecutionArtifact
	job       protocol.JobExecution
	events    []protocol.JobExecutionEvent
}

type blockingArtifactDownloadStore struct {
	artifactDownloadStoreStub
	calls   atomic.Int32
	entered chan struct{}
	release chan struct{}
}

func (s *blockingArtifactDownloadStore) ListJobExecutionArtifacts(jobID string) ([]protocol.JobExecutionArtifact, error) {
	if s.calls.Add(1) > 1 {
		s.entered <- struct{}{}
		<-s.release
	}
	return s.artifactDownloadStoreStub.ListJobExecutionArtifacts(jobID)
}

func (s artifactDownloadStoreStub) ListJobExecutionArtifacts(string) ([]protocol.JobExecutionArtifact, error) {
	return append([]protocol.JobExecutionArtifact(nil), s.artifacts...), nil
}

func (s artifactDownloadStoreStub) GetJobExecution(string) (protocol.JobExecution, error) {
	return s.job, nil
}

func (s artifactDownloadStoreStub) ListJobExecutionEvents(string) ([]protocol.JobExecutionEvent, error) {
	return append([]protocol.JobExecutionEvent(nil), s.events...), nil
}

func TestArtifactDownloadServiceStreamsFilesAndPrefixArchives(t *testing.T) {
	artifactsDir := t.TempDir()
	jobID := "job-1"
	files := map[string][]byte{
		"dist/app.bin":    bytes.Repeat([]byte("a"), artifactDownloadChunkSize+37),
		"dist/readme.txt": []byte("hello"),
		"logs/build.log":  []byte("compiled"),
	}
	artifacts := make([]protocol.JobExecutionArtifact, 0, len(files))
	for path, content := range files {
		fullPath := filepath.Join(artifactsDir, jobID, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(fullPath, content, 0o644); err != nil {
			t.Fatal(err)
		}
		artifacts = append(artifacts, protocol.JobExecutionArtifact{Path: path, SizeBytes: int64(len(content))})
	}
	service := newArtifactDownloadService(artifactDownloadStoreStub{artifacts: artifacts}, artifactsDir)

	first, err := service.DownloadArtifact(context.Background(), application.ArtifactDownloadRequest{
		JobExecutionID: jobID, Kind: "file", Path: "dist/app.bin",
	})
	if err != nil {
		t.Fatal(err)
	}
	if first.Complete || len(first.Data) != artifactDownloadChunkSize || first.NextOffset != artifactDownloadChunkSize || first.Token == "" || !strings.HasPrefix(first.ContentID, "sha256:") {
		t.Fatalf("first chunk = %+v", first)
	}
	second, err := service.DownloadArtifact(context.Background(), application.ArtifactDownloadRequest{Token: first.Token, Offset: first.NextOffset})
	if err != nil {
		t.Fatal(err)
	}
	if !second.Complete || len(second.Data) != 37 || second.FileName != "app.bin" {
		t.Fatalf("second chunk = %+v", second)
	}

	archive, err := service.DownloadArtifact(context.Background(), application.ArtifactDownloadRequest{
		JobExecutionID: jobID, Kind: "prefix", Path: "dist",
	})
	if err != nil {
		t.Fatal(err)
	}
	archiveData := append([]byte(nil), archive.Data...)
	for !archive.Complete {
		archive, err = service.DownloadArtifact(context.Background(), application.ArtifactDownloadRequest{Token: archive.Token, Offset: archive.NextOffset})
		if err != nil {
			t.Fatal(err)
		}
		archiveData = append(archiveData, archive.Data...)
	}
	reader, err := zip.NewReader(bytes.NewReader(archiveData), int64(len(archiveData)))
	if err != nil {
		t.Fatal(err)
	}
	entries := map[string]string{}
	for _, entry := range reader.File {
		file, err := entry.Open()
		if err != nil {
			t.Fatal(err)
		}
		content, err := io.ReadAll(file)
		_ = file.Close()
		if err != nil {
			t.Fatal(err)
		}
		entries[entry.Name] = string(content)
	}
	if len(entries) != 2 || entries["dist/readme.txt"] != "hello" || len(entries["dist/app.bin"]) != artifactDownloadChunkSize+37 {
		t.Fatalf("archive entries = %#v", entries)
	}
}

func TestArtifactDownloadServiceResumesWithoutLiveTokenWhenContentMatches(t *testing.T) {
	artifactsDir := t.TempDir()
	jobID := "job-resume"
	content := bytes.Repeat([]byte("resume-me"), artifactDownloadChunkSize/4)
	artifactPath := filepath.Join(artifactsDir, jobID, "large.bin")
	if err := os.MkdirAll(filepath.Dir(artifactPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(artifactPath, content, 0o644); err != nil {
		t.Fatal(err)
	}
	store := artifactDownloadStoreStub{artifacts: []protocol.JobExecutionArtifact{{Path: "large.bin", SizeBytes: int64(len(content))}}}
	first, err := newArtifactDownloadService(store, artifactsDir).DownloadArtifact(t.Context(), application.ArtifactDownloadRequest{
		JobExecutionID: jobID, Kind: "file", Path: "large.bin",
	})
	if err != nil {
		t.Fatal(err)
	}
	if first.Complete || first.ContentID == "" {
		t.Fatalf("first chunk = %+v", first)
	}

	// A fresh service simulates token loss after expiry or a server restart.
	resumed, err := newArtifactDownloadService(store, artifactsDir).DownloadArtifact(t.Context(), application.ArtifactDownloadRequest{
		JobExecutionID: jobID, Kind: "file", Path: "large.bin", Offset: first.NextOffset, ExpectedContentID: first.ContentID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if resumed.ContentID != first.ContentID || resumed.NextOffset <= first.NextOffset || !bytes.Equal(resumed.Data, content[first.NextOffset:resumed.NextOffset]) {
		t.Fatalf("resumed chunk = %+v", resumed)
	}
}

func TestArtifactDownloadServiceRejectsResumeWhenContentChanged(t *testing.T) {
	artifactsDir := t.TempDir()
	jobID := "job-changed"
	artifactPath := filepath.Join(artifactsDir, jobID, "large.bin")
	if err := os.MkdirAll(filepath.Dir(artifactPath), 0o755); err != nil {
		t.Fatal(err)
	}
	content := bytes.Repeat([]byte("a"), artifactDownloadChunkSize+1)
	if err := os.WriteFile(artifactPath, content, 0o644); err != nil {
		t.Fatal(err)
	}
	store := artifactDownloadStoreStub{artifacts: []protocol.JobExecutionArtifact{{Path: "large.bin", SizeBytes: int64(len(content))}}}
	first, err := newArtifactDownloadService(store, artifactsDir).DownloadArtifact(t.Context(), application.ArtifactDownloadRequest{
		JobExecutionID: jobID, Kind: "file", Path: "large.bin",
	})
	if err != nil {
		t.Fatal(err)
	}
	content[len(content)-1] = 'b'
	if err := os.WriteFile(artifactPath, content, 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = newArtifactDownloadService(store, artifactsDir).DownloadArtifact(t.Context(), application.ArtifactDownloadRequest{
		JobExecutionID: jobID, Kind: "file", Path: "large.bin", Offset: first.NextOffset, ExpectedContentID: first.ContentID,
	})
	if application.ErrorKindOf(err) != application.ErrorFailedPrecondition {
		t.Fatalf("resume error = %v", err)
	}
}

func TestArtifactDownloadServiceExpiredTokenCanResumeAndMissingSourceCannot(t *testing.T) {
	artifactsDir := t.TempDir()
	jobID := "job-expired"
	content := bytes.Repeat([]byte("x"), artifactDownloadChunkSize+1)
	artifactPath := filepath.Join(artifactsDir, jobID, "large.bin")
	if err := os.MkdirAll(filepath.Dir(artifactPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(artifactPath, content, 0o644); err != nil {
		t.Fatal(err)
	}
	store := artifactDownloadStoreStub{artifacts: []protocol.JobExecutionArtifact{{Path: "large.bin", SizeBytes: int64(len(content))}}}
	service := newArtifactDownloadService(store, artifactsDir)
	now := time.Now()
	service.now = func() time.Time { return now }
	first, err := service.DownloadArtifact(t.Context(), application.ArtifactDownloadRequest{JobExecutionID: jobID, Kind: "file", Path: "large.bin"})
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(artifactDownloadTTL + time.Second)
	if _, err := service.DownloadArtifact(t.Context(), application.ArtifactDownloadRequest{Token: first.Token, Offset: first.NextOffset}); application.ErrorKindOf(err) != application.ErrorNotFound {
		t.Fatalf("expired token error = %v", err)
	}
	resumed, err := service.DownloadArtifact(t.Context(), application.ArtifactDownloadRequest{
		JobExecutionID: jobID, Kind: "file", Path: "large.bin", Offset: first.NextOffset, ExpectedContentID: first.ContentID,
	})
	if err != nil || !resumed.Complete {
		t.Fatalf("tokenless resume = %+v, %v", resumed, err)
	}
	if err := os.Remove(artifactPath); err != nil {
		t.Fatal(err)
	}
	_, err = newArtifactDownloadService(store, artifactsDir).DownloadArtifact(t.Context(), application.ArtifactDownloadRequest{
		JobExecutionID: jobID, Kind: "file", Path: "large.bin", Offset: first.NextOffset, ExpectedContentID: first.ContentID,
	})
	if application.ErrorKindOf(err) != application.ErrorNotFound {
		t.Fatalf("deleted source error = %v", err)
	}
}

func TestArtifactDownloadServiceReturnsIdentityForEmptyFile(t *testing.T) {
	artifactsDir := t.TempDir()
	jobID := "job-empty"
	artifactPath := filepath.Join(artifactsDir, jobID, "empty.txt")
	if err := os.MkdirAll(filepath.Dir(artifactPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(artifactPath, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := newArtifactDownloadService(artifactDownloadStoreStub{artifacts: []protocol.JobExecutionArtifact{{Path: "empty.txt"}}}, artifactsDir).DownloadArtifact(
		t.Context(), application.ArtifactDownloadRequest{JobExecutionID: jobID, Kind: "file", Path: "empty.txt"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Complete || result.NextOffset != 0 || result.TotalSize != 0 || result.ContentID != testArtifactContentID(nil) {
		t.Fatalf("empty download = %+v", result)
	}
}

func TestArtifactDownloadRebuiltArchiveAndLogHaveStableIdentity(t *testing.T) {
	artifactsDir := t.TempDir()
	jobID := "job-stable"
	artifactPath := filepath.Join(artifactsDir, jobID, "out.txt")
	if err := os.MkdirAll(filepath.Dir(artifactPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(artifactPath, []byte("stable"), 0o644); err != nil {
		t.Fatal(err)
	}
	store := artifactDownloadStoreStub{
		artifacts: []protocol.JobExecutionArtifact{{Path: "out.txt", SizeBytes: 6}},
		job:       protocol.JobExecution{ID: jobID, Status: protocol.JobExecutionStatusSucceeded},
		events:    []protocol.JobExecutionEvent{{Type: protocol.JobExecutionEventTypeSystemMessage, Message: "done"}},
	}
	for _, kind := range []string{"all", "log-clean", "log-raw"} {
		first, err := newArtifactDownloadService(store, artifactsDir).DownloadArtifact(t.Context(), application.ArtifactDownloadRequest{JobExecutionID: jobID, Kind: kind})
		if err != nil {
			t.Fatalf("%s first: %v", kind, err)
		}
		second, err := newArtifactDownloadService(store, artifactsDir).DownloadArtifact(t.Context(), application.ArtifactDownloadRequest{JobExecutionID: jobID, Kind: kind, ExpectedContentID: first.ContentID})
		if err != nil {
			t.Fatalf("%s rebuild: %v", kind, err)
		}
		if first.ContentID == "" || second.ContentID != first.ContentID {
			t.Fatalf("%s content ids = %q, %q", kind, first.ContentID, second.ContentID)
		}
	}
}

func testArtifactContentID(content []byte) string {
	hash := sha256.Sum256(content)
	return "sha256:" + hex.EncodeToString(hash[:])
}

func TestArtifactDownloadServiceStreamsCleanAndRawJobLogs(t *testing.T) {
	store := artifactDownloadStoreStub{
		job: protocol.JobExecution{ID: "job-1", Status: protocol.JobExecutionStatusFailed},
		events: []protocol.JobExecutionEvent{
			{Type: protocol.JobExecutionEventTypeSystemMessage, Message: "\x1b[31mfailed\x1b[0m"},
		},
	}
	service := newArtifactDownloadService(store, t.TempDir())
	clean, err := service.DownloadArtifact(context.Background(), application.ArtifactDownloadRequest{JobExecutionID: "job-1", Kind: "log-clean"})
	if err != nil {
		t.Fatal(err)
	}
	if clean.FileName != "ciwi-job-1-clean.log" || !clean.Complete || bytes.Contains(clean.Data, []byte("\x1b[31m")) {
		t.Fatalf("clean log chunk = %#v", clean)
	}
	raw, err := service.DownloadArtifact(context.Background(), application.ArtifactDownloadRequest{JobExecutionID: "job-1", Kind: "log-raw"})
	if err != nil {
		t.Fatal(err)
	}
	if raw.FileName != "ciwi-job-1-raw.log" || !raw.Complete || !bytes.Contains(raw.Data, []byte("\x1b[31mfailed")) {
		t.Fatalf("raw log chunk = %#v", raw)
	}
}

func TestArtifactDownloadServiceCancellationRemovesTemporaryArchive(t *testing.T) {
	artifactsDir := t.TempDir()
	jobID := "job-cancel"
	artifactPath := filepath.Join(artifactsDir, jobID, "dist", "app.bin")
	if err := os.MkdirAll(filepath.Dir(artifactPath), 0o755); err != nil {
		t.Fatal(err)
	}
	content := make([]byte, artifactDownloadChunkSize+1)
	if _, err := rand.Read(content); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(artifactPath, content, 0o644); err != nil {
		t.Fatal(err)
	}
	service := newArtifactDownloadService(artifactDownloadStoreStub{artifacts: []protocol.JobExecutionArtifact{{Path: "dist/app.bin", SizeBytes: artifactDownloadChunkSize + 1}}}, artifactsDir)
	first, err := service.DownloadArtifact(context.Background(), application.ArtifactDownloadRequest{JobExecutionID: jobID, Kind: "all"})
	if err != nil {
		t.Fatal(err)
	}
	session, ok := service.sessions[first.Token]
	if !ok || !session.temporary {
		t.Fatalf("temporary session not retained: %+v", service.sessions)
	}
	if _, err := os.Stat(session.path); err != nil {
		t.Fatalf("temporary archive missing before cancellation: %v", err)
	}
	result, err := service.DownloadArtifact(context.Background(), application.ArtifactDownloadRequest{Token: first.Token, Cancel: true})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Complete {
		t.Fatalf("cancellation result = %+v", result)
	}
	if _, ok := service.sessions[first.Token]; ok {
		t.Fatal("cancelled session was retained")
	}
	if _, err := os.Stat(session.path); !os.IsNotExist(err) {
		t.Fatalf("temporary archive survived cancellation: %v", err)
	}
}

func TestArtifactDownloadPreparationDoesNotHoldSessionMutex(t *testing.T) {
	artifactsDir := t.TempDir()
	jobID := "job-concurrent"
	artifactPath := filepath.Join(artifactsDir, jobID, "large.bin")
	if err := os.MkdirAll(filepath.Dir(artifactPath), 0o755); err != nil {
		t.Fatal(err)
	}
	content := bytes.Repeat([]byte("x"), artifactDownloadChunkSize+1)
	if err := os.WriteFile(artifactPath, content, 0o644); err != nil {
		t.Fatal(err)
	}
	store := &blockingArtifactDownloadStore{
		artifactDownloadStoreStub: artifactDownloadStoreStub{artifacts: []protocol.JobExecutionArtifact{{Path: "large.bin", SizeBytes: int64(len(content))}}},
		entered:                   make(chan struct{}, 1), release: make(chan struct{}),
	}
	service := newArtifactDownloadService(store, artifactsDir)
	first, err := service.DownloadArtifact(t.Context(), application.ArtifactDownloadRequest{JobExecutionID: jobID, Kind: "file", Path: "large.bin"})
	if err != nil {
		t.Fatal(err)
	}
	prepared := make(chan error, 1)
	go func() {
		_, err := service.DownloadArtifact(t.Context(), application.ArtifactDownloadRequest{JobExecutionID: jobID, Kind: "file", Path: "large.bin"})
		prepared <- err
	}()
	select {
	case <-store.entered:
	case <-time.After(time.Second):
		t.Fatal("second preparation did not reach the blocking store")
	}
	cancelled := make(chan error, 1)
	go func() {
		_, err := service.DownloadArtifact(t.Context(), application.ArtifactDownloadRequest{Token: first.Token, Cancel: true})
		cancelled <- err
	}()
	select {
	case err := <-cancelled:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("session mutex was held during artifact preparation")
	}
	close(store.release)
	if err := <-prepared; err != nil {
		t.Fatal(err)
	}
}
