package domain

import (
	"strconv"
	"strings"
)

// ExecutionMetadata retains the map representation used by SQLite and wire
// protocols while centralizing the semantics of ciwi-owned keys.
type ExecutionMetadata map[string]string

const (
	ExecutionMetadataProject                   = "project"
	ExecutionMetadataProjectID                 = "project_id"
	ExecutionMetadataPipelineID                = "pipeline_id"
	ExecutionMetadataPipelineJobID             = "pipeline_job_id"
	ExecutionMetadataPipelineRunID             = "pipeline_run_id"
	ExecutionMetadataPipelineJobIndex          = "pipeline_job_index"
	ExecutionMetadataPipelineChainName         = "pipeline_chain_name"
	ExecutionMetadataPipelineChainID           = "pipeline_chain_id"
	ExecutionMetadataPipelineChainIndex        = "pipeline_chain_index"
	ExecutionMetadataPipelineChainPosition     = "pipeline_chain_position"
	ExecutionMetadataPipelineChainTotal        = "pipeline_chain_total"
	ExecutionMetadataChainRunID                = "chain_run_id"
	ExecutionMetadataChainBlocked              = "chain_blocked"
	ExecutionMetadataChainCancelled            = "chain_cancelled"
	ExecutionMetadataChainDependsOnPipelines   = "chain_depends_on_pipelines"
	ExecutionMetadataDependencyBlocked         = "dependency_blocked"
	ExecutionMetadataNeedsBlocked              = "needs_blocked"
	ExecutionMetadataNeedsJobIDs               = "needs_job_ids"
	ExecutionMetadataMissingNeedsJobIDs        = "missing_needs_job_ids"
	ExecutionMetadataMatrixName                = "matrix_name"
	ExecutionMetadataMatrixIndex               = "matrix_index"
	ExecutionMetadataMatrixVariablePrefix      = "matrix_var."
	ExecutionMetadataDryRun                    = "dry_run"
	ExecutionMetadataBuildTarget               = "build_target"
	ExecutionMetadataBuildVersion              = "build_version"
	ExecutionMetadataPipelineVersion           = "pipeline_version"
	ExecutionMetadataPipelineVersionRaw        = "pipeline_version_raw"
	ExecutionMetadataPipelineSourceRepo        = "pipeline_source_repo"
	ExecutionMetadataPipelineSourceRef         = "pipeline_source_ref"
	ExecutionMetadataPipelineSourceRefRaw      = "pipeline_source_ref_raw"
	ExecutionMetadataPipelineSourceRefResolved = "pipeline_source_ref_resolved"
	ExecutionMetadataAdhoc                     = "adhoc"
	ExecutionMetadataAdhocAgentID              = "adhoc_agent_id"
	ExecutionMetadataAdhocShell                = "adhoc_shell"
	ExecutionMetadataHasSecrets                = "has_secrets"
	ExecutionMetadataNextVersion               = "next_version"
	ExecutionMetadataAutoBumpBranch            = "auto_bump_branch"
	ExecutionMetadataVersion                   = "version"
	ExecutionMetadataTag                       = "tag"
	ExecutionMetadataArtifacts                 = "artifacts"
	ExecutionMetadataArtifactSourcesJSON       = "artifact_sources_json"
	ExecutionMetadataRuntimeContainerImage     = "runtime_probe.container_image"
	ExecutionMetadataRuntimeContainerWorkdir   = "runtime_exec.container_workdir"
	ExecutionMetadataRuntimeContainerUser      = "runtime_exec.container_user"
	ExecutionMetadataRuntimeContainerDevices   = "runtime_exec.container_devices"
	ExecutionMetadataRuntimeContainerGroups    = "runtime_exec.container_groups"
	ExecutionMetadataAttemptRootJobID          = "attempt_root_job_id"
	ExecutionMetadataRerunOfJobID              = "rerun_of_job_id"
	ExecutionMetadataSchedulingBlocked         = "scheduling_blocked"
	ExecutionMetadataSchedulingBlockedReason   = "scheduling_blocked_reason"
	ExecutionMetadataSchedulingRetryUTC        = "scheduling_retry_utc"
)

func (m ExecutionMetadata) Value(key string) string {
	return strings.TrimSpace(m[key])
}

func (m ExecutionMetadata) Flag(key string) bool {
	return m.Value(key) == "1"
}

func (m ExecutionMetadata) Int64(key string) (int64, bool) {
	value := m.Value(key)
	if value == "" {
		return 0, false
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	return parsed, err == nil
}

func (m ExecutionMetadata) CSV(key string) []string {
	raw := m.Value(key)
	if raw == "" {
		return nil
	}
	values := make([]string, 0)
	for _, item := range strings.Split(raw, ",") {
		if value := strings.TrimSpace(item); value != "" {
			values = append(values, value)
		}
	}
	return values
}

func (m ExecutionMetadata) Set(key, value string) {
	if m != nil {
		m[key] = value
	}
}

func (m ExecutionMetadata) SetFlag(key string, enabled bool) {
	if enabled {
		m.Set(key, "1")
	} else {
		m.Set(key, "")
	}
}

func (m ExecutionMetadata) Clone() ExecutionMetadata {
	if m == nil {
		return nil
	}
	clone := make(ExecutionMetadata, len(m))
	for key, value := range m {
		clone[key] = value
	}
	return clone
}
