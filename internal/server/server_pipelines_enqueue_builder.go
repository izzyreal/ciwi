package server

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/izzyreal/ciwi/internal/config"
	"github.com/izzyreal/ciwi/internal/protocol"
	"github.com/izzyreal/ciwi/internal/store"
)

func (s *stateStore) buildPendingPipelineJobs(
	p store.PersistedPipeline,
	selection *protocol.RunPipelineSelectionRequest,
	opts enqueuePipelineOptions,
	runCtx pipelineRunContext,
	depCtx pipelineDependencyContext,
	runID string,
) ([]pendingJob, error) {
	pending := make([]pendingJob, 0)
	sortedJobs := p.SortedJobs()
	selectedJobIDs := map[string]bool{}
	for _, pj := range sortedJobs {
		if selection != nil && strings.TrimSpace(selection.PipelineJobID) != "" && selection.PipelineJobID != pj.ID {
			continue
		}
		selectedJobIDs[pj.ID] = true
	}
	for _, pj := range sortedJobs {
		if selection != nil && strings.TrimSpace(selection.PipelineJobID) != "" && selection.PipelineJobID != pj.ID {
			continue
		}
		for _, need := range pj.Needs {
			need = strings.TrimSpace(need)
			if need == "" {
				continue
			}
			if !selectedJobIDs[need] {
				return nil, fmt.Errorf("selection excludes required job %q needed by %q", need, pj.ID)
			}
		}
		needs := normalizePipelineJobNeeds(pj.Needs)
		if len(pj.Steps) == 0 {
			return nil, fmt.Errorf("pipeline job %q has no steps", pj.ID)
		}
		originalMatrixEntries := pj.MatrixInclude
		matrixEntries := originalMatrixEntries
		if len(matrixEntries) == 0 {
			matrixEntries = []map[string]string{{}}
		}
		for index, vars := range matrixEntries {
			if selection != nil {
				if selection.MatrixIndex != nil && *selection.MatrixIndex != index {
					continue
				}
				if strings.TrimSpace(selection.MatrixName) != "" && vars["name"] != selection.MatrixName {
					continue
				}
			}
			spec, err := s.buildPendingPipelineJobMatrixEntry(
				p,
				pj.ID,
				pj.Steps,
				pj.RunsOn,
				pj.RequiresTools,
				pj.RequiresContainerTools,
				pj.TimeoutSeconds,
				pj.Artifacts,
				pj.Caches,
				p.DependsOn,
				index,
				vars,
				originalMatrixEntries,
				needs,
				selection,
				opts,
				runCtx,
				depCtx,
				runID,
			)
			if err != nil {
				return nil, err
			}
			if spec == nil {
				continue
			}
			pending = append(pending, *spec)
		}
	}
	return pending, nil
}

func (s *stateStore) buildPendingPipelineJobMatrixEntry(
	p store.PersistedPipeline,
	pipelineJobID string,
	steps []config.PipelineJobStep,
	runsOn map[string]string,
	requiresTools map[string]string,
	requiresContainerTools map[string]string,
	timeoutSeconds int,
	artifacts []string,
	caches []config.PipelineJobCacheSpec,
	pipelineDependsOn []string,
	matrixIndex int,
	matrixVars map[string]string,
	originalMatrixEntries []map[string]string,
	needs []string,
	selection *protocol.RunPipelineSelectionRequest,
	opts enqueuePipelineOptions,
	runCtx pipelineRunContext,
	depCtx pipelineDependencyContext,
	runID string,
) (*pendingJob, error) {
	renderVars := cloneMap(matrixVars)
	if renderVars == nil {
		renderVars = map[string]string{}
	}
	if runCtx.VersionRaw != "" {
		renderVars["ciwi.version_raw"] = runCtx.VersionRaw
	}
	if runCtx.Version != "" {
		renderVars["ciwi.version"] = runCtx.Version
	}
	if runCtx.TagPrefix != "" {
		renderVars["ciwi.tag_prefix"] = runCtx.TagPrefix
	}
	rendered := make([]string, 0, len(steps))
	stepPlan := make([]protocol.JobStepPlanItem, 0, len(steps))
	env := make(map[string]string)
	for idx, step := range steps {
		if selection != nil && selection.DryRun && step.SkipDryRun {
			stepPlan = append(stepPlan, protocol.JobStepPlanItem{
				Name: describeSkippedPipelineStepLiteral(step, idx, pipelineJobID),
				Kind: "dryrun_skip",
			})
			continue
		}
		if step.Test != nil {
			for k, v := range step.Env {
				env[k] = renderTemplate(v, renderVars)
			}
			command := renderTemplate(step.Test.Command, renderVars)
			if strings.TrimSpace(command) == "" {
				continue
			}
			name := strings.TrimSpace(step.Test.Name)
			if name == "" {
				name = fmt.Sprintf("%s-test-%d", pipelineJobID, len(stepPlan)+1)
			}
			format := strings.TrimSpace(step.Test.Format)
			if format == "" {
				format = "go-test-json"
			}
			rendered = append(rendered, command)
			stepPlan = append(stepPlan, protocol.JobStepPlanItem{
				Name:           "test " + name,
				Script:         command,
				Kind:           "test",
				TestName:       strings.TrimSpace(name),
				TestFormat:     strings.TrimSpace(format),
				TestReport:     strings.TrimSpace(step.Test.Report),
				CoverageFormat: strings.TrimSpace(step.Test.CoverageFormat),
				CoverageReport: strings.TrimSpace(step.Test.CoverageReport),
			})
			continue
		}
		for k, v := range step.Env {
			env[k] = renderTemplate(v, renderVars)
		}
		line := renderTemplate(step.Run, renderVars)
		if strings.TrimSpace(line) == "" {
			continue
		}
		rendered = append(rendered, line)
		stepPlan = append(stepPlan, protocol.JobStepPlanItem{
			Name:   describePipelineStep(step, idx, pipelineJobID),
			Script: line,
		})
	}
	if len(stepPlan) == 0 {
		return nil, fmt.Errorf("pipeline job %q has no executable steps after rendering", pipelineJobID)
	}
	for stepIndex := range stepPlan {
		stepPlan[stepIndex].Index = stepIndex + 1
		stepPlan[stepIndex].Total = len(stepPlan)
		if strings.TrimSpace(stepPlan[stepIndex].Name) == "" {
			stepPlan[stepIndex].Name = fmt.Sprintf("step %d", stepIndex+1)
		}
	}
	metadata := map[string]string{
		"project":            p.ProjectName,
		"project_id":         strconv.FormatInt(p.ProjectID, 10),
		"pipeline_id":        p.PipelineID,
		"pipeline_run_id":    runID,
		"pipeline_job_id":    pipelineJobID,
		"pipeline_job_index": strconv.Itoa(matrixIndex),
	}
	if len(originalMatrixEntries) > 0 {
		metadata["matrix_index"] = strconv.Itoa(matrixIndex)
	}
	if selection != nil && selection.DryRun {
		metadata["dry_run"] = "1"
	}
	if name := matrixVars["name"]; name != "" {
		metadata["matrix_name"] = name
		metadata["build_target"] = name
	}
	if runCtx.VersionRaw != "" {
		metadata["pipeline_version_raw"] = runCtx.VersionRaw
	}
	if runCtx.Version != "" {
		metadata["pipeline_version"] = runCtx.Version
		metadata["build_version"] = runCtx.Version
	}
	if runCtx.SourceRefResolved != "" {
		metadata["pipeline_source_ref_resolved"] = runCtx.SourceRefResolved
	}
	metadata["pipeline_source_repo"] = p.SourceRepo
	for k, v := range opts.metaPatch {
		if strings.TrimSpace(k) == "" {
			continue
		}
		metadata[k] = strings.TrimSpace(v)
	}
	if opts.blocked {
		metadata["chain_blocked"] = "1"
	}
	if len(needs) > 0 {
		metadata["needs_job_ids"] = strings.Join(needs, ",")
		metadata["needs_blocked"] = "1"
	}
	if selection != nil && selection.DryRun {
		env["CIWI_DRY_RUN"] = "1"
	}
	if runCtx.VersionRaw != "" {
		env["CIWI_PIPELINE_VERSION_RAW"] = runCtx.VersionRaw
	}
	if runCtx.Version != "" {
		env["CIWI_PIPELINE_VERSION"] = runCtx.Version
		env["CIWI_PIPELINE_TAG"] = runCtx.Version
	}
	if runCtx.TagPrefix != "" {
		env["CIWI_PIPELINE_TAG_PREFIX"] = runCtx.TagPrefix
	}
	if runCtx.VersionFile != "" {
		env["CIWI_PIPELINE_VERSION_FILE"] = runCtx.VersionFile
	}
	if runCtx.SourceRefResolved != "" {
		env["CIWI_PIPELINE_SOURCE_REF"] = runCtx.SourceRefResolved
	}
	if strings.TrimSpace(p.SourceRef) != "" {
		env["CIWI_PIPELINE_SOURCE_REF_RAW"] = strings.TrimSpace(p.SourceRef)
	}
	env["CIWI_PIPELINE_SOURCE_REPO"] = p.SourceRepo
	depJobID := resolveDependencyArtifactJobID(pipelineDependsOn, depCtx.ArtifactJobIDs, pipelineJobID, matrixVars)
	if depJobID != "" {
		env["CIWI_DEP_ARTIFACT_JOB_ID"] = depJobID
	}
	if depJobIDs := resolveDependencyArtifactJobIDs(pipelineDependsOn, depCtx.ArtifactJobIDsAll, depJobID); len(depJobIDs) > 0 {
		env["CIWI_DEP_ARTIFACT_JOB_IDS"] = strings.Join(depJobIDs, ",")
	}
	if containerImage := strings.TrimSpace(runsOn["container_image"]); containerImage != "" {
		metadata["runtime_probe.container_image"] = containerImage
	}
	if containerWorkdir := strings.TrimSpace(runsOn["container_workdir"]); containerWorkdir != "" {
		metadata["runtime_exec.container_workdir"] = containerWorkdir
	}
	if containerUser := strings.TrimSpace(runsOn["container_user"]); containerUser != "" {
		metadata["runtime_exec.container_user"] = containerUser
	}
	if containerDevices := strings.TrimSpace(runsOn["container_devices"]); containerDevices != "" {
		metadata["runtime_exec.container_devices"] = containerDevices
	}
	if containerGroups := strings.TrimSpace(runsOn["container_groups"]); containerGroups != "" {
		metadata["runtime_exec.container_groups"] = containerGroups
	}

	requiredCaps := cloneMap(runsOn)
	for k := range requiredCaps {
		if strings.HasPrefix(k, "container_") {
			delete(requiredCaps, k)
		}
	}
	for tool, constraint := range requiresTools {
		tool = strings.TrimSpace(tool)
		if tool == "" {
			continue
		}
		if requiredCaps == nil {
			requiredCaps = map[string]string{}
		}
		requiredCaps["requires.tool."+tool] = strings.TrimSpace(constraint)
	}
	for tool, constraint := range requiresContainerTools {
		tool = strings.TrimSpace(tool)
		if tool == "" {
			continue
		}
		if requiredCaps == nil {
			requiredCaps = map[string]string{}
		}
		requiredCaps["requires.container.tool."+tool] = strings.TrimSpace(constraint)
	}
	sourceRef := p.SourceRef
	if runCtx.SourceRefResolved != "" {
		sourceRef = runCtx.SourceRefResolved
	}
	return &pendingJob{
		pipelineJobID:  pipelineJobID,
		needs:          append([]string(nil), needs...),
		script:         strings.Join(rendered, "\n"),
		env:            cloneMap(env),
		requiredCaps:   requiredCaps,
		timeoutSeconds: timeoutSeconds,
		artifactGlobs:  append([]string(nil), artifacts...),
		caches:         cloneJobCachesFromPersisted(caches),
		sourceRepo:     p.SourceRepo,
		sourceRef:      sourceRef,
		metadata:       metadata,
		stepPlan:       stepPlan,
	}, nil
}
