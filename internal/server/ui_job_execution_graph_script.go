package server

const jobExecutionGraphCSS = `
    .run-context-card-head { display:flex; align-items:center; justify-content:space-between; gap:8px; flex-wrap:wrap; }
    .run-context-body { margin-top:10px; }
    .run-context-card.collapsed .run-context-body { display:none; }
    .run-context-pipeline-details { margin-top:10px; padding-top:10px; border-top:1px solid var(--line); }
    .run-context-pipeline-details h4 { margin:0 0 8px; }
    .run-context-graph .project-graph-viewport { height:148px; min-height:0; }
    .run-context-job-layout { display:grid; grid-template-columns:minmax(0,3fr) minmax(280px,2fr); gap:12px; align-items:start; }
    .run-context-job-detail { min-height:190px; padding:10px; border:1px solid var(--line); border-radius:9px; background:var(--surface-subtle); }
    .run-context-job-detail h4 { margin:0 0 7px; }
    .run-context-execution-row { display:flex; align-items:center; justify-content:space-between; gap:8px; padding:7px 0; border-top:1px solid var(--line); }
    .run-context-execution-row:first-of-type { border-top:0; }
    .run-context-execution-meta { min-width:0; font-size:12px; overflow-wrap:anywhere; }
    .run-context-execution-actions { display:flex; align-items:center; gap:6px; flex:0 0 auto; }
    .run-context-status { display:inline-block; margin-left:5px; font-size:11px; font-weight:700; }
    .run-context-status.status-succeeded { color:var(--ok); }
    .run-context-status.status-failed { color:var(--bad); }
    .run-context-status.status-running { color:var(--warn); }
    .execution-step-navigator { margin:8px 0 12px; padding:9px; border:1px solid var(--line); border-radius:9px; background:var(--surface-subtle); }
    .execution-step-navigator-title { margin-bottom:7px; font-size:12px; font-weight:700; color:var(--muted); }
    .execution-step-track { display:flex; align-items:stretch; overflow:auto; padding:2px 1px 7px; }
    .execution-step-node { flex:0 0 150px; min-height:62px; padding:7px 8px; border:1px solid var(--graph-node-border); border-radius:7px; background:var(--graph-node-bg); color:var(--ink); text-align:left; cursor:pointer; -webkit-user-select:text; user-select:text; }
    .execution-step-node.status-running { border-color:var(--graph-running-border); background:var(--graph-running-bg); }
    .execution-step-node.status-succeeded { border-color:var(--graph-succeeded-border); background:var(--graph-succeeded-bg); }
    .execution-step-node.status-failed { border-color:var(--graph-failed-border); background:var(--graph-failed-bg); }
    .execution-step-node.status-skipped { border-color:var(--graph-skipped-border); background:var(--graph-skipped-bg); }
    .execution-step-node.status-not-reached { opacity:.62; }
    .execution-step-node:focus { outline:none; }
    .execution-step-node-title { display:block; font-size:12px; font-weight:700; overflow:hidden; text-overflow:ellipsis; white-space:nowrap; }
    .execution-step-node-meta { display:block; margin-top:5px; font-size:10px; color:var(--muted); }
    .execution-step-arrow { flex:0 0 24px; display:flex; align-items:center; justify-content:center; color:var(--graph-edge); }
    @media (max-width:760px) {
      .run-context-job-layout { grid-template-columns:1fr; }
    }
`

const jobExecutionGraphJS = `
    const runContextCollapsedStorageKey = 'ciwi.jobExecution.runContext.collapsed.v1';
    const jobExecutionGraphState = {
      context: null,
      selectedPipelineID: '',
      selectedJobID: '',
      terminalLoaded: false,
      contextSignature: '',
    };

    function readRunContextCollapsed() {
      try { return localStorage.getItem(runContextCollapsedStorageKey) === '1'; } catch (_) { return false; }
    }

    function writeRunContextCollapsed(collapsed) {
      try { localStorage.setItem(runContextCollapsedStorageKey, collapsed ? '1' : '0'); } catch (_) {}
    }

    function initializeJobRunContextCard() {
      const card = document.getElementById('runContextCard');
      const button = document.getElementById('runContextToggleBtn');
      if (!card || !button) return;
      const apply = collapsed => {
        card.classList.toggle('collapsed', collapsed);
        button.textContent = collapsed ? 'Expand' : 'Collapse';
        button.setAttribute('aria-expanded', collapsed ? 'false' : 'true');
      };
      apply(readRunContextCollapsed());
      button.onclick = () => {
        const collapsed = !card.classList.contains('collapsed');
        apply(collapsed);
        writeRunContextCollapsed(collapsed);
      };
    }

    function fitExecutionGraphViewport(viewport, layout) {
      if (!viewport || !layout) return;
      const viewportHeight = Math.min(360, Math.max(148, layout.contentHeight + 16));
      viewport.style.height = viewportHeight + 'px';
      requestAnimationFrame(() => {
        const stage = viewport.querySelector('.project-graph-stage');
        const content = viewport.querySelector('.project-graph-content');
        if (!stage || !content) return;
        const scale = Math.max(projectGraphMinScale, Math.min(1, (viewport.clientWidth - 16) / layout.contentWidth, (viewport.clientHeight - 16) / layout.contentHeight));
        stage.style.width = Math.ceil(layout.contentWidth * scale) + 'px';
        stage.style.height = Math.ceil(layout.contentHeight * scale) + 'px';
        content.style.transform = 'scale(' + scale + ')';
      });
    }

    async function runCurrentDefinitionPipeline(event, pipeline, button) {
      const pipelineDBID = Number((pipeline && pipeline.pipeline_db_id) || 0);
      if (!pipelineDBID) return;
      if (button) button.disabled = true;
      try {
        const result = await runWithOptionalSourceRef(event, {
          runPath: '/api/v1/pipelines/' + pipelineDBID + '/run-selection',
          sourceRefsPath: '/api/v1/pipelines/' + pipelineDBID + '/source-refs',
          eligibleAgentsPath: '/api/v1/pipelines/' + pipelineDBID + '/eligible-agents',
          payload: {},
          title: 'Run Current Pipeline Definition',
          subtitle: String(pipeline.pipeline_id || ''),
          runLabel: 'Run Pipeline',
        });
        if (result.cancelled) return;
        showQueuedJobsSnackbar('Current ' + String(pipeline.pipeline_id || 'pipeline') + ' definition started');
      } catch (error) {
        await showAlertDialog({ title: 'Run failed', message: 'Run failed: ' + String(error && error.message || error) });
      } finally {
        if (button) button.disabled = false;
      }
    }

    async function rerunGraphExecution(execution, button) {
      const id = String((execution && execution.id) || '').trim();
      if (!id) return false;
      if (button) button.disabled = true;
      try {
        const response = await fetch('/api/v1/jobs/' + encodeURIComponent(id) + '/rerun', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: '{}',
        });
        if (!response.ok) throw new Error(await response.text() || ('HTTP ' + response.status));
        const data = await response.json();
        const queuedID = String((((data || {}).job_execution || {}).id) || '').trim();
        showJobStartedSnackbar('Job rerun started', queuedID);
        jobExecutionGraphState.terminalLoaded = false;
        return true;
      } catch (error) {
        await showAlertDialog({ title: 'Run again failed', message: 'Run again failed: ' + String(error && error.message || error) });
        return false;
      } finally {
        if (button) button.disabled = false;
      }
    }

    function latestGraphExecutions(jobNode) {
      return (Array.isArray(jobNode && jobNode.executions) ? jobNode.executions : []).filter(execution => !!execution.latest_attempt);
    }

    function graphExecutionLabel(execution) {
      return String((execution && execution.matrix_name) || '').trim() ||
        (String((execution && execution.matrix_index) || '').trim() ? ('index-' + String(execution.matrix_index)) : 'job');
    }

    function renderJobRunContext() {
      const card = document.getElementById('runContextCard');
      const body = document.getElementById('runContextBody');
      const context = jobExecutionGraphState.context;
      if (!card || !body || !context) return;
      card.style.display = '';
      const pipelines = Array.isArray(context.pipelines) ? context.pipelines : [];
      if (!pipelines.some(pipeline => pipeline.pipeline_id === jobExecutionGraphState.selectedPipelineID)) {
        jobExecutionGraphState.selectedPipelineID = String(context.current_pipeline_id || (pipelines[0] && pipelines[0].pipeline_id) || '');
        jobExecutionGraphState.selectedJobID = '';
      }
      const selectedPipeline = pipelines.find(pipeline => pipeline.pipeline_id === jobExecutionGraphState.selectedPipelineID) || pipelines[0];
      const oldPipelineViewport = document.getElementById('runContextPipelineViewport');
      const oldJobViewport = document.getElementById('runContextJobViewport');
      const oldPipelineScroll = oldPipelineViewport ? { left: oldPipelineViewport.scrollLeft, top: oldPipelineViewport.scrollTop } : null;
      const oldJobScroll = oldJobViewport ? { left: oldJobViewport.scrollLeft, top: oldJobViewport.scrollTop } : null;
      destroyOverflowTooltips(body);
      body.innerHTML = '';
      const graph = document.createElement('div');
      graph.className = 'run-context-graph';
      const viewport = document.createElement('div');
      viewport.id = 'runContextPipelineViewport';
      viewport.className = 'project-graph-viewport';
      graph.appendChild(viewport);
      const pipelineIDs = new Set(pipelines.map(pipeline => pipeline.pipeline_id));
      const nodes = pipelines.map(pipeline => ({
        id: pipeline.pipeline_id,
        label: pipeline.pipeline_id,
        dependsOn: (pipeline.depends_on || []).filter(dep => pipelineIDs.has(dep)),
        meta: String(pipeline.status || 'unknown') + ' · ' + (pipeline.jobs || []).length + ' job(s)',
        status: pipeline.status,
        runnable: Number(pipeline.pipeline_db_id || 0) > 0,
        runLabel: 'Run current definition of pipeline ' + pipeline.pipeline_id,
        runTitle: 'Starts a fresh run from the current project definition. Shift-click to choose source ref and agent.',
      }));
      const pipelineLayout = renderProjectDAG(viewport, nodes, {
        scale: 1,
        selectedID: selectedPipeline ? selectedPipeline.pipeline_id : '',
        emptyText: 'No run context available',
        onSelect: id => {
          jobExecutionGraphState.selectedPipelineID = id;
          jobExecutionGraphState.selectedJobID = '';
          setProjectDAGSelectedNode(viewport, id);
          const previousDetails = body.querySelector('.run-context-pipeline-details');
          if (previousDetails) {
            destroyOverflowTooltips(previousDetails);
            previousDetails.remove();
          }
          const pipeline = pipelines.find(item => item.pipeline_id === id);
          const nextDetails = pipeline ? renderJobRunContextPipeline(body, pipeline, context, null) : null;
          if (nextDetails) bindOverflowTooltips(nextDetails);
        },
        onRun: async (id, event, button) => {
          const pipeline = pipelines.find(item => item.pipeline_id === id);
          if (pipeline) await runCurrentDefinitionPipeline(event, pipeline, button);
        },
      });
      body.appendChild(graph);
      fitExecutionGraphViewport(viewport, pipelineLayout);
      if (oldPipelineScroll) {
        viewport.scrollLeft = oldPipelineScroll.left;
        viewport.scrollTop = oldPipelineScroll.top;
      }
      if (selectedPipeline) renderJobRunContextPipeline(body, selectedPipeline, context, oldJobScroll);
      bindOverflowTooltips(body);
    }

    function renderJobRunContextPipeline(body, pipeline, context, oldScroll) {
      const section = document.createElement('section');
      section.className = 'run-context-pipeline-details';
      const heading = document.createElement('h4');
      heading.innerHTML = 'Jobs in <code>' + escapeHtml(pipeline.pipeline_id || '') + '</code>';
      section.appendChild(heading);
      const jobs = Array.isArray(pipeline.jobs) ? pipeline.jobs : [];
      const jobIDs = new Set(jobs.map(job => job.pipeline_job_id));
      if (!jobIDs.has(jobExecutionGraphState.selectedJobID)) {
        jobExecutionGraphState.selectedJobID = pipeline.pipeline_id === context.current_pipeline_id && jobIDs.has(context.current_pipeline_job_id)
          ? context.current_pipeline_job_id
          : String((jobs[0] && jobs[0].pipeline_job_id) || '');
      }
      const selectedJob = jobs.find(job => job.pipeline_job_id === jobExecutionGraphState.selectedJobID) || jobs[0];
      const layout = document.createElement('div');
      layout.className = 'run-context-job-layout';
      const graph = document.createElement('div');
      graph.className = 'run-context-graph';
      const viewport = document.createElement('div');
      viewport.id = 'runContextJobViewport';
      viewport.className = 'project-graph-viewport';
      graph.appendChild(viewport);
      const nodes = jobs.map(job => ({
        id: job.pipeline_job_id,
        label: job.pipeline_job_id,
        dependsOn: (job.needs || []).filter(need => jobIDs.has(need)),
        meta: String(job.status || 'unknown') + ' · ' + latestGraphExecutions(job).length + ' execution(s)',
        status: job.status,
        runnable: latestGraphExecutions(job).length > 0,
        runLabel: 'Run job ' + job.pipeline_job_id + ' again',
        runTitle: latestGraphExecutions(job).length > 1 ? 'Choose a matrix execution to rerun.' : 'Rerun the latest stored job execution.',
      }));
      const selectJob = id => {
        jobExecutionGraphState.selectedJobID = id;
        setProjectDAGSelectedNode(viewport, id);
        const job = jobs.find(item => item.pipeline_job_id === id);
        const previousDetail = layout.querySelector('.run-context-job-detail');
        if (previousDetail) previousDetail.replaceWith(buildRunContextJobDetail(job, context));
      };
      const jobLayout = renderProjectDAG(viewport, nodes, {
        scale: 1,
        selectedID: selectedJob ? selectedJob.pipeline_job_id : '',
        emptyText: 'No jobs in this pipeline run',
        onSelect: selectJob,
        onRun: async (id, event, button) => {
          const job = jobs.find(item => item.pipeline_job_id === id);
          if (!job) return;
          const latest = latestGraphExecutions(job);
          if (latest.length === 1) {
            await rerunGraphExecution(latest[0], button);
          } else {
            selectJob(id);
          }
        },
      });
      layout.appendChild(graph);
      fitExecutionGraphViewport(viewport, jobLayout);
      if (oldScroll) {
        viewport.scrollLeft = oldScroll.left;
        viewport.scrollTop = oldScroll.top;
      }
      layout.appendChild(buildRunContextJobDetail(selectedJob, context));
      section.appendChild(layout);
      body.appendChild(section);
      return section;
    }

    function buildRunContextJobDetail(job, context) {
      const panel = document.createElement('div');
      panel.className = 'run-context-job-detail';
      if (!job) return panel;
      const heading = document.createElement('h4');
      heading.innerHTML = 'Job: <code>' + escapeHtml(job.pipeline_job_id || '') + '</code> <span class="run-context-status status-' + escapeHtml(job.status || 'unknown') + '">' + escapeHtml(job.status || 'unknown') + '</span>';
      panel.appendChild(heading);
      const executions = Array.isArray(job.executions) ? job.executions.slice().reverse() : [];
      executions.forEach(execution => {
        const row = document.createElement('div');
        row.className = 'run-context-execution-row';
        const info = document.createElement('div');
        info.className = 'run-context-execution-meta';
        const label = graphExecutionLabel(execution);
        info.innerHTML = '<a href="/jobs/' + encodeURIComponent(execution.id) + '?back=' + encodeURIComponent(window.location.pathname + window.location.search) + '">' + escapeHtml(label) + '</a>' +
          ' <span class="run-context-status status-' + escapeHtml(execution.status || 'unknown') + '">' + escapeHtml(execution.status || 'unknown') + '</span>' +
          (execution.id === context.current_execution_id ? ' <strong>(viewing)</strong>' : '') +
          (!execution.latest_attempt ? ' <span class="muted">older attempt</span>' : '');
        const actions = document.createElement('div');
        actions.className = 'run-context-execution-actions';
        if (execution.latest_attempt) {
          const rerun = document.createElement('button');
          rerun.type = 'button';
          rerun.className = 'secondary';
          rerun.classList.add('ciwi-icon-only');
          rerun.appendChild(ciwiIconElement('player-play'));
          rerun.setAttribute('aria-label', 'Run ' + label + ' again');
          rerun.title = 'Rerun this stored job execution';
          rerun.onclick = () => rerunGraphExecution(execution, rerun);
          actions.appendChild(rerun);
        }
        row.appendChild(info);
        row.appendChild(actions);
        panel.appendChild(row);
      });
      return panel;
    }

    async function refreshJobRunContext(job, active) {
      if (!active && jobExecutionGraphState.terminalLoaded && jobExecutionGraphState.context) return;
      const id = String((job && job.id) || jobExecutionIdFromPath()).trim();
      if (!id) return;
      try {
        const response = await fetch('/api/v1/jobs/' + encodeURIComponent(id) + '/graph-context', { cache: 'no-store' });
        if (!response.ok) throw new Error(await response.text());
        const nextContext = await response.json();
        const nextSignature = JSON.stringify(nextContext);
        jobExecutionGraphState.context = nextContext;
        jobExecutionGraphState.terminalLoaded = !active;
        if (nextSignature === jobExecutionGraphState.contextSignature) return;
        jobExecutionGraphState.contextSignature = nextSignature;
        renderJobRunContext();
      } catch (_) {
        const card = document.getElementById('runContextCard');
        if (card && !jobExecutionGraphState.context) card.style.display = 'none';
      }
    }

    function executionNavigatorStatus(group, job) {
      const item = (group && group.item) || {};
      if (group && group.step && String(group.step.kind || '') === 'dryrun_skip') return 'skipped';
      const finish = group && group.finish;
      if (finish) {
        const exitCode = finish.exit_code;
        return String(finish.error || '').trim() || (exitCode !== null && exitCode !== undefined && Number(exitCode) !== 0) ? 'failed' : 'succeeded';
      }
      const activeIndex = activeTimelineIndex(job);
      if (isRunningJobStatus((job && job.status) || '') && Number(item.index || 0) === activeIndex + 1) return 'running';
      return group && group.reached ? 'running' : 'not-reached';
    }

    function navigateToExecutionStep(key) {
      const logBox = document.getElementById('logBox');
      if (!logBox) return;
      const details = Array.from(logBox.querySelectorAll('details.log-step[data-step-key]')).find(node => String(node.getAttribute('data-step-key') || '') === key);
      if (!details) return;
      setTailingEnabled(false);
      details.open = true;
      logStepOpenState[key] = true;
      saveLogStepOpenState();
      suppressLogScrollEvent = true;
      requestAnimationFrame(() => {
        if (!details.isConnected) {
          suppressLogScrollEvent = false;
          return;
        }
        const target = details.getBoundingClientRect().top - logBox.getBoundingClientRect().top + logBox.scrollTop;
        logBox.scrollTop = Math.max(0, target - 8);
        updateLogStepCollapseButtons();
        requestAnimationFrame(() => { suppressLogScrollEvent = false; });
      });
    }

    function executionStepNavigatorSignature(groups) {
      return JSON.stringify((groups || []).map(group => [
        String(group.key || ''),
        executionGroupTitle(group),
        String(((group && group.item) || {}).kind || ''),
      ]));
    }

    function updateExecutionStepNavigatorNodes(track, groups, job) {
      const nodes = track ? track.querySelectorAll('.execution-step-node') : [];
      groups.forEach((group, index) => {
        const node = nodes[index];
        if (!node) return;
        const status = executionNavigatorStatus(group, job);
        const item = group.item || {};
        node.className = 'execution-step-node status-' + status;
        const meta = node.querySelector('.execution-step-node-meta');
        if (meta) meta.textContent = (String(item.kind || '') === 'phase' ? 'Ciwi phase' : 'Job step') + ' · ' + status.replace('-', ' ');
      });
    }

    function centerExecutionStepNavigatorOnActive(host, track, groups, job) {
      if (!tailingEnabled || !host || !track) return;
      const activeIndex = activeTimelineIndex(job);
      const active = activeIndex >= 0 ? track.querySelectorAll('.execution-step-node')[activeIndex] : null;
      if (!active) return;
      const group = groups[activeIndex] || {};
      const activeKey = String(group.key || activeIndex);
      if (host.__ciwiExecutionStepActiveKey === activeKey) return;
      host.__ciwiExecutionStepActiveKey = activeKey;
      requestAnimationFrame(() => {
        const target = Math.max(0, active.offsetLeft - (track.clientWidth - active.offsetWidth) / 2);
        if (Math.abs(track.scrollLeft - target) > 1) track.scrollLeft = target;
      });
    }

    function renderExecutionStepNavigator(job, events) {
      const host = document.getElementById('executionStepNavigator');
      if (!host) return;
      const previousScroll = host.querySelector('.execution-step-track');
      const previousScrollLeft = previousScroll ? previousScroll.scrollLeft : 0;
      const groups = structuredExecutionGroups(job, events);
      if (!groups.length) {
        destroyOverflowTooltips(host);
        host.innerHTML = '';
        host.__ciwiExecutionStepSignature = '';
        host.__ciwiExecutionStepActiveKey = '';
        host.style.display = 'none';
        return;
      }
      host.style.display = '';
      const signature = executionStepNavigatorSignature(groups);
      if (previousScroll && host.__ciwiExecutionStepSignature === signature) {
        updateExecutionStepNavigatorNodes(previousScroll, groups, job);
        centerExecutionStepNavigatorOnActive(host, previousScroll, groups, job);
        return;
      }
      destroyOverflowTooltips(host);
      host.innerHTML = '';
      const title = document.createElement('div');
      title.className = 'execution-step-navigator-title';
      title.textContent = 'Execution path';
      host.appendChild(title);
      const track = document.createElement('div');
      track.className = 'execution-step-track';
      groups.forEach((group, index) => {
        if (index > 0) {
          const arrow = document.createElement('span');
          arrow.className = 'execution-step-arrow';
          arrow.setAttribute('aria-hidden', 'true');
          arrow.textContent = '→';
          track.appendChild(arrow);
        }
        const status = executionNavigatorStatus(group, job);
        const item = group.item || {};
        const node = document.createElement('button');
        node.type = 'button';
        node.className = 'execution-step-node status-' + status;
        const nodeTitle = document.createElement('span');
        nodeTitle.className = 'execution-step-node-title';
        nodeTitle.textContent = executionGroupTitle(group);
        nodeTitle.setAttribute('data-ciwi-overflow-text', nodeTitle.textContent);
        const meta = document.createElement('span');
        meta.className = 'execution-step-node-meta';
        meta.textContent = (String(item.kind || '') === 'phase' ? 'Ciwi phase' : 'Job step') + ' · ' + status.replace('-', ' ');
        node.appendChild(nodeTitle);
        node.appendChild(meta);
        node.onclick = () => {
          if (ciwiElementContainsTextSelection(node)) return;
          navigateToExecutionStep(group.key);
        };
        track.appendChild(node);
      });
      host.appendChild(track);
      host.__ciwiExecutionStepSignature = signature;
      track.scrollLeft = previousScrollLeft;
      bindOverflowTooltips(host);
      centerExecutionStepNavigatorOnActive(host, track, groups, job);
    }

    async function renderJobExecutionGraphs(job, events, active) {
      renderExecutionStepNavigator(job, events);
      await refreshJobRunContext(job, active);
    }
`
