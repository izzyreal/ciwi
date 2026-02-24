package server

const projectHTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="UTF-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>ciwi project</title>
  <link rel="icon" type="image/png" href="/ciwi-favicon.png" />
  <style>
` + uiPageChromeCSS + `
    main { max-width: 1150px; }
    .top { display:flex; justify-content:space-between; align-items:center; gap:8px; flex-wrap:wrap; }
    .row { display:flex; gap:8px; align-items:center; flex-wrap:wrap; }
    .pill { font-size: 12px; padding: 2px 8px; border-radius: 999px; background: #edf8f2; color: #26644b; }
    table { width:100%; border-collapse: collapse; font-size: 13px; table-layout: fixed; }
    th, td { border-bottom: 1px solid var(--line); text-align: left; padding: 8px 6px; vertical-align: top; }
    td code { white-space: pre-wrap; max-height: 80px; overflow: auto; display: block; max-width: 100%; overflow-wrap: anywhere; word-break: break-word; }
    .status-succeeded { color: var(--ok); font-weight: 600; }
    .status-failed { color: var(--bad); font-weight: 600; }
    .status-blocked { color: #8a5a14; font-weight: 600; }
    .status-running { color: #a56a00; font-weight: 600; }
    .status-queued, .status-leased { color: var(--muted); }
    .pipeline { border-top: 1px solid var(--line); padding-top: 10px; margin-top: 10px; }
    .pipeline-head { display:flex; justify-content:space-between; align-items:flex-start; gap:8px; flex-wrap:wrap; }
    .pipeline-meta { display:flex; flex-direction:column; gap:4px; min-width: 0; }
    .pipeline-controls { display:flex; align-items:center; gap:8px; flex-wrap:wrap; margin-left:auto; }
    .pipeline-body { margin-top: 8px; }
    .pipeline.collapsed .pipeline-body { display:none; }
    .jobbox { margin: 8px 0 0 8px; padding: 8px; border-left: 2px solid var(--line); }
    .job-head { display:flex; justify-content:space-between; align-items:flex-start; gap:8px; flex-wrap:wrap; }
    .job-desc { display:flex; flex-direction:column; gap:4px; min-width:0; }
    .job-actions { display:flex; align-items:center; gap:8px; flex-wrap:wrap; margin-left:auto; }
    .matrix-list { display:flex; flex-direction:column; gap:6px; margin-top: 6px; }
    .matrix-item { border:1px solid var(--line); border-radius:8px; padding:6px; background:#fbfefd; display:flex; justify-content:space-between; align-items:flex-start; gap:8px; flex-wrap:wrap; }
    .matrix-info { min-width: 0; }
    .matrix-actions { display:flex; align-items:center; gap:8px; flex-wrap:wrap; margin-left:auto; }
    .inspect-toolbar { display:flex; gap:8px; align-items:center; margin-bottom:8px; flex-wrap:wrap; }
    .inspect-select { font-size:13px; padding:5px 8px; border:1px solid var(--line); border-radius:6px; background:#fff; color:#1f2a24; }
    .inspect-checkbox { display:inline-flex; align-items:center; gap:6px; font-size:13px; color:#1f2a24; user-select:none; }
    .inspect-content { margin:0; background:#0f1412; color:#cde7dc; border-radius:8px; border:1px solid #22352d; padding:12px; width:100%; height:100%; overflow:auto; font-size:12px; line-height:1.35; white-space:pre; font-family:ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", "Courier New", monospace; }
    .project-header-icon {
      width: 100px;
      height: 100px;
      object-fit: contain;
      border: none;
      background: transparent;
      image-rendering: pixelated;
      image-rendering: crisp-edges;
    }
  </style>
</head>
<body>
  <main>
    <div class="card top">
      <div class="brand">
        <img id="projectIcon" class="project-header-icon" alt="" style="display:none;" />
        <div>
          <div id="title" style="font-size:22px;font-weight:700;">Project</div>
          <div id="subtitle" class="muted">Loading...</div>
        </div>
      </div>
      <div><a class="nav-btn" href="/">Back to Projects <span class="nav-emoji" aria-hidden="true">↩</span></a></div>
      <div id="runtimeStateBanner" class="runtime-banner"></div>
    </div>

    <div class="card">
      <h2 style="margin:0 0 10px;">Structure</h2>
      <div id="structure">Loading...</div>
    </div>
    <div class="card">
      <h2 style="margin:0 0 10px;">Execution History</h2>
      <table>
        <thead>
          <tr><th>Job Execution</th><th>Status</th><th>Pipeline</th><th>Build</th><th>Agent</th><th>Created</th><th>Duration</th></tr>
        </thead>
        <tbody id="historyBody"></tbody>
      </table>
    </div>
  </main>

  <script src="/ui/shared.js"></script>
  <script src="/ui/pages.js"></script>
  <script>
    let refreshInFlight = false;
    const refreshGuard = createRefreshGuard(5000);
    let inspectModalState = null;

    function projectIdFromPath() {
      const parts = window.location.pathname.split('/').filter(Boolean);
      return parts.length >= 2 ? parts[1] : '';
    }
    let currentProjectName = '';
    let currentProjectID = 0;

    function ensureProjectInspectModal() {
      let overlay = document.getElementById('projectInspectOverlay');
      if (overlay) return overlay;
      ensureModalBaseStyles();
      overlay = document.createElement('div');
      overlay.id = 'projectInspectOverlay';
      overlay.className = 'ciwi-modal-overlay';
      overlay.setAttribute('aria-hidden', 'true');
      overlay.innerHTML = '' +
        '<div class="ciwi-modal" role="dialog" aria-modal="true" aria-label="Inspect pipeline or job">' +
          '<div class="ciwi-modal-head">' +
            '<div>' +
              '<div class="ciwi-modal-title" id="projectInspectTitle">Inspect</div>' +
              '<div class="ciwi-modal-subtitle" id="projectInspectSubtitle"></div>' +
            '</div>' +
            '<button type="button" class="secondary" id="projectInspectCloseBtn">Close</button>' +
          '</div>' +
          '<div class="ciwi-modal-body">' +
            '<div class="inspect-toolbar">' +
              '<label for="projectInspectView" class="muted">View:</label>' +
              '<select id="projectInspectView" class="inspect-select">' +
                '<option value="raw_yaml">Raw YAML</option>' +
                '<option value="executor_script">Executor script</option>' +
              '</select>' +
              '<label class="inspect-checkbox">' +
                '<input id="projectInspectDryRun" type="checkbox" />' +
                '<span>Dry run</span>' +
              '</label>' +
            '</div>' +
            '<pre id="projectInspectContent" class="inspect-content">Loading...</pre>' +
          '</div>' +
        '</div>';
      document.body.appendChild(overlay);
      wireModalCloseBehavior(overlay, closeProjectInspectModal);
      const closeBtn = document.getElementById('projectInspectCloseBtn');
      if (closeBtn) closeBtn.onclick = closeProjectInspectModal;
      const viewSelect = document.getElementById('projectInspectView');
      if (viewSelect) {
        viewSelect.onchange = () => {
          if (!inspectModalState) return;
          inspectModalState.view = String(viewSelect.value || 'raw_yaml').trim() || 'raw_yaml';
          syncProjectInspectControls();
          loadProjectInspectContent();
        };
      }
      const dryRunInput = document.getElementById('projectInspectDryRun');
      if (dryRunInput) {
        dryRunInput.onchange = () => {
          if (!inspectModalState) return;
          inspectModalState.dryRun = !!dryRunInput.checked;
          loadProjectInspectContent();
        };
      }
      return overlay;
    }

    function closeProjectInspectModal() {
      inspectModalState = null;
      const overlay = document.getElementById('projectInspectOverlay');
      closeModalOverlay(overlay);
    }

    function openProjectInspectModal(req, title, subtitle) {
      inspectModalState = {
        req: req || {},
        view: 'raw_yaml',
        dryRun: false,
        title: String(title || 'Inspect').trim(),
        subtitle: String(subtitle || '').trim(),
      };
      const overlay = ensureProjectInspectModal();
      const titleEl = document.getElementById('projectInspectTitle');
      const subtitleEl = document.getElementById('projectInspectSubtitle');
      const viewSelect = document.getElementById('projectInspectView');
      const dryRunInput = document.getElementById('projectInspectDryRun');
      if (titleEl) titleEl.textContent = inspectModalState.title;
      if (subtitleEl) subtitleEl.textContent = inspectModalState.subtitle;
      if (viewSelect) viewSelect.value = inspectModalState.view;
      if (dryRunInput) dryRunInput.checked = inspectModalState.dryRun;
      syncProjectInspectControls();
      openModalOverlay(overlay, '900px', '78vh');
      loadProjectInspectContent();
    }

    function syncProjectInspectControls() {
      const dryRunInput = document.getElementById('projectInspectDryRun');
      if (!dryRunInput || !inspectModalState) return;
      const isScript = String(inspectModalState.view || '').trim() === 'executor_script';
      dryRunInput.disabled = !isScript;
    }

    async function loadProjectInspectContent() {
      if (!inspectModalState) return;
      const contentEl = document.getElementById('projectInspectContent');
      if (!contentEl) return;
      contentEl.textContent = 'Loading...';
      const req = {
        ...(inspectModalState.req || {}),
        view: inspectModalState.view || 'raw_yaml',
        dry_run: !!inspectModalState.dryRun,
      };
      try {
        const data = await apiJSON('/api/v1/projects/' + encodeURIComponent(String(currentProjectID || '')) + '/inspect', {
          method: 'POST',
          body: JSON.stringify(req),
        });
        const payload = data || {};
        const title = String(payload.title || '').trim();
        const content = String(payload.content || '').trim();
        if (title) {
          const titleEl = document.getElementById('projectInspectTitle');
          if (titleEl) titleEl.textContent = title;
        }
        contentEl.textContent = content || '(empty)';
      } catch (e) {
        contentEl.textContent = 'Failed to load: ' + String(e && e.message || e);
      }
    }

    async function loadProject() {
      const id = projectIdFromPath();
      if (!id) return;
      const data = await apiJSON('/api/v1/projects/' + encodeURIComponent(id));
      const p = data.project;
      currentProjectID = p.id;
      currentProjectName = p.name || '';
      document.getElementById('title').textContent = p.name || 'Project';
      const projectRepoRef = String(p.repo_ref || '').trim() || 'default';
      document.getElementById('subtitle').innerHTML =
        '<span class="pill">' + escapeHtml(p.repo_url || '') + '</span> ' +
        '<span class="pill">' + escapeHtml('branch:' + projectRepoRef) + '</span> ' +
        '<span class="pill">' + escapeHtml(p.config_file || '') + '</span>';
      const icon = document.getElementById('projectIcon');
      icon.src = '/api/v1/projects/' + encodeURIComponent(String(p.id || '')) + '/icon';
      icon.onload = () => { icon.style.display = 'inline-block'; };
      icon.onerror = () => { icon.style.display = 'none'; };

      const structure = document.getElementById('structure');
      if (!p.pipelines || p.pipelines.length === 0) {
        structure.innerHTML = '<div class="muted">No pipelines</div>';
        return;
      }

      structure.innerHTML = '';
      p.pipelines.forEach(pl => {
        const pipelineSupportsDryRun = (pl.jobs || []).some(job =>
          (job.steps || []).some(step => !!step.skip_dry_run)
        );
        const container = document.createElement('div');
        container.className = 'pipeline';
        const head = document.createElement('div');
        head.className = 'pipeline-head';
        const deps = (pl.depends_on || []).join(', ');
        const versioning = pl.versioning || {};
        const vparts = [];
        if (versioning.file) vparts.push('file=' + versioning.file);
        if (versioning.tag_prefix) vparts.push('tag_prefix=' + versioning.tag_prefix);
        if (versioning.auto_bump) vparts.push('auto_bump=' + versioning.auto_bump);
        const headMeta = document.createElement('div');
        headMeta.className = 'pipeline-meta';
        headMeta.innerHTML = '<strong>Pipeline: <code>' + escapeHtml(pl.pipeline_id) + '</code></strong>' +
          (deps ? ('<span class="muted">depends_on: ' + escapeHtml(deps) + '</span>') : '') +
          (vparts.length > 0 ? ('<span class="muted">versioning: ' + escapeHtml(vparts.join(', ')) + '</span>') : '');
        head.appendChild(headMeta);
        const headControls = document.createElement('div');
        headControls.className = 'pipeline-controls';
        const runAll = document.createElement('button');
        runAll.textContent = 'Run Pipeline';
        runAll.className = 'secondary';
        runAll.onclick = async (ev) => {
          runAll.disabled = true;
          try {
            const runResult = await runWithOptionalSourceRef(ev, {
              runPath: '/api/v1/pipelines/' + pl.id + '/run-selection',
              sourceRefsPath: '/api/v1/pipelines/' + pl.id + '/source-refs',
              eligibleAgentsPath: '/api/v1/pipelines/' + pl.id + '/eligible-agents',
              payload: {},
              title: 'Run Pipeline With Source Ref',
              subtitle: String(pl.pipeline_id || ''),
              runLabel: 'Run',
            });
            if (runResult.cancelled) return;
            showQueuedJobsSnackbar((currentProjectName || 'Project') + ' ' + (pl.pipeline_id || 'pipeline') + ' started');
            await loadHistory();
          } catch (e) {
            await showAlertDialog({ title: 'Run failed', message: 'Run failed: ' + e.message });
          } finally {
            runAll.disabled = false;
          }
        };
        const dryAll = document.createElement('button');
        dryAll.textContent = 'Dry Run Pipeline';
        dryAll.className = 'secondary';
        dryAll.onclick = async (ev) => {
          dryAll.disabled = true;
          try {
            const runResult = await runWithOptionalSourceRef(ev, {
              runPath: '/api/v1/pipelines/' + pl.id + '/run-selection',
              sourceRefsPath: '/api/v1/pipelines/' + pl.id + '/source-refs',
              eligibleAgentsPath: '/api/v1/pipelines/' + pl.id + '/eligible-agents',
              payload: { dry_run: true },
              title: 'Dry Run Pipeline With Source Ref',
              subtitle: String(pl.pipeline_id || ''),
              runLabel: 'Dry Run',
            });
            if (runResult.cancelled) return;
            showQueuedJobsSnackbar((currentProjectName || 'Project') + ' ' + (pl.pipeline_id || 'pipeline') + ' started');
            await loadHistory();
          } catch (e) {
            await showAlertDialog({ title: 'Dry run failed', message: 'Dry run failed: ' + e.message });
          } finally {
            dryAll.disabled = false;
          }
        };
        const resolveBtn = document.createElement('button');
        resolveBtn.textContent = 'Resolve Upcoming Build Version';
        resolveBtn.className = 'secondary';
        resolveBtn.onclick = () => openVersionResolveModal(pl.id, pl.pipeline_id);
        const previewBtn = document.createElement('button');
        previewBtn.textContent = 'Preview Dry Run';
        previewBtn.className = 'secondary';
        previewBtn.onclick = () => {
          openDryRunPreviewModal({
            title: 'Preview Dry Run',
            subtitle: String(pl.pipeline_id || ''),
            previewPath: '/api/v1/pipelines/' + pl.id + '/dry-run-preview',
            runPath: '/api/v1/pipelines/' + pl.id + '/run-selection',
            sourceRefsPath: '/api/v1/pipelines/' + pl.id + '/source-refs',
            eligibleAgentsPath: '/api/v1/pipelines/' + pl.id + '/eligible-agents',
            payload: { dry_run: true },
          });
        };
        const inspectPipelineBtn = document.createElement('button');
        inspectPipelineBtn.textContent = 'Inspect Pipeline';
        inspectPipelineBtn.className = 'secondary';
        inspectPipelineBtn.onclick = () => openProjectInspectModal(
          { pipeline_db_id: pl.id },
          'Pipeline ' + (pl.pipeline_id || ''),
          'Preview raw YAML or rendered executor scripts'
        );
        const toggleBtn = document.createElement('button');
        toggleBtn.textContent = 'Collapse';
        toggleBtn.className = 'secondary';
        toggleBtn.onclick = () => {
          const collapsed = container.classList.toggle('collapsed');
          toggleBtn.textContent = collapsed ? 'Expand' : 'Collapse';
        };
        headControls.appendChild(runAll);
        if (pipelineSupportsDryRun) {
          headControls.appendChild(dryAll);
        }
        headControls.appendChild(previewBtn);
        headControls.appendChild(resolveBtn);
        headControls.appendChild(inspectPipelineBtn);
        headControls.appendChild(toggleBtn);
        head.appendChild(headControls);
        container.appendChild(head);
        const pipelineBody = document.createElement('div');
        pipelineBody.className = 'pipeline-body';

        (pl.jobs || []).forEach(j => {
          const jobSupportsDryRun = (j.steps || []).some(step => !!step.skip_dry_run);
          const jb = document.createElement('div');
          jb.className = 'jobbox';
          const jobHead = document.createElement('div');
          jobHead.className = 'job-head';
          const jobDesc = document.createElement('div');
          jobDesc.className = 'job-desc';
          const jobActions = document.createElement('div');
          jobActions.className = 'job-actions';
          const runsOn = Object.entries(j.runs_on || {}).map(kv => kv[0] + '=' + kv[1]).join(', ');
          const requiresTools = Object.entries(j.requires_tools || {}).map(kv => kv[0] + '=' + (kv[1] || '*')).join(', ');
          jobDesc.innerHTML =
            '<div><strong>Job: ' + escapeHtml(j.id || '') + '</strong> <span class="muted">timeout=' + (j.timeout_seconds || 0) + 's</span></div>' +
            '<div class="muted">runs_on: ' + escapeHtml(runsOn) + '</div>' +
            '<div class="muted">requires.tools: ' + escapeHtml(requiresTools) + '</div>';
          jobHead.appendChild(jobDesc);
          jobHead.appendChild(jobActions);
          jb.appendChild(jobHead);

          const hasMatrixIncludes = Array.isArray(j.matrix_includes) && j.matrix_includes.length > 0;
          const createActionButton = (label, payload, successName, errorPrefix, modalTitle) => {
            const btn = document.createElement('button');
            btn.textContent = label;
            btn.className = 'secondary';
            btn.onclick = async (ev) => {
              btn.disabled = true;
              try {
                const runResult = await runWithOptionalSourceRef(ev, {
                  runPath: '/api/v1/pipelines/' + pl.id + '/run-selection',
                  sourceRefsPath: '/api/v1/pipelines/' + pl.id + '/source-refs',
                  eligibleAgentsPath: '/api/v1/pipelines/' + pl.id + '/eligible-agents',
                  payload: payload,
                  title: modalTitle,
                  subtitle: String(pl.pipeline_id || ''),
                  runLabel: label,
                });
                if (runResult.cancelled) return;
                showQueuedJobsSnackbar((currentProjectName || 'Project') + ' ' + successName + ' started');
                await loadHistory();
              } catch (e) {
                await showAlertDialog({ title: errorPrefix, message: errorPrefix + ': ' + e.message });
              } finally {
                btn.disabled = false;
              }
            };
            return btn;
          };

          if (hasMatrixIncludes) {
            const matrixList = document.createElement('div');
            matrixList.className = 'matrix-list';
            const includes = j.matrix_includes;
            includes.forEach(mi => {
              const item = document.createElement('div');
              item.className = 'matrix-item';
              const name = (mi.name || '').trim() || ('index-' + mi.index);
              const vars = Object.entries(mi.vars || {}).map(kv => kv[0] + '=' + kv[1]).join(', ');
              const info = document.createElement('div');
              info.className = 'matrix-info';
              info.innerHTML = '<div><code>' + escapeHtml(name) + '</code></div><div class="muted">' + escapeHtml(vars) + '</div>';
              const actions = document.createElement('div');
              actions.className = 'matrix-actions';
              const btn = createActionButton('Run', { pipeline_job_id: j.id, matrix_index: mi.index }, name, 'Run selection failed', 'Run Matrix Entry With Source Ref');
              actions.appendChild(btn);
              if (jobSupportsDryRun) {
                const dryBtn = createActionButton('Dry Run', { pipeline_job_id: j.id, matrix_index: mi.index, dry_run: true }, name, 'Dry run selection failed', 'Dry Run Matrix Entry With Source Ref');
                actions.appendChild(dryBtn);
              }
              const previewBtn = document.createElement('button');
              previewBtn.textContent = 'Preview Dry Run';
              previewBtn.className = 'secondary';
              previewBtn.onclick = () => {
                openDryRunPreviewModal({
                  title: 'Preview Matrix Dry Run',
                  subtitle: String(pl.pipeline_id || '') + ' / ' + String(j.id || '') + ' / ' + name,
                  previewPath: '/api/v1/pipelines/' + pl.id + '/dry-run-preview',
                  runPath: '/api/v1/pipelines/' + pl.id + '/run-selection',
                  sourceRefsPath: '/api/v1/pipelines/' + pl.id + '/source-refs',
                  eligibleAgentsPath: '/api/v1/pipelines/' + pl.id + '/eligible-agents',
                  payload: { dry_run: true, pipeline_job_id: j.id, matrix_index: mi.index },
                });
              };
              actions.appendChild(previewBtn);
              const inspectBtn = document.createElement('button');
              inspectBtn.textContent = 'Inspect';
              inspectBtn.className = 'secondary';
              inspectBtn.onclick = () => openProjectInspectModal(
                { pipeline_db_id: pl.id, pipeline_job_id: j.id, matrix_index: mi.index },
                'Job ' + (j.id || ''),
                'Matrix ' + name
              );
              actions.appendChild(inspectBtn);
              item.appendChild(info);
              item.appendChild(actions);
              matrixList.appendChild(item);
            });
            jb.appendChild(matrixList);
          } else {
            const runBtn = createActionButton('Run Job', { pipeline_job_id: j.id }, (j.id || 'job'), 'Run selection failed', 'Run Job With Source Ref');
            jobActions.appendChild(runBtn);
            if (jobSupportsDryRun) {
              const dryBtn = createActionButton('Dry Run Job', { pipeline_job_id: j.id, dry_run: true }, (j.id || 'job'), 'Dry run selection failed', 'Dry Run Job With Source Ref');
              jobActions.appendChild(dryBtn);
            }
            const previewBtn = document.createElement('button');
            previewBtn.textContent = 'Preview Dry Run';
            previewBtn.className = 'secondary';
            previewBtn.onclick = () => {
              openDryRunPreviewModal({
                title: 'Preview Job Dry Run',
                subtitle: String(pl.pipeline_id || '') + ' / ' + String(j.id || ''),
                previewPath: '/api/v1/pipelines/' + pl.id + '/dry-run-preview',
                runPath: '/api/v1/pipelines/' + pl.id + '/run-selection',
                sourceRefsPath: '/api/v1/pipelines/' + pl.id + '/source-refs',
                eligibleAgentsPath: '/api/v1/pipelines/' + pl.id + '/eligible-agents',
                payload: { dry_run: true, pipeline_job_id: j.id },
              });
            };
            jobActions.appendChild(previewBtn);
            const inspectBtn = document.createElement('button');
            inspectBtn.textContent = 'Inspect Job';
            inspectBtn.className = 'secondary';
            inspectBtn.onclick = () => openProjectInspectModal(
              { pipeline_db_id: pl.id, pipeline_job_id: j.id },
              'Job ' + (j.id || ''),
              'Preview raw YAML or rendered executor script'
            );
            jobActions.appendChild(inspectBtn);
          }
          pipelineBody.appendChild(jb);
        });
        container.appendChild(pipelineBody);
        structure.appendChild(container);
      });
    }

    async function loadHistory(force) {
      if (refreshInFlight || (!force && refreshGuard.shouldPause())) {
        return;
      }
      refreshInFlight = true;
      try {
        const data = await apiJSON('/api/v1/jobs');
        const body = document.getElementById('historyBody');
        body.innerHTML = '';
        const projectID = String(currentProjectID || '').trim();
        const rows = (data.job_executions || []).filter(j => {
          const metadata = (j && j.metadata) || {};
          const jobProjectID = String(metadata.project_id || '').trim();
          if (jobProjectID) return jobProjectID === projectID;
          // Backward compatibility for older executions missing project_id metadata.
          return String(metadata.project || '').trim() === currentProjectName;
        }).slice(0, 120);
        rows.forEach(job => {
          const tr = buildJobExecutionRow(job, {
            includeActions: false,
            includeDuration: true,
            backPath: window.location.pathname || '/'
          });
          body.appendChild(tr);
        });
      } finally {
        refreshInFlight = false;
      }
    }

    async function tick() {
      try {
        await refreshRuntimeStateBanner('runtimeStateBanner');
        await loadProject();
        await loadHistory(true);
      } catch (e) {
        document.getElementById('subtitle').textContent = 'Failed to load project: ' + e.message;
      }
    }

    refreshGuard.bindSelectionListener();
    tick();
    setInterval(() => {
      refreshRuntimeStateBanner('runtimeStateBanner');
      loadHistory(false);
    }, 4000);
  </script>
</body>
</html>`
