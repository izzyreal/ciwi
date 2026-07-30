package protocol

import (
	"fmt"
	"strings"
)

func BuildJobExecutionTimeline(job JobExecution) []JobExecutionTimelineItem {
	items := make([]JobExecutionTimelineItem, 0, len(job.StepPlan)+6)
	addPhase := func(id, name, description string) {
		items = append(items, JobExecutionTimelineItem{ID: id, Kind: "phase", Name: name, Description: description})
	}
	addPhase(JobExecutionPhaseWorkspace, "Prepare workspace", "Create a clean workspace for this execution.")
	if job.Source != nil && strings.TrimSpace(job.Source.Repo) != "" {
		description := "Repository: " + strings.TrimSpace(job.Source.Repo)
		if ref := strings.TrimSpace(job.Source.Ref); ref != "" {
			description += "\nRef: " + ref
		}
		addPhase(JobExecutionPhaseCheckout, "Check out source", description)
	}
	if ids := dependencyArtifactIDsForTimeline(job.DependencyArtifactJobIDs); len(ids) > 0 {
		addPhase(JobExecutionPhaseDependencies, "Restore dependency artifacts", "Source jobs: "+strings.Join(ids, ", "))
	}
	envDescription := "Resolve caches, runtime capabilities, tools, shell, and execution environment."
	if len(job.Caches) > 0 {
		cacheIDs := make([]string, 0, len(job.Caches))
		for _, cache := range job.Caches {
			if id := strings.TrimSpace(cache.ID); id != "" {
				cacheIDs = append(cacheIDs, id)
			}
		}
		if len(cacheIDs) > 0 {
			envDescription += "\nCaches: " + strings.Join(cacheIDs, ", ")
		}
	}
	addPhase(JobExecutionPhaseEnvironment, "Prepare execution environment", envDescription)
	for _, step := range job.StepPlan {
		name := strings.TrimSpace(step.Name)
		items = append(items, JobExecutionTimelineItem{
			ID: "step:" + fmt.Sprint(step.Index), Kind: "step", Name: name, StepIndex: step.Index,
		})
	}
	if len(job.ArtifactGlobs) > 0 {
		addPhase(JobExecutionPhaseArtifacts, "Publish artifacts", "Globs: "+strings.Join(job.ArtifactGlobs, ", "))
	}
	if jobPublishesTestResults(job.StepPlan) {
		addPhase(JobExecutionPhaseTests, "Publish test results", "Upload collected test and coverage reports.")
	}
	for i := range items {
		items[i].Index = i + 1
		items[i].Total = len(items)
	}
	return items
}

func TimelinePhase(items []JobExecutionTimelineItem, id string) (JobExecutionPhase, bool) {
	total := 0
	for _, item := range items {
		if item.Kind == "phase" {
			total++
		}
	}
	index := 0
	for _, item := range items {
		if item.Kind != "phase" {
			continue
		}
		index++
		if item.ID == id {
			return JobExecutionPhase{ID: item.ID, Name: item.Name, Description: item.Description, Index: index, Total: total}, true
		}
	}
	return JobExecutionPhase{}, false
}

func dependencyArtifactIDsForTimeline(ids []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0)
	for _, value := range ids {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func jobPublishesTestResults(plan []JobStepPlanItem) bool {
	for _, step := range plan {
		if strings.TrimSpace(step.TestReport) != "" || strings.TrimSpace(step.CoverageReport) != "" {
			return true
		}
	}
	return false
}
