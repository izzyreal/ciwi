package server

const indexHTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="UTF-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>ciwi</title>
  <link rel="icon" type="image/png" href="/ciwi-favicon.png" />
  <style>
    :root {
      --bg: #f2f7f4;
      --bg2: #d9efe2;
      --card: #ffffff;
      --ink: #1f2a24;
      --muted: #5f6f67;
      --ok: #1f8a4c;
      --bad: #b23a48;
      --accent: #157f66;
      --line: #c4ddd0;
    }
    * { box-sizing: border-box; }
    body {
      margin: 0;
      font-family: "Avenir Next", "Segoe UI", sans-serif;
      color: var(--ink);
      background: radial-gradient(circle at 20% 0%, var(--bg2), var(--bg));
    }
    main { max-width: 1100px; margin: 24px auto; padding: 0 16px; }
    .card {
      background: var(--card);
      border: 1px solid var(--line);
      border-radius: 12px;
      padding: 16px;
      margin-bottom: 16px;
      box-shadow: 0 8px 24px rgba(21,127,102,.08);
    }
    h1 { margin: 0 0 4px; font-size: 28px; }
    h2 { margin: 0 0 12px; font-size: 18px; }
    p { margin: 0 0 10px; color: var(--muted); }
    input, button {
      border: 1px solid var(--line);
      border-radius: 8px;
      padding: 9px 12px;
      font-size: 14px;
    }
    input { width: 280px; max-width: 100%; }
    button {
      background: var(--accent);
      color: white;
      border-color: var(--accent);
      cursor: pointer;
    }
    button.secondary { background: white; color: var(--accent); }
    .row { display: flex; gap: 8px; flex-wrap: wrap; align-items: center; }
    .brand { display: flex; align-items: center; gap: 12px; }
    .brand img {
      width: 110px;
      height: 91px;
      object-fit: contain;
      display: block;
      image-rendering: crisp-edges;
      image-rendering: pixelated;
    }
    .project { border-top: 1px solid var(--line); padding-top: 10px; margin-top: 10px; }
    .project-head { display:flex; justify-content: space-between; gap:10px; align-items:center; flex-wrap:wrap; }
    .pipeline { display: flex; justify-content: space-between; gap: 8px; padding: 8px 0; }
    .pill { font-size: 12px; padding: 2px 8px; border-radius: 999px; background: #edf8f2; color: #26644b; }
    table { width: 100%; border-collapse: collapse; font-size: 13px; table-layout: fixed; }
    th, td { border-bottom: 1px solid var(--line); text-align: left; padding: 8px 6px; vertical-align: top; }
    td code { white-space: pre-wrap; max-height: 80px; overflow: auto; display: block; max-width: 100%; overflow-wrap: anywhere; word-break: break-word; }
    .status-succeeded { color: var(--ok); font-weight: 600; }
    .status-failed { color: var(--bad); font-weight: 600; }
    .status-running { color: #a56a00; font-weight: 600; }
    .status-queued, .status-leased { color: var(--muted); }
    a.job-link { color: var(--accent); text-decoration: none; }
    a.job-link:hover { text-decoration: underline; }
    @media (max-width: 760px) { table { font-size: 12px; } }
  </style>
</head>
<body>
  <main>
    <div class="card">
      <div class="brand">
        <img src="/ciwi-logo.png" alt="ciwi logo" />
        <div>
          <h1>ciwi</h1>
          <p>Projects, pipelines and jobs</p>
        </div>
      </div>
      <div class="row">
        <input id="repoUrl" placeholder="https://github.com/you/project.git" style="width:380px" />
        <input id="repoRef" placeholder="ref (optional: main, tag, sha)" />
        <input id="configFile" value="ciwi-project.yaml" />
        <button id="importProjectBtn">Add Project</button>
        <a class="job-link" href="/vault">Vault Connections</a>
        <a class="job-link" href="/agents">Agents</a>
        <span id="importResult"></span>
      </div>
      <div class="row" style="margin-top:10px;">
        <button id="checkUpdatesBtn" class="secondary">Check for updates</button>
        <button id="applyUpdateBtn" class="secondary">Update now</button>
        <span id="updateResult" style="color:#5f6f67;"></span>
      </div>
      <div id="updateStatus" style="margin-top:8px;color:#5f6f67;font-size:12px;"></div>
    </div>
    <div class="card">
      <h2>Projects</h2>
      <div id="projects"></div>
    </div>
    <div class="card">
      <h2>Queued Job Executions</h2>
      <div class="row" style="margin-bottom:10px;">
        <button id="clearQueueBtn" class="secondary">Clear Queue</button>
      </div>
      <table>
        <thead>
          <tr><th>Job Execution</th><th>Status</th><th>Pipeline</th><th>Build</th><th>Agent</th><th>Created</th><th>Actions</th></tr>
        </thead>
        <tbody id="queuedJobsBody"></tbody>
      </table>
    </div>
    <div class="card">
      <h2>Job Execution History</h2>
      <div class="row" style="margin-bottom:10px;">
        <button id="flushHistoryBtn" class="secondary">Flush History</button>
      </div>
      <table>
        <thead>
          <tr><th>Job Execution</th><th>Status</th><th>Pipeline</th><th>Build</th><th>Agent</th><th>Created</th></tr>
        </thead>
        <tbody id="historyJobsBody"></tbody>
      </table>
    </div>
  </main>
  <script src="/ui/shared.js"></script>
  <script src="/ui/pages.js"></script>
  <script>
    const projectReloadState = new Map();
    function setProjectReloadState(projectId, text, color) {
      projectReloadState.set(String(projectId), { text, color });
    }
    async function refreshProjects() {
      const data = await apiJSON('/api/v1/projects');
      const root = document.getElementById('projects');
      if (!data.projects || data.projects.length === 0) {
        root.innerHTML = '<p>No projects loaded yet.</p>';
        return;
      }
      root.innerHTML = '';
      data.projects.forEach(project => {
        const wrap = document.createElement('div');
        wrap.className = 'project';
        const top = document.createElement('div');
        top.className = 'project-head';
        const topInfo = document.createElement('div');
        topInfo.innerHTML = '<strong>Project: <a class="job-link" href="/projects/' + project.id + '">' + project.name + '</a></strong> <span class="pill">' + (project.repo_url || '') + '</span> <span class="pill">' + (project.config_file || project.config_path || '') + '</span>';
        const reloadStatus = document.createElement('span');
        reloadStatus.style.fontSize = '12px';
        const state = projectReloadState.get(String(project.id));
        if (state) {
          reloadStatus.textContent = state.text;
          reloadStatus.style.color = state.color;
        } else {
          reloadStatus.style.color = '#5f6f67';
        }
        const reloadBtn = document.createElement('button');
        reloadBtn.className = 'secondary';
        reloadBtn.textContent = 'Reload project definition from VCS';
        reloadBtn.onclick = async () => {
          setProjectReloadState(project.id, 'Reloading...', '#5f6f67');
          reloadStatus.textContent = 'Reloading...';
          reloadStatus.style.color = '#5f6f67';
            reloadBtn.disabled = true;
          try {
            await apiJSON('/api/v1/projects/' + project.id + '/reload', { method: 'POST', body: '{}' });
            await Promise.all([refreshProjects(), refreshJobs()]);
            setProjectReloadState(project.id, 'Reloaded successfully', '#1f8a4c');
            reloadStatus.textContent = 'Reloaded successfully';
            reloadStatus.style.color = '#1f8a4c';
          } catch (e) {
            const msg = 'Reload failed: ' + e.message;
            setProjectReloadState(project.id, msg, '#b23a48');
            reloadStatus.textContent = msg;
            reloadStatus.style.color = '#b23a48';
          } finally {
            reloadBtn.disabled = false;
          }
        };
        top.appendChild(topInfo);
        const controls = document.createElement('div');
        controls.className = 'row';
        controls.appendChild(reloadBtn);
        controls.appendChild(reloadStatus);
        top.appendChild(controls);
        wrap.appendChild(top);
        (project.pipelines || []).forEach(p => {
          const row = document.createElement('div');
          row.className = 'pipeline';
          const deps = (p.depends_on || []).join(', ');
          const info = document.createElement('div');
          info.innerHTML = '<div><span class="muted">Pipeline:</span> <code>' + p.pipeline_id + '</code></div><div style="color:#5f6f67;font-size:12px;">' +
            (p.source_repo || '') + (deps ? (' | depends_on: ' + deps) : '') + '</div>';
          const btn = document.createElement('button');
          btn.className = 'secondary';
          btn.textContent = 'Run';
          btn.onclick = async () => {
            btn.disabled = true;
            try {
              await apiJSON('/api/v1/pipelines/' + p.id + '/run', { method: 'POST', body: '{}' });
              await refreshJobs();
            } catch (e) {
              alert('Run failed: ' + e.message);
            } finally {
              btn.disabled = false;
            }
          };
          const dryBtn = document.createElement('button');
          dryBtn.className = 'secondary';
          dryBtn.textContent = 'Dry Run';
          dryBtn.onclick = async () => {
            dryBtn.disabled = true;
            try {
              await apiJSON('/api/v1/pipelines/' + p.id + '/run', { method: 'POST', body: JSON.stringify({ dry_run: true }) });
              await refreshJobs();
            } catch (e) {
              alert('Dry run failed: ' + e.message);
            } finally {
              dryBtn.disabled = false;
            }
          };
          row.appendChild(info);
          const actions = document.createElement('div');
          actions.className = 'row';
          actions.appendChild(btn);
          actions.appendChild(dryBtn);
          row.appendChild(actions);
          wrap.appendChild(row);
        });
        root.appendChild(wrap);
      });
    }
    async function refreshJobs() {
      const data = await apiJSON('/api/v1/jobs');
      const queuedBody = document.getElementById('queuedJobsBody');
      const historyBody = document.getElementById('historyJobsBody');
      queuedBody.innerHTML = '';
      historyBody.innerHTML = '';
      const queuedStatuses = new Set(['queued', 'leased', 'running']);
      const jobs = (data.job_executions || data.jobs || []).slice(0, 150);
      const queuedJobs = jobs.filter(job => queuedStatuses.has((job.status || '').toLowerCase()));
      const historyJobs = jobs.filter(job => !queuedStatuses.has((job.status || '').toLowerCase()));
      queuedJobs.forEach(job => queuedBody.appendChild(buildJobExecutionRow(job, {
        includeActions: true,
        backPath: window.location.pathname || '/',
        linkClass: 'job-link',
        onRemove: async (j) => {
          try {
            await apiJSON('/api/v1/jobs/' + j.id, { method: 'DELETE' });
            await refreshJobs();
          } catch (e) {
            alert('Remove failed: ' + e.message);
          }
        }
      })));
      historyJobs.forEach(job => historyBody.appendChild(buildJobExecutionRow(job, {
        includeActions: false,
        backPath: window.location.pathname || '/',
        linkClass: 'job-link'
      })));
    }
    document.getElementById('importProjectBtn').onclick = async () => {
      const repoUrl = (document.getElementById('repoUrl').value || '').trim();
      const repoRef = (document.getElementById('repoRef').value || '').trim();
      const configFile = (document.getElementById('configFile').value || 'ciwi-project.yaml').trim();
      const result = document.getElementById('importResult');
      if (!repoUrl) {
        result.textContent = 'Repo URL required';
        return;
      }
      result.textContent = 'Importing...';
      try {
        await apiJSON('/api/v1/projects/import', {
          method: 'POST',
          body: JSON.stringify({ repo_url: repoUrl, repo_ref: repoRef, config_file: configFile }),
        });
        result.textContent = 'Imported';
        await Promise.all([refreshProjects(), refreshJobs()]);
      } catch (e) {
        result.textContent = 'Error: ' + e.message;
      }
    };
    document.getElementById('clearQueueBtn').onclick = async () => {
      if (!confirm('Clear all queued/leased jobs?')) {
        return;
      }
      try {
        await apiJSON('/api/v1/jobs/clear-queue', { method: 'POST', body: '{}' });
        await refreshJobs();
      } catch (e) {
        alert('Clear queue failed: ' + e.message);
      }
    };
    document.getElementById('flushHistoryBtn').onclick = async () => {
      if (!confirm('Flush all finished jobs from history?')) {
        return;
      }
      try {
        await apiJSON('/api/v1/jobs/flush-history', { method: 'POST', body: '{}' });
        await refreshJobs();
      } catch (e) {
        alert('Flush history failed: ' + e.message);
      }
    };
    document.getElementById('checkUpdatesBtn').onclick = async () => {
      const result = document.getElementById('updateResult');
      result.textContent = 'Checking...';
      try {
        const r = await apiJSON('/api/v1/update/check', { method: 'POST', body: '{}' });
        const latest = r.latest_version || '';
        const current = r.current_version || '';
        if (r.update_available) {
          result.textContent = 'Update available: ' + current + ' -> ' + latest + (r.asset_name ? (' (' + r.asset_name + ')') : '');
        } else {
          result.textContent = r.message || ('Up to date (' + current + ')');
        }
      } catch (e) {
        result.textContent = 'Update check failed: ' + e.message;
      }
      await refreshUpdateStatus();
    };
    document.getElementById('applyUpdateBtn').onclick = async () => {
      const result = document.getElementById('updateResult');
      if (!confirm('Apply update now and restart ciwi?')) return;
      result.textContent = 'Starting update...';
      try {
        const r = await apiJSON('/api/v1/update/apply', { method: 'POST', body: '{}' });
        result.textContent = (r.message || 'Update started. Refresh in a moment.');
      } catch (e) {
        result.textContent = 'Update failed: ' + e.message;
      }
      await refreshUpdateStatus();
    };
    async function refreshUpdateStatus() {
      const box = document.getElementById('updateStatus');
      try {
        const r = await apiJSON('/api/v1/update/status');
        const s = r.status || {};
        const parts = [];
        if (s.update_current_version) parts.push('Current: ' + s.update_current_version);
        if (s.update_latest_version) parts.push('Latest: ' + s.update_latest_version);
        if (s.update_available === '1') parts.push('Update available');
        if (s.update_last_checked_utc) parts.push('Checked: ' + formatTimestamp(s.update_last_checked_utc));
        if (s.update_last_apply_status) parts.push('Apply: ' + s.update_last_apply_status);
        if (s.update_last_apply_utc) parts.push('Apply time: ' + formatTimestamp(s.update_last_apply_utc));
        if (s.update_message) parts.push('Message: ' + s.update_message);
        box.textContent = parts.join(' | ');
      } catch (e) {
        box.textContent = 'Update status unavailable';
      }
    }
    async function tick() {
      try {
        await Promise.all([refreshProjects(), refreshJobs(), refreshUpdateStatus()]);
      } catch (e) {
        console.error(e);
      }
    }
    tick();
    setInterval(refreshJobs, 3000);
    setInterval(refreshProjects, 7000);
    setInterval(refreshUpdateStatus, 3000);
  </script>
</body>
</html>`
