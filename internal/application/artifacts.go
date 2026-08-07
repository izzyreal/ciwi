package application

import "context"

type ArtifactDownloadRequest struct {
	JobExecutionID string
	Kind           string
	Path           string
	Token          string
	Offset         int64
	Cancel         bool
}

type ArtifactDownloadChunk struct {
	Token       string
	FileName    string
	ContentType string
	Data        []byte
	NextOffset  int64
	TotalSize   int64
	Complete    bool
}

type ArtifactDownloadService interface {
	DownloadArtifact(context.Context, ArtifactDownloadRequest) (ArtifactDownloadChunk, error)
}
