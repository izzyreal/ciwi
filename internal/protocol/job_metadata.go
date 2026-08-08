package protocol

import "github.com/izzyreal/ciwi/internal/domain"

const (
	JobMetadataProject       = domain.ExecutionMetadataProject
	JobMetadataProjectID     = domain.ExecutionMetadataProjectID
	JobMetadataPipelineID    = domain.ExecutionMetadataPipelineID
	JobMetadataPipelineJobID = domain.ExecutionMetadataPipelineJobID
	JobMetadataMatrixName    = domain.ExecutionMetadataMatrixName
	JobMetadataDryRun        = domain.ExecutionMetadataDryRun
)

func JobMetadataValue(job JobExecution, key string) string {
	return job.Metadata.Value(key)
}

func IsJobWaitingForPrerequisites(job JobExecution) bool {
	return job.Metadata.Flag(domain.ExecutionMetadataChainBlocked) || job.Metadata.Flag(domain.ExecutionMetadataNeedsBlocked)
}
