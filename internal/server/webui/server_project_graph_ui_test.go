package webui

import (
	"strings"
	"testing"
)

func TestProjectUIIncludesDefinitionGraphAndRememberedListToggle(t *testing.T) {
	combined := projectHTML + projectCSS + projectJS
	for _, want := range []string{
		`id="structureGraphBtn"`,
		`id="structureListBtn"`,
		`const structureViewStorageKey = 'ciwi.project.structure.view.v1'`,
		`function projectGraphChainStorageKey(projectID)`,
		`function setProjectStructureView(view)`,
		`function initializeProjectGraph(project)`,
		`localStorage.getItem(key)`,
		`localStorage.setItem(key, value)`,
	} {
		if !strings.Contains(combined, want) {
			t.Fatalf("project graph UI no longer contains %q", want)
		}
	}
}

func TestProjectGraphUsesConfiguredPipelineAndJobDependencies(t *testing.T) {
	for _, want := range []string{
		`function buildProjectDAGLayout(nodes)`,
		`dependsOn: (pipeline.depends_on || []).filter`,
		`dependsOn: (job.needs || []).filter`,
		`filteredProjectGraphPipelines(project)`,
		`pipelineChainDisplayName(chain)`,
		`The dependency data contains a cycle`,
		`is not present.`,
	} {
		if !strings.Contains(projectJS, want) {
			t.Fatalf("project graph implementation no longer contains %q", want)
		}
	}
	if strings.Contains(projectJS, "chain.pipelines.map") {
		t.Fatal("project graph appears to derive dependency edges from chain ordering")
	}
}

func TestProjectGraphProvidesAccessibleNavigationAndSharedActions(t *testing.T) {
	combined := projectHTML + projectJS
	for _, want := range []string{
		`button.setAttribute('aria-pressed'`,
		`button.setAttribute('aria-label'`,
		`svg.setAttribute('aria-hidden', 'true')`,
		`title.setAttribute('data-ciwi-overflow-text', node.label)`,
		`destroyOverflowTooltips(host)`,
		`bindOverflowTooltips(host)`,
		`fitProjectGraph`,
		`applyProjectGraphScale`,
		`appendPipelineActionControls(actions, pipeline)`,
		`appendJobActionControls(panel, pipeline, job)`,
		`appendPipelineActionControls(headControls, pl)`,
		`appendJobActionControls(jobActions, pl, j, true)`,
	} {
		if !strings.Contains(combined, want) {
			t.Fatalf("project graph navigation/action integration no longer contains %q", want)
		}
	}
}

func TestProjectGraphFitCanEnlargeAndCenterNarrowGraphs(t *testing.T) {
	for _, want := range []string{
		`Math.min(projectGraphMaxScale, (viewport.clientWidth - 18) / width)`,
		`content.style.left = Math.max(0, Math.floor((viewport.clientWidth - width * next) / 2)) + 'px';`,
	} {
		if !strings.Contains(projectJS, want) {
			t.Fatalf("project graph fit behavior no longer contains %q", want)
		}
	}
	if strings.Contains(projectJS, `const widthScale = Math.min(1,`) {
		t.Fatal("project graph fit must not cap narrow graphs at 100% zoom")
	}
}

func TestProjectGraphProvidesPlayActionsMatrixChooserAndConfiguredSteps(t *testing.T) {
	combined := projectHTML + projectCSS + projectJS
	for _, want := range []string{
		`play.appendChild(ciwiIconElement('player-play'))`,
		`await options.onRun(node.id, event, play)`,
		`function openProjectMatrixRunChooser(pipeline, job)`,
		`runProjectPipeline(event, pipeline, button)`,
		`runProjectJobSelection(event, pipeline`,
		`function renderProjectStepSequence(host, job)`,
		`heading.textContent = 'Configured steps'`,
		`step.skip_dry_run ? ' · skipped in dry run'`,
	} {
		if !strings.Contains(combined, want) {
			t.Fatalf("project graph play/step integration no longer contains %q", want)
		}
	}
}

func TestJobExecutionUIIncludesRunContextAndStepNavigator(t *testing.T) {
	combined := jobExecutionHTML + jobExecutionCSS + jobExecutionJS + jobExecutionJS + projectJS
	for _, want := range []string{
		`id="runContextCard"`,
		`id="executionStepNavigator"`,
		`/graph-context`,
		`function renderJobRunContext()`,
		`function rerunGraphExecution(execution, button)`,
		`function runCurrentDefinitionPipeline(event, pipeline, button)`,
		`function renderExecutionStepNavigator(job, events)`,
		`setTailingEnabled(false)`,
		`details.open = true`,
		`await renderJobExecutionGraphs(job, events, active)`,
		`ciwi.jobExecution.runContext.collapsed.v1`,
	} {
		if !strings.Contains(combined, want) {
			t.Fatalf("job execution graph UI no longer contains %q", want)
		}
	}
}

func TestJobExecutionStepNavigatorDoesNotRebuildOrRescrollOnEveryPoll(t *testing.T) {
	for _, want := range []string{
		`function executionStepNavigatorSignature(groups)`,
		`host.__ciwiExecutionStepSignature === signature`,
		`updateExecutionStepNavigatorNodes(previousScroll, groups, job)`,
		`host.__ciwiExecutionStepActiveKey === activeKey`,
		`Math.abs(track.scrollLeft - target) > 1`,
		`if (!wasTailing && tailingEnabled)`,
	} {
		if !strings.Contains(jobExecutionJS+jobExecutionJS, want) {
			t.Fatalf("job execution step navigator stability no longer contains %q", want)
		}
	}
}

func TestJobExecutionStepNavigationUsesLogRelativePositionWithoutFocusOutline(t *testing.T) {
	for _, want := range []string{
		`.execution-step-node:focus { outline:none; }`,
		`details.getBoundingClientRect().top - logBox.getBoundingClientRect().top + logBox.scrollTop`,
		`requestAnimationFrame(() => {`,
	} {
		if !strings.Contains(jobExecutionCSS+jobExecutionJS, want) {
			t.Fatalf("job execution step navigation no longer contains %q", want)
		}
	}
	if strings.Contains(jobExecutionJS, `details.offsetTop - 8`) {
		t.Fatal("job execution step navigation must not use an offsetParent-relative position")
	}
}

func TestRunContextGraphsSizeToTheirContent(t *testing.T) {
	for _, want := range []string{
		`.run-context-graph .project-graph-viewport { height:148px; min-height:0; }`,
		`const viewportHeight = Math.min(360, Math.max(148, layout.contentHeight + 16));`,
		`viewport.style.height = viewportHeight + 'px';`,
	} {
		if !strings.Contains(jobExecutionCSS+jobExecutionJS, want) {
			t.Fatalf("run-context graph content sizing no longer contains %q", want)
		}
	}
}

func TestRunContextGraphSelectionUpdatesInPlace(t *testing.T) {
	for _, want := range []string{
		`wrapper.dataset.nodeId = node.id`,
		`function setProjectDAGSelectedNode(viewport, selectedID)`,
		`setProjectDAGSelectedNode(viewport, id)`,
		`onSelect: selectJob`,
		`previousDetail.replaceWith(buildRunContextJobDetail(job, context))`,
		`if (nextSignature === jobExecutionGraphState.contextSignature) return;`,
	} {
		if !strings.Contains(projectJS+jobExecutionJS, want) {
			t.Fatalf("run-context graph stable selection no longer contains %q", want)
		}
	}
}

func TestProjectGraphFitAlsoFitsViewportHeight(t *testing.T) {
	for _, want := range []string{
		`const widthScale = Math.max(projectGraphMinScale, Math.min(projectGraphMaxScale, (viewport.clientWidth - 18) / width));`,
		`const fittedHeight = Math.min(520, Math.max(148, Math.ceil(height * widthScale) + 18));`,
		`viewport.style.height = fittedHeight + 'px';`,
	} {
		if !strings.Contains(projectJS, want) {
			t.Fatalf("project graph fitted viewport sizing no longer contains %q", want)
		}
	}
}

func TestProjectGraphRestoresFitAndKeepsLargeJobGraphsScrollable(t *testing.T) {
	for _, want := range []string{
		`requestAnimationFrame(() => requestAnimationFrame(fitProjectGraph));`,
		`const previousHeight = previousViewport ? previousViewport.style.height : '';`,
		`if (previousHeight) viewport.style.height = previousHeight;`,
		`scale: projectGraphState.scale`,
		`select.className = 'ciwi-select project-graph-select';`,
	} {
		if !strings.Contains(projectJS, want) {
			t.Fatalf("project graph sizing/select behavior no longer contains %q", want)
		}
	}
	if strings.Contains(projectJS, `(viewport.clientWidth - 16) / jobLayout.contentWidth`) {
		t.Fatal("selected pipeline job graph must scroll instead of shrinking all jobs to a tiny scale")
	}
}

func TestGraphPlayTooltipsExplainIndependentConcurrentExecutions(t *testing.T) {
	combined := sharedJS + projectJS + jobExecutionJS
	for _, want := range []string{
		`function ciwiIndependentExecutionTooltip(action, options)`,
		`It does not cancel, pause, replace, or otherwise change any queued or running execution`,
		`ciwiIndependentExecutionTooltip('Run this pipeline.'`,
		`ciwiIndependentExecutionTooltip('Rerun the latest stored job execution.'`,
	} {
		if !strings.Contains(combined, want) {
			t.Fatalf("graph play tooltip behavior no longer contains %q", want)
		}
	}
}

func TestJobHeaderTooltipsKeepStableAnchorsAcrossUnchangedPolls(t *testing.T) {
	for _, want := range []string{
		`if (meta.__ciwiHTML !== metaHTML)`,
		`meta.__ciwiHTML = metaHTML`,
		`if (subtitleElement.__ciwiHTML !== subtitle)`,
		`subtitleElement.__ciwiHTML = subtitle`,
	} {
		if !strings.Contains(jobExecutionJS, want) {
			t.Fatalf("job header stable tooltip rendering no longer contains %q", want)
		}
	}
}
