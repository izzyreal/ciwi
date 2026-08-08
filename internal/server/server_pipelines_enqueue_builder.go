package server

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/izzyreal/ciwi/internal/config"
	"github.com/izzyreal/ciwi/internal/domain"
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
	missingNeedsByJobID := map[string][]string{}
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
				if !opts.allowSelectionNeedsGap {
					return nil, fmt.Errorf("selection excludes required job %q needed by %q", need, pj.ID)
				}
				missingNeedsByJobID[pj.ID] = append(missingNeedsByJobID[pj.ID], need)
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
				pj.ArtifactSources,
				pj.Caches,
				index,
				vars,
				originalMatrixEntries,
				needs,
				missingNeedsByJobID[pj.ID],
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
	artifactSources []config.PipelineJobArtifactSource,
	caches []config.PipelineJobCacheSpec,
	matrixIndex int,
	matrixVars map[string]string,
	originalMatrixEntries []map[string]string,
	needs []string,
	missingNeeds []string,
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
		stepEnv := map[string]string{}
		for k, v := range step.Env {
			stepEnv[k] = renderTemplate(v, renderVars)
		}
		stepVaultConnection := ""
		stepVaultSecrets := make([]protocol.ProjectSecretSpec, 0)
		if step.Vault != nil {
			stepVaultConnection = strings.TrimSpace(step.Vault.Connection)
			for _, sec := range step.Vault.Secrets {
				stepVaultSecrets = append(stepVaultSecrets, protocol.ProjectSecretSpec{
					Name:      strings.TrimSpace(sec.Name),
					Mount:     strings.TrimSpace(sec.Mount),
					Path:      strings.TrimSpace(sec.Path),
					Key:       strings.TrimSpace(sec.Key),
					KVVersion: sec.KVVersion,
				})
			}
		}
		if selection != nil && selection.DryRun && step.SkipDryRun {
			stepPlan = append(stepPlan, protocol.JobStepPlanItem{
				Name:            describePipelineStep(step, idx, pipelineJobID),
				YAMLLiteral:     pipelineStepYAMLLiteral(step),
				Kind:            "dryrun_skip",
				Env:             stepEnv,
				VaultConnection: stepVaultConnection,
				VaultSecrets:    stepVaultSecrets,
			})
			continue
		}
		if step.Test != nil {
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
			displayName := strings.TrimSpace(step.Name)
			if displayName == "" {
				displayName = "test " + name
			}
			rendered = append(rendered, command)
			stepPlan = append(stepPlan, protocol.JobStepPlanItem{
				Name:            displayName,
				YAMLLiteral:     pipelineStepYAMLLiteral(step),
				Script:          command,
				Kind:            "test",
				Env:             stepEnv,
				VaultConnection: stepVaultConnection,
				VaultSecrets:    stepVaultSecrets,
				TestName:        strings.TrimSpace(name),
				TestFormat:      strings.TrimSpace(format),
				TestReport:      strings.TrimSpace(step.Test.Report),
				CoverageFormat:  strings.TrimSpace(step.Test.CoverageFormat),
				CoverageReport:  strings.TrimSpace(step.Test.CoverageReport),
			})
			continue
		}
		line := renderTemplate(step.Run, renderVars)
		if strings.TrimSpace(line) == "" {
			continue
		}
		rendered = append(rendered, line)
		stepPlan = append(stepPlan, protocol.JobStepPlanItem{
			Name:            describePipelineStep(step, idx, pipelineJobID),
			YAMLLiteral:     pipelineStepYAMLLiteral(step),
			Script:          line,
			Env:             stepEnv,
			VaultConnection: stepVaultConnection,
			VaultSecrets:    stepVaultSecrets,
		})
	}
	if len(stepPlan) == 0 {
		return nil, fmt.Errorf("pipeline job %q has no executable steps after rendering", pipelineJobID)
	}
	if len(rendered) == 0 {
		// In dry-run mode a job may contain only skip_dry_run steps. Persist a harmless
		// placeholder script so queue validation does not reject an empty script.
		rendered = append(rendered, "echo [dry-run] all steps skipped")
	}
	for stepIndex := range stepPlan {
		stepPlan[stepIndex].Index = stepIndex + 1
		stepPlan[stepIndex].Total = len(stepPlan)
		if strings.TrimSpace(stepPlan[stepIndex].Name) == "" {
			stepPlan[stepIndex].Name = fmt.Sprintf("step %d", stepIndex+1)
		}
	}
	metadata := domain.ExecutionMetadata{
		domain.ExecutionMetadataProject:          p.ProjectName,
		domain.ExecutionMetadataProjectID:        strconv.FormatInt(p.ProjectID, 10),
		domain.ExecutionMetadataPipelineID:       p.PipelineID,
		domain.ExecutionMetadataPipelineRunID:    runID,
		domain.ExecutionMetadataPipelineJobID:    pipelineJobID,
		domain.ExecutionMetadataPipelineJobIndex: strconv.Itoa(matrixIndex),
	}
	if len(originalMatrixEntries) > 0 {
		metadata.Set(domain.ExecutionMetadataMatrixIndex, strconv.Itoa(matrixIndex))
	}
	if selection != nil && selection.DryRun {
		metadata.SetFlag(domain.ExecutionMetadataDryRun, true)
	}
	if name := matrixVars["name"]; name != "" {
		metadata.Set(domain.ExecutionMetadataMatrixName, name)
		metadata.Set(domain.ExecutionMetadataBuildTarget, name)
	}
	for key, value := range matrixVars {
		if key = strings.TrimSpace(key); key != "" {
			metadata.Set(domain.ExecutionMetadataMatrixVariablePrefix+key, value)
		}
	}
	artifactSourcesJSON, err := encodeArtifactSources(artifactSources)
	if err != nil {
		return nil, fmt.Errorf("pipeline job %q: %w", pipelineJobID, err)
	}
	if artifactSourcesJSON != "" {
		metadata.Set(domain.ExecutionMetadataArtifactSourcesJSON, artifactSourcesJSON)
	}
	if runCtx.VersionRaw != "" {
		metadata.Set(domain.ExecutionMetadataPipelineVersionRaw, runCtx.VersionRaw)
	}
	if runCtx.Version != "" {
		metadata.Set(domain.ExecutionMetadataPipelineVersion, runCtx.Version)
		metadata.Set(domain.ExecutionMetadataBuildVersion, runCtx.Version)
	}
	if runCtx.SourceRefResolved != "" {
		metadata.Set(domain.ExecutionMetadataPipelineSourceRefResolved, runCtx.SourceRefResolved)
	}
	if strings.TrimSpace(runCtx.SourceRefRaw) != "" {
		metadata.Set(domain.ExecutionMetadataPipelineSourceRefRaw, strings.TrimSpace(runCtx.SourceRefRaw))
	}
	metadata.Set(domain.ExecutionMetadataPipelineSourceRepo, p.SourceRepo)
	for k, v := range opts.metaPatch {
		if strings.TrimSpace(k) == "" {
			continue
		}
		metadata[k] = strings.TrimSpace(v)
	}
	if opts.blocked {
		metadata.SetFlag(domain.ExecutionMetadataChainBlocked, true)
	}
	if opts.dependencyBlocked {
		metadata.SetFlag(domain.ExecutionMetadataDependencyBlocked, true)
	}
	if len(needs) > 0 {
		metadata.Set(domain.ExecutionMetadataNeedsJobIDs, strings.Join(needs, ","))
		metadata.SetFlag(domain.ExecutionMetadataNeedsBlocked, true)
	}
	if len(missingNeeds) > 0 {
		metadata.Set(domain.ExecutionMetadataMissingNeedsJobIDs, strings.Join(missingNeeds, ","))
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
	if strings.TrimSpace(runCtx.SourceRefRaw) != "" {
		env["CIWI_PIPELINE_SOURCE_REF_RAW"] = strings.TrimSpace(runCtx.SourceRefRaw)
	}
	env["CIWI_PIPELINE_SOURCE_REPO"] = p.SourceRepo
	var dependencyArtifactJobIDs []string
	if !opts.dependencyBlocked && !opts.blocked {
		depJobIDs, resolveErr := resolveDependencyArtifactJobIDs(artifactSources, depCtx)
		if resolveErr != nil {
			return nil, fmt.Errorf("pipeline job %q: %w", pipelineJobID, resolveErr)
		}
		dependencyArtifactJobIDs = depJobIDs
	}
	if containerImage := strings.TrimSpace(runsOn["container_image"]); containerImage != "" {
		metadata.Set(domain.ExecutionMetadataRuntimeContainerImage, containerImage)
	}
	if containerWorkdir := strings.TrimSpace(runsOn["container_workdir"]); containerWorkdir != "" {
		metadata.Set(domain.ExecutionMetadataRuntimeContainerWorkdir, containerWorkdir)
	}
	if containerUser := strings.TrimSpace(runsOn["container_user"]); containerUser != "" {
		metadata.Set(domain.ExecutionMetadataRuntimeContainerUser, containerUser)
	}
	if containerDevices := strings.TrimSpace(runsOn["container_devices"]); containerDevices != "" {
		metadata.Set(domain.ExecutionMetadataRuntimeContainerDevices, containerDevices)
	}
	if containerGroups := strings.TrimSpace(runsOn["container_groups"]); containerGroups != "" {
		metadata.Set(domain.ExecutionMetadataRuntimeContainerGroups, containerGroups)
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
	if selection != nil {
		agentID := strings.TrimSpace(selection.AgentID)
		if agentID != "" {
			if requiredCaps == nil {
				requiredCaps = map[string]string{}
			}
			if existing := strings.TrimSpace(requiredCaps["agent_id"]); existing != "" && existing != agentID {
				return nil, fmt.Errorf("selection requested agent_id %q but job %q requires agent_id %q", agentID, pipelineJobID, existing)
			}
			requiredCaps["agent_id"] = agentID
		}
	}
	sourceRef := p.SourceRef
	if runCtx.SourceRefResolved != "" {
		sourceRef = runCtx.SourceRefResolved
	}
	return &pendingJob{
		pipelineJobID:            pipelineJobID,
		needs:                    append([]string(nil), needs...),
		script:                   strings.Join(rendered, "\n"),
		env:                      cloneMap(env),
		requiredCaps:             requiredCaps,
		timeoutSeconds:           timeoutSeconds,
		artifactGlobs:            append([]string(nil), artifacts...),
		dependencyArtifactJobIDs: append([]string(nil), dependencyArtifactJobIDs...),
		caches:                   cloneJobCachesFromPersisted(caches),
		sourceRepo:               p.SourceRepo,
		sourceRef:                sourceRef,
		metadata:                 metadata,
		stepPlan:                 stepPlan,
	}, nil
}
