package server

import (
	"strings"
	"testing"
)

func TestJobExecutionUIRendersUnreachedTimelineSteps(t *testing.T) {
	for _, want := range []string{
		"job && job.execution_timeline",
		"reached: false",
		"groups.forEach(group => {",
		"log-step-unreached",
		"Not reached",
		"(step was not reached)",
		"const category = isPhase ? 'Ciwi phase' : 'Job step';",
		"group.categoryIndex = index + 1;",
		"group.categoryTotal = phases.length;",
		"group.categoryTotal = jobSteps.length;",
	} {
		if !strings.Contains(jobExecutionRenderJS, want) {
			t.Fatalf("job execution renderer no longer contains %q", want)
		}
	}
}

func TestJobExecutionUIMapsCurrentCategoryPositionToTimeline(t *testing.T) {
	for _, want := range []string{
		"function activeTimelineIndex(job)",
		"text.match(/^Job step",
		"text.match(/^Ciwi phase",
		"Number((entry && entry.step_index) || 0) === stepIndex",
		"phases[phaseIndex - 1]",
		"Backward compatibility for jobs currently running on an older agent.",
	} {
		if !strings.Contains(jobExecutionDataJS, want) {
			t.Fatalf("job execution current-position mapping no longer contains %q", want)
		}
	}
}

func TestJobExecutionUISavesStepOpenStateInLocalStorage(t *testing.T) {
	for _, want := range []string{
		"ciwi.jobExecution.stepOpen.v1.",
		"localStorage.getItem(storageKey)",
		"localStorage.setItem(storageKey, JSON.stringify(logStepOpenState))",
		"logStepOpenState[key] = !!d.open;",
		"saveLogStepOpenState();",
	} {
		if !strings.Contains(jobExecutionDataJS, want) {
			t.Fatalf("job execution step state persistence no longer contains %q", want)
		}
	}
}
