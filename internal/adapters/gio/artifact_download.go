//go:build darwin || ios || linux || windows

package gio

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	cnpv1 "github.com/izzyreal/ciwi/pkg/cnp/v1"
)

type artifactChunkClient interface {
	DownloadArtifactChunk(context.Context, *cnpv1.ArtifactDownloadRequest) (*cnpv1.ArtifactDownloadChunk, error)
}

type artifactDestinationPicker interface {
	CreateFile(string) (io.WriteCloser, error)
}

func downloadJobLogText(ctx context.Context, client artifactChunkClient, jobID string) (string, error) {
	request := &cnpv1.ArtifactDownloadRequest{JobExecutionId: strings.TrimSpace(jobID), Kind: "log-clean"}
	if client == nil || request.JobExecutionId == "" {
		return "", fmt.Errorf("job execution log is unavailable")
	}
	var output strings.Builder
	for {
		chunk, err := client.DownloadArtifactChunk(ctx, request)
		if err != nil {
			if request.Token != "" {
				cancelArtifactDownload(ctx, client, request.Token)
			}
			return "", err
		}
		output.Write(chunk.GetData())
		if chunk.GetComplete() {
			return output.String(), nil
		}
		if chunk.GetToken() == "" || chunk.GetNextOffset() <= request.GetOffset() {
			cancelArtifactDownload(ctx, client, chunk.GetToken())
			return "", fmt.Errorf("server returned invalid log download progress")
		}
		request.Token, request.Offset = chunk.GetToken(), chunk.GetNextOffset()
	}
}

func cancelArtifactDownload(ctx context.Context, client artifactChunkClient, token string) {
	token = strings.TrimSpace(token)
	if client == nil || token == "" || ctx == nil || ctx.Err() != nil {
		return
	}
	cancelCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
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
