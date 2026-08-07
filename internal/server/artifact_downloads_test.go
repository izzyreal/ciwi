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
}

func (s artifactDownloadStoreStub) ListJobExecutionArtifacts(string) ([]protocol.JobExecutionArtifact, error) {
	return append([]protocol.JobExecutionArtifact(nil), s.artifacts...), nil
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
