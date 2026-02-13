package server

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/izzyreal/ciwi/internal/config"
	"github.com/izzyreal/ciwi/internal/protocol"
	serverpipeline "github.com/izzyreal/ciwi/internal/server/pipeline"
	"github.com/izzyreal/ciwi/internal/store"
)

func (s *stateStore) enqueuePersistedPipeline(p store.PersistedPipeline, selection *protocol.RunPipelineSelectionRequest) (protocol.RunPipelineResponse, error) {
	if strings.TrimSpace(p.SourceRepo) == "" {
		return protocol.RunPipelineResponse{}, fmt.Errorf("pipeline source.repo is required")
	}
	depCtx, err := s.checkPipelineDependenciesWithReporter(p, nil)
	if err != nil {
		return protocol.RunPipelineResponse{}, err
	}
	runCtx, err := resolvePipelineRunContextWithReporter(p, depCtx, nil)
	if err != nil {
		return protocol.RunPipelineResponse{}, err
	}

	jobIDs := make([]string, 0)
	runID := fmt.Sprintf("run-%d", time.Now().UTC().UnixNano())
	type pendingJob struct {
		script         string
		env            map[string]string
		requiredCaps   map[string]string
		timeoutSeconds int
		artifactGlobs  []string
		caches         []protocol.JobCacheSpec
		sourceRepo     string
		sourceRef      string
		metadata       map[string]string
	}
	pending := make([]pendingJob, 0)
	for _, pj := range p.SortedJobs() {
		if selection != nil && strings.TrimSpace(selection.PipelineJobID) != "" && selection.PipelineJobID != pj.ID {
			continue
		}
		if len(pj.Steps) == 0 {
			return protocol.RunPipelineResponse{}, fmt.Errorf("pipeline job %q has no steps", pj.ID)
		}
		matrixEntries := pj.MatrixInclude
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
			renderVars := cloneMap(vars)
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
			rendered := make([]string, 0, len(pj.Steps)*4)
			stepBlocks := make([]string, 0, len(pj.Steps))
			stepLabels := make([]string, 0, len(pj.Steps))
			stepMarkerMeta := make([]map[string]string, 0, len(pj.Steps))
			env := make(map[string]string)
			for idx, step := range pj.Steps {
				for k, v := range step.Env {
					env[k] = renderTemplate(v, renderVars)
				}
				if step.Test != nil {
					command := renderTemplate(step.Test.Command, renderVars)
					if strings.TrimSpace(command) == "" {
						continue
					}
					name := strings.TrimSpace(step.Test.Name)
					if name == "" {
						name = fmt.Sprintf("%s-test-%d", pj.ID, idx+1)
					}
					format := strings.TrimSpace(step.Test.Format)
					if format == "" {
						format = "go-test-json"
					}
					stepBlocks = append(stepBlocks, serverpipeline.BuildTestStepBlock(strings.TrimSpace(pj.RunsOn["shell"]), command))
					stepLabels = append(stepLabels, "test "+name)
					stepMarkerMeta = append(stepMarkerMeta, map[string]string{
						"kind":        "test",
						"test_name":   serverpipeline.SanitizeMarkerToken(name),
						"test_format": serverpipeline.SanitizeMarkerToken(format),
						"test_report": serverpipeline.SanitizeMarkerToken(strings.TrimSpace(step.Test.Report)),
					})
					continue
				}
				line := renderTemplate(step.Run, renderVars)
				if strings.TrimSpace(line) == "" {
					continue
				}
				stepBlocks = append(stepBlocks, line)
				stepLabels = append(stepLabels, fmt.Sprintf("step %d", idx+1))
				stepMarkerMeta = append(stepMarkerMeta, nil)
			}
			for stepIndex := range stepBlocks {
				rendered = append(rendered, serverpipeline.BuildStepMarkerCommand(strings.TrimSpace(pj.RunsOn["shell"]), stepIndex+1, len(stepBlocks), stepLabels[stepIndex], stepMarkerMeta[stepIndex]))
				rendered = append(rendered, stepBlocks[stepIndex])
			}
			if len(rendered) == 0 {
				return protocol.RunPipelineResponse{}, fmt.Errorf("pipeline job %q rendered empty script", pj.ID)
			}

			metadata := map[string]string{
				"project":            p.ProjectName,
				"pipeline_id":        p.PipelineID,
				"pipeline_run_id":    runID,
				"pipeline_job_id":    pj.ID,
				"pipeline_job_index": strconv.Itoa(index),
			}
			if selection != nil && selection.DryRun {
				metadata["dry_run"] = "1"
			}
			if name := vars["name"]; name != "" {
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

			requiredCaps := cloneMap(pj.RunsOn)
			for tool, constraint := range pj.RequiresTools {
				tool = strings.TrimSpace(tool)
				if tool == "" {
					continue
				}
				if requiredCaps == nil {
					requiredCaps = map[string]string{}
				}
				requiredCaps["requires.tool."+tool] = strings.TrimSpace(constraint)
			}

			sourceRef := p.SourceRef
			if runCtx.SourceRefResolved != "" {
				sourceRef = runCtx.SourceRefResolved
			}
			pending = append(pending, pendingJob{
				script:         strings.Join(rendered, "\n"),
				env:            cloneMap(env),
				requiredCaps:   requiredCaps,
				timeoutSeconds: pj.TimeoutSeconds,
				artifactGlobs:  append([]string(nil), pj.Artifacts...),
				caches:         cloneJobCachesFromPersisted(pj.Caches),
				sourceRepo:     p.SourceRepo,
				sourceRef:      sourceRef,
				metadata:       metadata,
			})
		}
	}
	if runCtx.AutoBump != "" && selection != nil && selection.DryRun {
		// Explicitly skip auto bump script in dry-run mode.
		runCtx.AutoBump = ""
	}
	if runCtx.AutoBump != "" {
		if len(pending) != 1 {
			return protocol.RunPipelineResponse{}, fmt.Errorf("versioning.auto_bump requires exactly one job execution in the pipeline run")
		}
		pending[0].script = pending[0].script + "\n" + buildAutoBumpStepScript(runCtx.AutoBump)
	}
	for _, spec := range pending {
		job, err := s.pipelineStore().CreateJobExecution(protocol.CreateJobExecutionRequest{
			Script:               spec.script,
			Env:                  cloneMap(spec.env),
			RequiredCapabilities: spec.requiredCaps,
			TimeoutSeconds:       spec.timeoutSeconds,
			ArtifactGlobs:        append([]string(nil), spec.artifactGlobs...),
			Caches:               cloneProtocolJobCaches(spec.caches),
			Source:               &protocol.SourceSpec{Repo: spec.sourceRepo, Ref: spec.sourceRef},
			Metadata:             spec.metadata,
		})
		if err != nil {
			return protocol.RunPipelineResponse{}, err
		}
		jobIDs = append(jobIDs, job.ID)
	}

	if selection != nil && len(jobIDs) == 0 {
		return protocol.RunPipelineResponse{}, fmt.Errorf("selection matched no matrix entries")
	}

	return protocol.RunPipelineResponse{ProjectName: p.ProjectName, PipelineID: p.PipelineID, Enqueued: len(jobIDs), JobExecutionIDs: jobIDs}, nil
}

func cloneProtocolJobCaches(in []protocol.JobCacheSpec) []protocol.JobCacheSpec {
	if len(in) == 0 {
		return nil
	}
	out := make([]protocol.JobCacheSpec, 0, len(in))
	for _, c := range in {
		out = append(out, protocol.JobCacheSpec{
			ID:          c.ID,
			Env:         c.Env,
			Key:         cloneProtocolJobCacheKey(c.Key),
			RestoreKeys: append([]string(nil), c.RestoreKeys...),
			Policy:      c.Policy,
			TTLDays:     c.TTLDays,
			MaxSizeMB:   c.MaxSizeMB,
		})
	}
	return out
}

func cloneProtocolJobCacheKey(in protocol.JobCacheKey) protocol.JobCacheKey {
	return protocol.JobCacheKey{
		Prefix:  in.Prefix,
		Files:   append([]string(nil), in.Files...),
		Runtime: append([]string(nil), in.Runtime...),
		Tools:   append([]string(nil), in.Tools...),
		Env:     append([]string(nil), in.Env...),
	}
}

func cloneJobCachesFromPersisted(in []config.PipelineJobCacheSpec) []protocol.JobCacheSpec {
	if len(in) == 0 {
		return nil
	}
	out := make([]protocol.JobCacheSpec, 0, len(in))
	for _, c := range in {
		out = append(out, protocol.JobCacheSpec{
			ID:  c.ID,
			Env: c.Env,
			Key: protocol.JobCacheKey{
				Prefix:  c.Key.Prefix,
				Files:   append([]string(nil), c.Key.Files...),
				Runtime: append([]string(nil), c.Key.Runtime...),
				Tools:   append([]string(nil), c.Key.Tools...),
				Env:     append([]string(nil), c.Key.Env...),
			},
			RestoreKeys: append([]string(nil), c.RestoreKeys...),
			Policy:      c.Policy,
			TTLDays:     c.TTLDays,
			MaxSizeMB:   c.MaxSizeMB,
		})
	}
	return out
}
