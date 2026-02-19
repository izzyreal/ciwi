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
    </div>

    <div class="card">
      <h2 style="margin:0 0 10px;">Structure</h2>
      <div id="structure">Loading...</div>
    </div>
    <div class="card">
      <h2 style="margin:0 0 10px;">Vault Access</h2>
      <div class="row" style="margin-bottom:8px;">
        <label for="vaultConnectionSelect" class="muted">Connection:</label>
        <select id="vaultConnectionSelect"></select>
        <button id="saveVaultBtn" class="secondary">Save Vault Settings</button>
        <button id="testVaultBtn" class="secondary">Test</button>
        <span id="vaultMsg" class="muted"></span>
      </div>
      <div class="muted" style="margin-bottom:6px;">Secret mappings (one per line): <code>name=mount/path#key</code></div>
      <textarea id="vaultSecretsText" style="width:100%;min-height:120px;border:1px solid var(--line);border-radius:8px;padding:8px;font-family:ui-monospace,Menlo,Consolas,monospace;font-size:12px;"></textarea>
    </div>

    <div class="card">
      <h2 style="margin:0 0 10px;">Execution History</h2>
      <table>
        <thead>
          <tr><th>Job Execution</th><th>Status</th><th>Pipeline</th><th>Build</th><th>Agent</th><th>Created</th><th>Reason</th></tr>
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

    function projectIdFromPath() {
      const parts = window.location.pathname.split('/').filter(Boolean);
      return parts.length >= 2 ? parts[1] : '';
    }
    let currentProjectName = '';
    let currentProjectID = 0;

    async function loadProject() {
      const id = projectIdFromPath();
      if (!id) return;
      const data = await apiJSON('/api/v1/projects/' + encodeURIComponent(id));
      const p = data.project;
      currentProjectID = p.id;
      currentProjectName = p.name || '';
      document.getElementById('title').textContent = p.name || 'Project';
      document.getElementById('subtitle').innerHTML =
        '<span class="pill">' + escapeHtml(p.repo_url || '') + '</span> ' +
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
        runAll.onclick = async () => {
          runAll.disabled = true;
          try {
            const resp = await apiJSON('/api/v1/pipelines/' + pl.id + '/run-selection', { method: 'POST', body: '{}' });
            showQueuedJobsSnackbar((currentProjectName || 'Project') + ' ' + (pl.pipeline_id || 'pipeline') + ' started');
            await loadHistory();
          } catch (e) {
            alert('Run failed: ' + e.message);
          } finally {
            runAll.disabled = false;
          }
        };
        const dryAll = document.createElement('button');
        dryAll.textContent = 'Dry Run Pipeline';
        dryAll.className = 'secondary';
        dryAll.onclick = async () => {
          dryAll.disabled = true;
          try {
            const resp = await apiJSON('/api/v1/pipelines/' + pl.id + '/run-selection', { method: 'POST', body: JSON.stringify({ dry_run: true }) });
            showQueuedJobsSnackbar((currentProjectName || 'Project') + ' ' + (pl.pipeline_id || 'pipeline') + ' started');
            await loadHistory();
          } catch (e) {
            alert('Dry run failed: ' + e.message);
          } finally {
            dryAll.disabled = false;
          }
        };
        const resolveBtn = document.createElement('button');
        resolveBtn.textContent = 'Resolve Upcoming Build Version';
        resolveBtn.className = 'secondary';
        resolveBtn.onclick = () => openVersionResolveModal(pl.id, pl.pipeline_id);
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
        headControls.appendChild(resolveBtn);
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
          const requiresCaps = Object.entries(j.requires_capabilities || {}).map(kv => kv[0] + '=' + (kv[1] || '*')).join(', ');
          jobDesc.innerHTML =
            '<div><strong>Job: ' + escapeHtml(j.id || '') + '</strong> <span class="muted">timeout=' + (j.timeout_seconds || 0) + 's</span></div>' +
            '<div class="muted">runs_on: ' + escapeHtml(runsOn) + '</div>' +
            '<div class="muted">requires.tools: ' + escapeHtml(requiresTools) + '</div>' +
            '<div class="muted">requires.capabilities: ' + escapeHtml(requiresCaps) + '</div>';
          jobHead.appendChild(jobDesc);
          jobHead.appendChild(jobActions);
          jb.appendChild(jobHead);

          const hasMatrixIncludes = Array.isArray(j.matrix_includes) && j.matrix_includes.length > 0;
          const createActionButton = (label, payload, successName, errorPrefix) => {
            const btn = document.createElement('button');
            btn.textContent = label;
            btn.className = 'secondary';
            btn.onclick = async () => {
              btn.disabled = true;
              try {
                const resp = await apiJSON('/api/v1/pipelines/' + pl.id + '/run-selection', {
                  method: 'POST',
                  body: JSON.stringify(payload)
                });
                showQueuedJobsSnackbar((currentProjectName || 'Project') + ' ' + successName + ' started');
                await loadHistory();
              } catch (e) {
                alert(errorPrefix + ': ' + e.message);
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
              const btn = createActionButton('Run', { pipeline_job_id: j.id, matrix_index: mi.index }, name, 'Run selection failed');
              actions.appendChild(btn);
              if (jobSupportsDryRun) {
                const dryBtn = createActionButton('Dry Run', { pipeline_job_id: j.id, matrix_index: mi.index, dry_run: true }, name, 'Dry run selection failed');
                actions.appendChild(dryBtn);
              }
              item.appendChild(info);
              item.appendChild(actions);
              matrixList.appendChild(item);
            });
            jb.appendChild(matrixList);
          } else {
            const runBtn = createActionButton('Run Job', { pipeline_job_id: j.id }, (j.id || 'job'), 'Run selection failed');
            jobActions.appendChild(runBtn);
            if (jobSupportsDryRun) {
              const dryBtn = createActionButton('Dry Run Job', { pipeline_job_id: j.id, dry_run: true }, (j.id || 'job'), 'Dry run selection failed');
              jobActions.appendChild(dryBtn);
            }
          }
          pipelineBody.appendChild(jb);
        });
        container.appendChild(pipelineBody);
        structure.appendChild(container);
      });
    }

    async function loadVaultSection() {
      const id = projectIdFromPath();
      const select = document.getElementById('vaultConnectionSelect');
      const txt = document.getElementById('vaultSecretsText');
      const conns = await apiJSON('/api/v1/vault/connections');
      select.innerHTML = '<option value="0">(none)</option>';
      (conns.connections || []).forEach(c => {
        const opt = document.createElement('option');
        opt.value = String(c.id);
        opt.textContent = c.name + ' (' + c.url + ')';
        select.appendChild(opt);
      });
      const settingsResp = await apiJSON('/api/v1/projects/' + encodeURIComponent(id) + '/vault');
      const settings = settingsResp.settings || {};
      select.value = String(settings.vault_connection_id || 0);
      const lines = (settings.secrets || []).map(s => {
        const mount = (s.mount || '').trim();
        const prefix = mount ? (mount + '/') : '';
        return s.name + '=' + prefix + s.path + '#' + s.key;
      });
      txt.value = lines.join('\n');
    }

    function parseSecretLines(text) {
      const out = [];
      (text || '').split('\n').map(x => x.trim()).filter(Boolean).forEach(line => {
        const eq = line.indexOf('=');
        const hash = line.lastIndexOf('#');
        if (eq <= 0 || hash <= eq + 1) throw new Error('Invalid mapping line: ' + line);
        const name = line.slice(0, eq).trim();
        const pathPart = line.slice(eq + 1, hash).trim();
        const key = line.slice(hash + 1).trim();
        const slash = pathPart.indexOf('/');
        let mount = '';
        let path = pathPart;
        if (slash > 0) {
          mount = pathPart.slice(0, slash).trim();
          path = pathPart.slice(slash + 1).trim();
        }
        out.push({ name, mount, path, key });
      });
      return out;
    }

    document.getElementById('saveVaultBtn').onclick = async () => {
      const id = projectIdFromPath();
      const msg = document.getElementById('vaultMsg');
      msg.textContent = 'Saving...';
      try {
        const payload = {
          vault_connection_id: Number(document.getElementById('vaultConnectionSelect').value || '0'),
          secrets: parseSecretLines(document.getElementById('vaultSecretsText').value),
        };
        await apiJSON('/api/v1/projects/' + encodeURIComponent(id) + '/vault', { method: 'PUT', body: JSON.stringify(payload) });
        msg.textContent = 'Saved';
      } catch (e) {
        msg.textContent = 'Error: ' + e.message;
      }
    };

    document.getElementById('testVaultBtn').onclick = async () => {
      const id = projectIdFromPath();
      const msg = document.getElementById('vaultMsg');
      msg.textContent = 'Testing...';
      try {
        const r = await apiJSON('/api/v1/projects/' + encodeURIComponent(id) + '/vault-test', { method: 'POST', body: '{}' });
        const details = r.details || {};
        const failures = Object.entries(details).filter(([, v]) => v !== 'ok');
        const suffix = failures.length > 0
          ? (' (' + failures.map(([k, v]) => k + ': ' + v).join('; ') + ')')
          : '';
        msg.textContent = (r.ok ? 'OK: ' : 'FAILED: ') + (r.message || '') + suffix;
      } catch (e) {
        msg.textContent = 'Test error: ' + e.message;
      }
    };

    async function loadHistory(force) {
      if (refreshInFlight || (!force && refreshGuard.shouldPause())) {
        return;
      }
      refreshInFlight = true;
      try {
        const data = await apiJSON('/api/v1/jobs');
        const body = document.getElementById('historyBody');
        body.innerHTML = '';
        const rows = (data.job_executions || []).filter(j => ((j.metadata && j.metadata.project) || '') === currentProjectName).slice(0, 120);
        rows.forEach(job => {
          const tr = buildJobExecutionRow(job, {
            includeActions: false,
            includeReason: true,
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
        await loadProject();
        await loadVaultSection();
        await loadHistory(true);
      } catch (e) {
        document.getElementById('subtitle').textContent = 'Failed to load project: ' + e.message;
      }
    }

    refreshGuard.bindSelectionListener();
    tick();
    setInterval(() => loadHistory(false), 4000);
  </script>
</body>
</html>`
