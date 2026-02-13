package server

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/izzyreal/ciwi/internal/config"
	"github.com/izzyreal/ciwi/internal/protocol"
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
		stepPlan       []protocol.JobStepPlanItem
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
			if selection != nil && selection.DryRun {
				renderVars["ciwi.release_created"] = "no (dry-run)"
			} else {
				renderVars["ciwi.release_created"] = "yes"
			}
			rendered := make([]string, 0, len(pj.Steps))
			stepPlan := make([]protocol.JobStepPlanItem, 0, len(pj.Steps))
			env := make(map[string]string)
			for idx, step := range pj.Steps {
				if selection != nil && selection.DryRun && step.SkipDryRun {
					stepPlan = append(stepPlan, protocol.JobStepPlanItem{
						Name: describePipelineStep(step, idx, pj.ID),
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
						name = fmt.Sprintf("%s-test-%d", pj.ID, len(stepPlan)+1)
					}
					format := strings.TrimSpace(step.Test.Format)
					if format == "" {
						format = "go-test-json"
					}
					rendered = append(rendered, command)
					stepPlan = append(stepPlan, protocol.JobStepPlanItem{
						Name:       "test " + name,
						Script:     command,
						Kind:       "test",
						TestName:   strings.TrimSpace(name),
						TestFormat: strings.TrimSpace(format),
						TestReport: strings.TrimSpace(step.Test.Report),
					})
					continue
				}
				if step.Metadata != nil {
					values := map[string]string{}
					for k, v := range step.Metadata.Values {
						key := strings.TrimSpace(k)
						if key == "" {
							continue
						}
						values[key] = strings.TrimSpace(renderTemplate(v, renderVars))
					}
					stepPlan = append(stepPlan, protocol.JobStepPlanItem{
						Name:     "metadata",
						Kind:     "metadata",
						Metadata: values,
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
					Name:   describePipelineStep(step, idx, pj.ID),
					Script: line,
				})
			}
			if len(stepPlan) == 0 {
				return protocol.RunPipelineResponse{}, fmt.Errorf("pipeline job %q has no executable or metadata steps after rendering", pj.ID)
			}
			for stepIndex := range stepPlan {
				stepPlan[stepIndex].Index = stepIndex + 1
				stepPlan[stepIndex].Total = len(stepPlan)
				if strings.TrimSpace(stepPlan[stepIndex].Name) == "" {
					stepPlan[stepIndex].Name = fmt.Sprintf("step %d", stepIndex+1)
				}
			}
			if len(rendered) == 0 {
				switch strings.TrimSpace(strings.ToLower(pj.RunsOn["shell"])) {
				case "cmd":
					rendered = append(rendered, "rem ciwi metadata-only job")
				case "powershell":
					rendered = append(rendered, "$null = 1")
				default:
					rendered = append(rendered, ":")
				}
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
				stepPlan:       stepPlan,
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
		autoBumpScript := buildAutoBumpStepScript(runCtx.AutoBump)
		pending[0].script = pending[0].script + "\n" + autoBumpScript
		pending[0].stepPlan = append(pending[0].stepPlan, protocol.JobStepPlanItem{
			Index:  len(pending[0].stepPlan) + 1,
			Total:  len(pending[0].stepPlan) + 1,
			Name:   "auto bump",
			Script: autoBumpScript,
		})
		meta := map[string]string{}
		if next := buildAutoBumpNextVersion(runCtx.VersionRaw, runCtx.AutoBump); next != "" {
			meta["next_version"] = next
		}
		if branch := deriveAutoBumpBranch(strings.TrimSpace(p.SourceRef)); branch != "" {
			meta["auto_bump_branch"] = branch
		}
		if len(meta) > 0 {
			pending[0].stepPlan = append(pending[0].stepPlan, protocol.JobStepPlanItem{
				Index:    len(pending[0].stepPlan) + 1,
				Total:    len(pending[0].stepPlan) + 1,
				Name:     "auto bump metadata",
				Kind:     "metadata",
				Metadata: meta,
			})
		}
		for i := range pending[0].stepPlan {
			pending[0].stepPlan[i].Index = i + 1
			pending[0].stepPlan[i].Total = len(pending[0].stepPlan)
		}
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
			StepPlan:             cloneJobStepPlan(spec.stepPlan),
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

func cloneJobStepPlan(in []protocol.JobStepPlanItem) []protocol.JobStepPlanItem {
	if len(in) == 0 {
		return nil
	}
	out := make([]protocol.JobStepPlanItem, 0, len(in))
	for _, step := range in {
		out = append(out, protocol.JobStepPlanItem{
			Index:      step.Index,
			Total:      step.Total,
			Name:       step.Name,
			Script:     step.Script,
			Kind:       step.Kind,
			TestName:   step.TestName,
			TestFormat: step.TestFormat,
			TestReport: step.TestReport,
			Metadata:   cloneMap(step.Metadata),
		})
	}
	return out
}

func buildAutoBumpNextVersion(versionRaw, mode string) string {
	parts := strings.Split(strings.TrimSpace(versionRaw), ".")
	if len(parts) != 3 {
		return ""
	}
	major, err1 := strconv.Atoi(parts[0])
	minor, err2 := strconv.Atoi(parts[1])
	patch, err3 := strconv.Atoi(parts[2])
	if err1 != nil || err2 != nil || err3 != nil {
		return ""
	}
	switch strings.TrimSpace(mode) {
	case "patch":
		patch++
	case "minor":
		minor++
		patch = 0
	case "major":
		major++
		minor = 0
		patch = 0
	default:
		return ""
	}
	return fmt.Sprintf("%d.%d.%d", major, minor, patch)
}

func deriveAutoBumpBranch(sourceRef string) string {
	ref := strings.TrimSpace(sourceRef)
	if ref == "" {
		return ""
	}
	switch {
	case strings.HasPrefix(ref, "refs/heads/"):
		return strings.TrimSpace(strings.TrimPrefix(ref, "refs/heads/"))
	case strings.HasPrefix(ref, "refs/"):
		return ""
	}
	if len(ref) >= 7 && len(ref) <= 40 {
		isHex := true
		for _, r := range ref {
			if (r < '0' || r > '9') && (r < 'a' || r > 'f') && (r < 'A' || r > 'F') {
				isHex = false
				break
			}
		}
		if isHex {
			return ""
		}
	}
	return ref
}

func describePipelineStep(step config.PipelineJobStep, idx int, jobID string) string {
	if step.Test != nil {
		name := strings.TrimSpace(step.Test.Name)
		if name == "" {
			name = fmt.Sprintf("%s-test-%d", jobID, idx+1)
		}
		return "test " + name
	}
	if step.Metadata != nil {
		return "metadata"
	}
	return fmt.Sprintf("step %d", idx+1)
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
