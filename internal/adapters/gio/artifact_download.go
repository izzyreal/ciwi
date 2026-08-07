//go:build darwin || ios || linux || windows

package gio

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"gioui.org/x/explorer"
	cnpv1 "github.com/izzyreal/ciwi/pkg/cnp/v1"
	"github.com/izzyreal/ciwi/pkg/cnpclient"
)

var errArtifactDownloadCancelled = errors.New("artifact download cancelled")

type artifactDownloadResult struct {
	path string
	err  error
}

type artifactChunkClient interface {
	DownloadArtifactChunk(context.Context, *cnpv1.ArtifactDownloadRequest) (*cnpv1.ArtifactDownloadChunk, error)
}

type artifactDestinationPicker interface {
	CreateFile(string) (io.WriteCloser, error)
}

func downloadArtifact(ctx context.Context, client *cnpclient.Client, picker artifactDestinationPicker, arguments map[string]string) (string, error) {
	return downloadArtifactWithPicker(ctx, client, picker, arguments)
}

func downloadArtifactWithPicker(ctx context.Context, client artifactChunkClient, picker artifactDestinationPicker, arguments map[string]string) (resultName string, resultErr error) {
	if client == nil {
		return "", fmt.Errorf("server is offline")
	}
	if picker == nil {
		return "", fmt.Errorf("native save dialog is unavailable")
	}
	request := &cnpv1.ArtifactDownloadRequest{
		JobExecutionId: strings.TrimSpace(arguments["jobExecutionId"]),
		Kind:           strings.TrimSpace(arguments["kind"]),
		Path:           strings.TrimSpace(arguments["path"]),
	}
	if request.JobExecutionId == "" {
		return "", fmt.Errorf("job execution id is required")
	}

	chunk, err := client.DownloadArtifactChunk(ctx, request)
	if err != nil {
		return "", fmt.Errorf("download artifact: %w", err)
	}
	fileName := safeDownloadFileName(chunk.GetFileName())
	activeToken := chunk.GetToken()
	writer, err := picker.CreateFile(fileName)
	if err != nil {
		if !chunk.GetComplete() {
			cancelArtifactDownload(client, chunk.GetToken())
		}
		if errors.Is(err, explorer.ErrUserDecline) {
			return "", errArtifactDownloadCancelled
		}
		return "", fmt.Errorf("choose artifact destination: %w", err)
	}
	closed := false
	complete := chunk.GetComplete()
	defer func() {
		if !closed {
			if closeErr := writer.Close(); resultErr == nil && closeErr != nil {
				resultErr = fmt.Errorf("close artifact destination: %w", closeErr)
			}
		}
		if resultErr != nil && !complete {
			cancelArtifactDownload(client, activeToken)
		}
	}()

	for {
		if len(chunk.GetData()) > 0 {
			if _, err := writer.Write(chunk.GetData()); err != nil {
				return "", fmt.Errorf("write artifact: %w", err)
			}
		}
		complete = chunk.GetComplete()
		if complete {
			break
		}
		if chunk.GetToken() == "" || chunk.GetNextOffset() <= request.GetOffset() {
			return "", fmt.Errorf("server returned invalid artifact download progress")
		}
		request.Token = chunk.GetToken()
		request.Offset = chunk.GetNextOffset()
		chunk, err = client.DownloadArtifactChunk(ctx, request)
		if err != nil {
			return "", fmt.Errorf("download artifact: %w", err)
		}
		activeToken = chunk.GetToken()
	}
	if err := writer.Close(); err != nil {
		return "", fmt.Errorf("close artifact destination: %w", err)
	}
	closed = true
	return fileName, nil
}

func cancelArtifactDownload(client artifactChunkClient, token string) {
	token = strings.TrimSpace(token)
	if client == nil || token == "" {
		return
	}
	cancelCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, _ = client.DownloadArtifactChunk(cancelCtx, &cnpv1.ArtifactDownloadRequest{Token: token, Cancel: true})
}

func safeDownloadFileName(raw string) string {
	name := filepath.Base(strings.ReplaceAll(strings.TrimSpace(raw), "\\", "/"))
	if name == "" || name == "." || name == string(filepath.Separator) {
		return "ciwi-artifact"
	}
	return name
}
