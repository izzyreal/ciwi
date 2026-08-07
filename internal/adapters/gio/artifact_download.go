//go:build darwin || ios || linux || windows

package gio

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	cnpv1 "github.com/izzyreal/ciwi/pkg/cnp/v1"
	"github.com/izzyreal/ciwi/pkg/cnpclient"
)

type artifactDownloadResult struct {
	path string
	err  error
}

func downloadArtifact(ctx context.Context, client *cnpclient.Client, arguments map[string]string) (resultPath string, resultErr error) {
	userHomePath, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locate Downloads folder: %w", err)
	}
	downloadDir := filepath.Join(userHomePath, "Downloads")
	if err := os.MkdirAll(downloadDir, 0o755); err != nil {
		return "", fmt.Errorf("prepare Downloads folder: %w", err)
	}
	return downloadArtifactToDir(ctx, client, arguments, downloadDir)
}

type artifactChunkClient interface {
	DownloadArtifactChunk(context.Context, *cnpv1.ArtifactDownloadRequest) (*cnpv1.ArtifactDownloadChunk, error)
}

func downloadArtifactToDir(ctx context.Context, client artifactChunkClient, arguments map[string]string, downloadDir string) (resultPath string, resultErr error) {
	if client == nil {
		return "", fmt.Errorf("server is offline")
	}
	request := &cnpv1.ArtifactDownloadRequest{
		JobExecutionId: strings.TrimSpace(arguments["jobExecutionId"]),
		Kind:           strings.TrimSpace(arguments["kind"]),
		Path:           strings.TrimSpace(arguments["path"]),
	}
	if request.JobExecutionId == "" {
		return "", fmt.Errorf("job execution id is required")
	}
	partial, err := os.CreateTemp(downloadDir, ".ciwi-download-*.part")
	if err != nil {
		return "", fmt.Errorf("create partial download: %w", err)
	}
	partialPath := partial.Name()
	defer func() {
		_ = partial.Close()
		if resultErr != nil {
			_ = os.Remove(partialPath)
		}
	}()

	fileName := ""
	for {
		chunk, err := client.DownloadArtifactChunk(ctx, request)
		if err != nil {
			return "", fmt.Errorf("download artifact: %w", err)
		}
		if fileName == "" {
			fileName = safeDownloadFileName(chunk.GetFileName())
		}
		if len(chunk.GetData()) > 0 {
			if _, err := partial.Write(chunk.GetData()); err != nil {
				return "", fmt.Errorf("write artifact: %w", err)
			}
		}
		if chunk.GetComplete() {
			break
		}
		if chunk.GetToken() == "" || chunk.GetNextOffset() <= request.GetOffset() {
			return "", fmt.Errorf("server returned invalid artifact download progress")
		}
		request.Token = chunk.GetToken()
		request.Offset = chunk.GetNextOffset()
	}
	if err := partial.Sync(); err != nil {
		return "", fmt.Errorf("flush artifact: %w", err)
	}
	if err := partial.Close(); err != nil {
		return "", fmt.Errorf("close artifact: %w", err)
	}

	for suffix := 0; ; suffix++ {
		candidate := filepath.Join(downloadDir, downloadFileNameWithSuffix(fileName, suffix))
		if err := os.Link(partialPath, candidate); err == nil {
			if err := os.Remove(partialPath); err != nil {
				return "", fmt.Errorf("finalize artifact: %w", err)
			}
			return candidate, nil
		} else if os.IsExist(err) {
			continue
		} else {
			return "", fmt.Errorf("finalize artifact: %w", err)
		}
	}
}

func safeDownloadFileName(raw string) string {
	name := filepath.Base(strings.ReplaceAll(strings.TrimSpace(raw), "\\", "/"))
	if name == "" || name == "." || name == string(filepath.Separator) {
		return "ciwi-artifact"
	}
	return name
}

func downloadFileNameWithSuffix(name string, suffix int) string {
	if suffix <= 0 {
		return name
	}
	extension := filepath.Ext(name)
	base := strings.TrimSuffix(name, extension)
	return fmt.Sprintf("%s (%d)%s", base, suffix, extension)
}
