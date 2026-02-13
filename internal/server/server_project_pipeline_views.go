package server

import (
	"strings"

	"github.com/izzyreal/ciwi/internal/protocol"
)

type projectDetailViewResponse struct {
	Project protocol.ProjectDetail `json:"project"`
}

type projectListViewResponse struct {
	Projects []protocol.ProjectSummary `json:"projects"`
}

type pipelineVersionPreviewErrorResponse struct {
	OK      bool   `json:"ok"`
	Message string `json:"message"`
}

type pipelineVersionPreviewSuccessResponse struct {
	OK                 bool   `json:"ok"`
	PipelineVersion    string `json:"pipeline_version"`
	PipelineVersionRaw string `json:"pipeline_version_raw"`
	SourceRefResolved  string `json:"source_ref_resolved"`
	VersionFile        string `json:"version_file"`
	TagPrefix          string `json:"tag_prefix"`
	AutoBump           string `json:"auto_bump"`
}

func buildPipelineVersionPreviewErrorResponse(message string) pipelineVersionPreviewErrorResponse {
	return pipelineVersionPreviewErrorResponse{
		OK:      false,
		Message: message,
	}
}

func buildPipelineVersionPreviewSuccessResponse(runCtx pipelineRunContext) pipelineVersionPreviewSuccessResponse {
	return pipelineVersionPreviewSuccessResponse{
		OK:                 true,
		PipelineVersion:    strings.TrimSpace(runCtx.Version),
		PipelineVersionRaw: strings.TrimSpace(runCtx.VersionRaw),
		SourceRefResolved:  strings.TrimSpace(runCtx.SourceRefResolved),
		VersionFile:        strings.TrimSpace(runCtx.VersionFile),
		TagPrefix:          strings.TrimSpace(runCtx.TagPrefix),
		AutoBump:           strings.TrimSpace(runCtx.AutoBump),
	}
}
