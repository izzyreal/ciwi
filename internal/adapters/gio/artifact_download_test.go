//go:build darwin || ios || linux || windows

package gio

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	cnpv1 "github.com/izzyreal/ciwi/pkg/cnp/v1"
)

type artifactChunkClientStub struct {
	calls int
}

func (s *artifactChunkClientStub) DownloadArtifactChunk(_ context.Context, request *cnpv1.ArtifactDownloadRequest) (*cnpv1.ArtifactDownloadChunk, error) {
	s.calls++
	if s.calls == 1 {
		return &cnpv1.ArtifactDownloadChunk{Token: "token", FileName: "../app.zip", Data: []byte("first-"), NextOffset: 6, TotalSize: 12}, nil
	}
	if request.GetToken() != "token" || request.GetOffset() != 6 {
		return nil, context.Canceled
	}
	return &cnpv1.ArtifactDownloadChunk{Token: "token", FileName: "../app.zip", Data: []byte("second"), NextOffset: 12, TotalSize: 12, Complete: true}, nil
}

func TestDownloadArtifactToDirStreamsWithoutOverwriting(t *testing.T) {
	downloadDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(downloadDir, "app.zip"), []byte("existing"), 0o644); err != nil {
		t.Fatal(err)
	}
	client := &artifactChunkClientStub{}
	path, err := downloadArtifactToDir(t.Context(), client, map[string]string{
		"jobExecutionId": "job-1", "kind": "file", "path": "dist/app.zip",
	}, downloadDir)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(path) != "app (1).zip" {
		t.Fatalf("download path = %q", path)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "first-second" || client.calls != 2 {
		t.Fatalf("download = %q, calls = %d", content, client.calls)
	}
	existing, err := os.ReadFile(filepath.Join(downloadDir, "app.zip"))
	if err != nil || string(existing) != "existing" {
		t.Fatalf("existing download was overwritten: %q, %v", existing, err)
	}
}
