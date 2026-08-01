package webui

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
		if !strings.Contains(jobExecutionJS, want) {
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
		if !strings.Contains(jobExecutionJS, want) {
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
		if !strings.Contains(jobExecutionJS, want) {
			t.Fatalf("job execution step state persistence no longer contains %q", want)
		}
	}
}

func TestJobExecutionUIOffersFloatingCollapseForLargeSteps(t *testing.T) {
	for _, want := range []string{
		"log-step-collapse-btn",
		"Collapse ' + ciwiIconHTML('arrow-up')",
		"function updateLogStepCollapseButtons()",
		"const largeStepThreshold = Math.max(480, logBox.clientHeight);",
		"collapseBtn.hidden = !d.open || contentHeight <= largeStepThreshold;",
		"d.open = false;",
	} {
		if !strings.Contains(jobExecutionHTML+jobExecutionJS+jobExecutionJS, want) {
			t.Fatalf("job execution floating collapse control no longer contains %q", want)
		}
	}
}

func TestJobExecutionUIDisablesTailingWhileBrowsingUnreachedSteps(t *testing.T) {
	for _, want := range []string{
		"function logUnreachedBoundary(el)",
		"const viewportBottom = el.scrollTop + el.clientHeight;",
		"if (viewportBottom > unreachedBoundary + 4) return false;",
		"if (d.classList.contains('log-step-unreached'))",
		"setTailingEnabled(false);",
	} {
		if !strings.Contains(jobExecutionJS, want) {
			t.Fatalf("job execution unreached-step scrolling no longer contains %q", want)
		}
	}
}

func TestOverflowTooltipsCoverTruncatedUI(t *testing.T) {
	for label, source := range map[string]string{
		"shared helper":          sharedJS,
		"job table cells":        pagesJS,
		"history card titles":    indexJS,
		"step command summaries": jobExecutionJS,
		"job detail bindings":    jobExecutionJS,
	} {
		if !strings.Contains(source, "data-ciwi-overflow-text") && label != "shared helper" {
			t.Fatalf("%s no longer marks truncated text for overflow tooltips", label)
		}
	}
	for _, want := range []string{
		"function elementHasOverflow(element)",
		"function createOverflowTooltip(anchor, opts)",
		"function bindOverflowTooltips(root, opts)",
		"function destroyOverflowTooltips(root)",
		"shouldShow: () => elementHasOverflow(anchor)",
		"showDelayMs: options.showDelayMs === undefined ? 1000 : options.showDelayMs",
		"hideOnAnchorLeave: true",
		"ciwiPendingHoverTooltip.cancelPendingShow()",
		"ciwiActiveHoverTooltip.hide()",
	} {
		if !strings.Contains(sharedJS, want) {
			t.Fatalf("shared overflow-tooltip helper no longer contains %q", want)
		}
	}
}
