package server

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/rand"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/izzyreal/ciwi/internal/application"
	"github.com/izzyreal/ciwi/internal/protocol"
)

type artifactDownloadStoreStub struct {
	artifacts []protocol.JobExecutionArtifact
	job       protocol.JobExecution
	events    []protocol.JobExecutionEvent
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
	if first.Complete || len(first.Data) != artifactDownloadChunkSize || first.NextOffset != artifactDownloadChunkSize || first.Token == "" {
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
