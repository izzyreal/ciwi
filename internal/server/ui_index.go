package server

const indexHTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="UTF-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>ciwi</title>
  <link rel="icon" type="image/png" href="/ciwi-favicon.png" />
  <style>
` + uiPageChromeCSS + `
    h1 { margin: 0 0 4px; font-size: 28px; }
    h2 { margin: 0 0 12px; font-size: 18px; }
    p { margin: 0 0 10px; color: var(--muted); }
    input {
      border: 1px solid var(--line);
      border-radius: 8px;
      padding: 9px 12px;
      font-size: 14px;
    }
    input { width: 280px; max-width: 100%; }
    .row { display: flex; gap: 8px; flex-wrap: wrap; align-items: center; }
    .header { display: flex; justify-content: space-between; align-items: center; gap: 12px; }
    .header-actions { display: flex; align-items: center; gap: 12px; }
    .project { border-top: 1px solid var(--line); padding-top: 10px; margin-top: 10px; }
    .project-head { display:flex; justify-content: space-between; gap:10px; align-items:center; flex-wrap:wrap; }
    .pipeline { display: flex; justify-content: space-between; gap: 8px; padding: 8px 0; }
    .pipeline-actions { display:flex; flex-direction:column; gap:6px; align-items:flex-end; }
    .pill { font-size: 12px; padding: 2px 8px; border-radius: 999px; background: #edf8f2; color: #26644b; }
    table { width: 100%; border-collapse: collapse; font-size: 13px; table-layout: fixed; }
    th, td {
      border-bottom: 1px solid var(--line);
      text-align: left;
      padding: 8px 6px;
      vertical-align: top;
      overflow-wrap: anywhere;
      word-break: break-word;
    }
    td code { white-space: pre-wrap; max-height: 80px; overflow: auto; display: block; max-width: 100%; overflow-wrap: anywhere; word-break: break-word; }
    .status-succeeded { color: var(--ok); font-weight: 600; }
    .status-failed { color: var(--bad); font-weight: 600; }
    .status-running { color: #a56a00; font-weight: 600; }
    .status-queued, .status-leased { color: var(--muted); }
    a.job-link { color: var(--accent); }
    @media (max-width: 760px) { table { font-size: 12px; } }
  </style>
</head>
<body>
  <main>
    <div class="card">
      <div class="header">
        <div class="brand">
          <img src="/ciwi-logo.png" alt="ciwi logo" />
          <div>
            <h1>ciwi</h1>
            <p>Projects, pipelines and job executions</p>
          </div>
        </div>
        <div class="header-actions">
          <a class="nav-btn" href="/agents">Agents <span class="nav-emoji" aria-hidden="true">🖥️</span></a>
          <a class="nav-btn" href="/settings" aria-label="Global Settings" title="Global Settings">Global Settings <span class="nav-emoji" aria-hidden="true">⚙️</span></a>
        </div>
      </div>
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
          <tr><th>Job Execution</th><th>Status</th><th>Pipeline</th><th>Build</th><th>Agent</th><th>Created</th><th>Reason</th><th>Actions</th></tr>
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
          <tr><th>Job Execution</th><th>Status</th><th>Pipeline</th><th>Build</th><th>Agent</th><th>Created</th><th>Reason</th></tr>
        </thead>
        <tbody id="historyJobsBody"></tbody>
      </table>
    </div>
  </main>
  <script src="/ui/shared.js"></script>
  <script src="/ui/pages.js"></script>
  <script>
    let refreshInFlight = false;
    const refreshGuard = createRefreshGuard(5000);
    let jobsRenderEpoch = 0;
    const JOBS_WINDOW = 150;
    const JOBS_BATCH_SIZE = 5;

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
        top.appendChild(topInfo);
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
          const resolveBtn = document.createElement('button');
          resolveBtn.className = 'secondary';
          resolveBtn.textContent = 'Resolve Upcoming Build Version';
          resolveBtn.onclick = () => openVersionResolveModal(p.id, p.pipeline_id);
          row.appendChild(info);
          const actions = document.createElement('div');
          actions.className = 'pipeline-actions';
          const btnRow = document.createElement('div');
          btnRow.className = 'row';
          btnRow.appendChild(btn);
          btnRow.appendChild(dryBtn);
          btnRow.appendChild(resolveBtn);
          actions.appendChild(btnRow);
          row.appendChild(actions);
          wrap.appendChild(row);
        });
        root.appendChild(wrap);
      });
    }
    function allocateJobSkeletonRows(tbody, count, columnCount, emptyText) {
      if (count <= 0) {
        const existing = tbody.querySelector('.ciwi-empty-row');
        if (existing && tbody.children.length === 1) {
          return;
        }
        tbody.innerHTML = '';
        const tr = document.createElement('tr');
        tr.className = 'ciwi-empty-row';
        tr.innerHTML = '<td colspan="' + String(columnCount) + '" class="muted">' + escapeHtml(emptyText) + '</td>';
        tbody.appendChild(tr);
        return;
      }
      if (tbody.children.length === 1 && tbody.firstElementChild && tbody.firstElementChild.classList.contains('ciwi-empty-row')) {
        tbody.innerHTML = '';
      }
      while (tbody.children.length < count) {
        tbody.appendChild(buildJobSkeletonRow(columnCount));
      }
      while (tbody.children.length > count) {
        tbody.removeChild(tbody.lastElementChild);
      }
    }

    function replaceJobRowAt(tbody, index, row) {
      const current = tbody.children[index];
      if (current) {
        const currentKey = (current.dataset && current.dataset.ciwiRenderKey) || '';
        const nextKey = (row.dataset && row.dataset.ciwiRenderKey) || '';
        const isSkeleton = current.classList && current.classList.contains('ciwi-job-skeleton-row');
        if (!isSkeleton && currentKey !== '' && currentKey === nextKey) {
          return;
        }
        tbody.replaceChild(row, current);
        if (isSkeleton) {
          fadeInJobRow(row);
        }
      } else {
        tbody.appendChild(row);
        fadeInJobRow(row);
      }
    }

    async function fetchAndRenderJobList(view, tbody, total, opts, epoch) {
      for (let offset = 0; offset < total; offset += JOBS_BATCH_SIZE) {
        if (epoch !== jobsRenderEpoch) return;
        const data = await apiJSON('/api/v1/jobs?view=' + encodeURIComponent(view) +
          '&max=' + String(JOBS_WINDOW) +
          '&offset=' + String(offset) +
          '&limit=' + String(JOBS_BATCH_SIZE));
        if (epoch !== jobsRenderEpoch) return;
        const jobs = data.job_executions || data.jobs || [];
        jobs.forEach((job, i) => {
          const row = buildJobExecutionRow(job, opts);
          replaceJobRowAt(tbody, offset + i, row);
        });
      }
    }

    async function refreshJobs() {
      const epoch = ++jobsRenderEpoch;
      const queuedBody = document.getElementById('queuedJobsBody');
      const historyBody = document.getElementById('historyJobsBody');
      const summary = await apiJSON('/api/v1/jobs?view=summary&max=' + String(JOBS_WINDOW));
      if (epoch !== jobsRenderEpoch) return;

      const queuedTotal = Number(summary.queued_count || 0);
      const historyTotal = Number(summary.history_count || 0);
      allocateJobSkeletonRows(queuedBody, queuedTotal, 8, 'No queued jobs.');
      allocateJobSkeletonRows(historyBody, historyTotal, 7, 'No job history yet.');

      const queuedOpts = {
        includeActions: true,
        includeReason: true,
        fixedLines: 2,
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
      };
      const historyOpts = {
        includeActions: false,
        includeReason: true,
        fixedLines: 2,
        backPath: window.location.pathname || '/',
        linkClass: 'job-link'
      };

      await Promise.all([
        fetchAndRenderJobList('queued', queuedBody, queuedTotal, queuedOpts, epoch),
        fetchAndRenderJobList('history', historyBody, historyTotal, historyOpts, epoch),
      ]);
    }
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
    async function tick() {
      if (refreshInFlight || refreshGuard.shouldPause()) {
        return;
      }
      refreshInFlight = true;
      try {
        await Promise.all([refreshProjects(), refreshJobs()]);
      } catch (e) {
        console.error(e);
      } finally {
        refreshInFlight = false;
      }
    }
    refreshGuard.bindSelectionListener();
    tick();
    setInterval(tick, 3000);
  </script>
</body>
</html>`
