package protocol

import "strings"

const (
	JobMetadataProject       = "project"
	JobMetadataProjectID     = "project_id"
	JobMetadataPipelineID    = "pipeline_id"
	JobMetadataPipelineJobID = "pipeline_job_id"
	JobMetadataMatrixName    = "matrix_name"
	JobMetadataDryRun        = "dry_run"
)

func JobMetadataValue(job JobExecution, key string) string {
	return strings.TrimSpace(job.Metadata[key])
}
