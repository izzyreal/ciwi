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
    .project-group {
      border: 1px solid var(--line);
      border-radius: 12px;
      margin-top: 10px;
      background: #fff;
      overflow: hidden;
    }
    .project-group > summary {
      list-style: none;
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: 12px;
      cursor: pointer;
      padding: 10px 12px;
      background: #f7fcf9;
    }
    .project-group > summary::-webkit-details-marker { display: none; }
    .project-group-toggle {
      color: var(--muted);
      font-size: 14px;
      line-height: 1;
      min-width: 14px;
      text-align: center;
    }
    .project-group[open] .project-group-toggle::before { content: "▾"; }
    .project-group:not([open]) .project-group-toggle::before { content: "▸"; }
    .project-head { display:flex; justify-content: space-between; gap:10px; align-items:center; flex-wrap:wrap; min-width:0; flex:1 1 auto; }
    .project-body {
      margin-top: 0;
      padding: 8px 12px 10px;
      border-top: 1px solid var(--line);
    }
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
    table th:first-child,
    table td:first-child { padding-left: 10px; }
    td code { white-space: pre-wrap; max-height: 80px; overflow: auto; display: block; max-width: 100%; overflow-wrap: anywhere; word-break: break-word; }
    .status-succeeded { color: var(--ok); font-weight: 600; }
    .status-failed { color: var(--bad); font-weight: 600; }
    .status-running { color: #a56a00; font-weight: 600; }
    .status-queued, .status-leased { color: var(--muted); }
    .ciwi-job-group-row td { padding: 4px 0; border-bottom: none; }
    .ciwi-job-group-details {
      margin: 0;
      border: 1px solid var(--line);
      border-radius: 10px;
      overflow: hidden;
      background: #fff;
    }
    .ciwi-job-group-details > summary {
      list-style: none;
      display: flex;
      align-items: center;
      justify-content: flex-start;
      gap: 10px;
      cursor: pointer;
      background: #f7fcf9;
      padding: 10px 10px;
    }
    .ciwi-job-group-details > summary::-webkit-details-marker { display: none; }
    .ciwi-job-group-toggle {
      color: var(--muted);
      font-size: 14px;
      line-height: 1;
      min-width: 14px;
      text-align: center;
      flex: 0 0 auto;
    }
    .ciwi-job-group-details[open] .ciwi-job-group-toggle::before { content: "▾"; }
    .ciwi-job-group-details:not([open]) .ciwi-job-group-toggle::before { content: "▸"; }
    .ciwi-job-group-main {
      display: inline-flex;
      align-items: center;
      gap: 8px;
      min-width: 0;
      font-weight: 600;
      flex: 1 1 auto;
    }
    .ciwi-job-group-title {
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
    }
    .ciwi-job-group-emoji { font-size: 15px; line-height: 1; }
    .ciwi-job-group-status { font-size: 12px; white-space: nowrap; flex: 0 0 auto; }
    .ciwi-job-group-table { width: 100%; border-collapse: collapse; }
    .ciwi-job-group-card {
      margin: 0;
      border: 1px solid var(--line);
      border-radius: 10px;
      overflow: hidden;
      background: #fff;
    }
    .ciwi-job-group-head {
      display: flex;
      align-items: center;
      justify-content: flex-start;
      gap: 10px;
      background: #f7fcf9;
      padding: 10px 10px;
    }
    .ciwi-job-group-skel-head {
      display: flex;
      align-items: center;
      justify-content: flex-start;
      gap: 10px;
      background: #f7fcf9;
      padding: 10px 10px;
    }
    .ciwi-job-group-skel-body {
      padding: 8px 10px;
      border-top: 1px solid var(--line);
      display: grid;
      gap: 8px;
    }
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
    let lastQueuedJobsSignature = '';
    let lastHistoryJobsSignature = '';
    const PROJECT_GROUPS_STORAGE_KEY = 'ciwi.index.projectGroupsCollapsed.v1';
    const JOB_GROUPS_STORAGE_KEY = 'ciwi.index.jobGroupsExpanded.v1';
    function loadStringSet(key) {
      try {
        const raw = localStorage.getItem(key);
        if (!raw) return new Set();
        const parsed = JSON.parse(raw);
        if (!Array.isArray(parsed)) return new Set();
        return new Set(parsed.map(v => String(v || '')));
      } catch (_) {
        return new Set();
      }
    }
    function saveStringSet(key, values) {
      try {
        localStorage.setItem(key, JSON.stringify(Array.from(values || [])));
      } catch (_) {}
    }
    const projectGroupCollapsed = loadStringSet(PROJECT_GROUPS_STORAGE_KEY);
    const expandedJobGroups = loadStringSet(JOB_GROUPS_STORAGE_KEY);
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
        const projectKey = String(project.id || project.name || '');
        const details = document.createElement('details');
        details.className = 'project-group';
        details.open = !projectGroupCollapsed.has(projectKey);
        const summary = document.createElement('summary');
        const top = document.createElement('div');
        top.className = 'project-head';
        const topInfo = document.createElement('div');
        topInfo.innerHTML = '<strong>Project: <a class="job-link" href="/projects/' + project.id + '">' + project.name + '</a></strong> <span class="pill">' + (project.repo_url || '') + '</span> <span class="pill">' + (project.config_file || project.config_path || '') + '</span>';
        const topRight = document.createElement('div');
        topRight.innerHTML = '<span class="pill">' + String((project.pipelines || []).length) + ' pipeline(s)</span>';
        top.appendChild(topInfo);
        top.appendChild(topRight);
        summary.appendChild(top);
        const toggle = document.createElement('span');
        toggle.className = 'project-group-toggle';
        toggle.setAttribute('aria-hidden', 'true');
        summary.appendChild(toggle);
        details.appendChild(summary);
        const body = document.createElement('div');
        body.className = 'project-body';
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
          body.appendChild(row);
        });
        details.appendChild(body);
        details.addEventListener('toggle', () => {
          if (details.open) {
            projectGroupCollapsed.delete(projectKey);
          } else {
            projectGroupCollapsed.add(projectKey);
          }
          saveStringSet(PROJECT_GROUPS_STORAGE_KEY, projectGroupCollapsed);
        });
        root.appendChild(details);
      });
    }
    function normalizeSummaryGroups(rawGroups, fallbackCount) {
      if (Array.isArray(rawGroups) && rawGroups.length > 0) {
        return rawGroups.map((g, idx) => {
          const runID = String((g && g.run_id) || '').trim();
          const key = String((g && g.key) || (runID ? ('run:' + runID) : ('idx:' + idx))).trim();
          const jobCount = Math.max(1, Number((g && g.job_count) || 1) || 1);
          const collapsible = !!((g && g.collapsible) || (runID && jobCount > 1));
          return { key: key, run_id: runID, job_count: jobCount, collapsible: collapsible };
        });
      }
      const count = Math.max(0, Number(fallbackCount || 0));
      const out = [];
      for (let i = 0; i < count; i += 1) {
        out.push({ key: 'fallback:' + i, run_id: '', job_count: 1, collapsible: false });
      }
      return out;
    }

    function buildJobGroupSkeletonRow(spec, viewKey, columnCount) {
      ensureJobSkeletonStyles();
      const tr = document.createElement('tr');
      tr.className = 'ciwi-job-group-row';
      const td = document.createElement('td');
      td.colSpan = columnCount;

      const isCollapsible = !!spec.collapsible;
      const runID = String(spec.run_id || '').trim();
      const groupKey = runID ? (viewKey + ':' + runID) : '';
      const expanded = isCollapsible && groupKey !== '' && expandedJobGroups.has(groupKey);
      const root = document.createElement('div');
      root.className = isCollapsible ? 'ciwi-job-group-details' : 'ciwi-job-group-card';
      const head = document.createElement('div');
      head.className = isCollapsible ? 'ciwi-job-group-skel-head' : 'ciwi-job-group-head';
      head.innerHTML =
        '<span class="ciwi-job-group-main">' +
          (isCollapsible ? '<span class="ciwi-job-group-toggle" aria-hidden="true">' + (expanded ? '▾' : '▸') + '</span>' : '') +
          '<span class="ciwi-job-group-emoji" aria-hidden="true">⏳</span>' +
          '<span class="ciwi-job-group-title"><span class="ciwi-job-skeleton-bar" style="width:180px;display:inline-block;"></span></span>' +
        '</span>' +
        '<span class="ciwi-job-group-status status-queued"><span class="ciwi-job-skeleton-bar ciwi-job-skeleton-bar-short" style="width:110px;display:inline-block;"></span></span>';
      root.appendChild(head);

      const showBody = !isCollapsible || expanded;
      if (showBody) {
        const body = document.createElement('div');
        body.className = 'ciwi-job-group-skel-body';
        const rows = Math.max(1, Number(spec.job_count || 1) || 1);
        for (let i = 0; i < rows; i += 1) {
          const line = document.createElement('div');
          line.className = 'ciwi-job-skeleton-lines';
          line.innerHTML = '<div class="ciwi-job-skeleton-bar"></div><div class="ciwi-job-skeleton-bar ciwi-job-skeleton-bar-short"></div>';
          body.appendChild(line);
        }
        root.appendChild(body);
      }

      td.appendChild(root);
      tr.appendChild(td);
      return tr;
    }

    function allocateGroupedSkeletonRows(tbody, groups, columnCount, emptyText, viewKey) {
      const specs = Array.isArray(groups) ? groups : [];
      if (specs.length === 0) {
        const existing = tbody.querySelector('.ciwi-empty-row');
        if (existing && tbody.children.length === 1) return;
        tbody.innerHTML = '';
        const tr = document.createElement('tr');
        tr.className = 'ciwi-empty-row';
        tr.innerHTML = '<td colspan="' + String(columnCount) + '" class="muted">' + escapeHtml(emptyText) + '</td>';
        tbody.appendChild(tr);
        return;
      }
      tbody.innerHTML = '';
      specs.forEach(spec => {
        tbody.appendChild(buildJobGroupSkeletonRow(spec, viewKey, columnCount));
      });
    }

    function tbodyHasConcreteRows(tbody) {
      if (!tbody) return false;
      const rows = Array.from(tbody.children || []);
      if (rows.length === 0) return false;
      return rows.some(row => {
        if (!row || !row.classList) return true;
        return !row.classList.contains('ciwi-job-skeleton-row') && !row.classList.contains('ciwi-empty-row');
      });
    }

    function jobsSignature(jobs) {
      if (!Array.isArray(jobs) || jobs.length === 0) return '';
      return jobs.map(job => {
        const runID = String((job && job.metadata && job.metadata.pipeline_run_id) || '').trim();
        return jobRowRenderKey(job) + '\x1e' + runID;
      }).join('\x1f');
    }

    async function fetchJobList(view, total, epoch) {
      if (total <= 0) return [];
      const out = [];
      for (let offset = 0; offset < total; offset += JOBS_BATCH_SIZE) {
        if (epoch !== jobsRenderEpoch) return null;
        const data = await apiJSON('/api/v1/jobs?view=' + encodeURIComponent(view) +
          '&max=' + String(JOBS_WINDOW) +
          '&offset=' + String(offset) +
          '&limit=' + String(JOBS_BATCH_SIZE));
        if (epoch !== jobsRenderEpoch) return null;
        const jobs = data.job_executions || data.jobs || [];
        out.push(...jobs);
      }
      return out;
    }

    function summarizeJobGroup(jobs) {
      const total = jobs.length;
      let succeeded = 0;
      let failed = 0;
      let inProgress = 0;
      jobs.forEach(job => {
        const status = String(job.status || '').toLowerCase();
        if (status === 'succeeded') succeeded += 1;
        else if (status === 'failed') failed += 1;
        else if (status === 'queued' || status === 'leased' || status === 'running') inProgress += 1;
      });
      if (failed > 0) {
        return { emoji: '❌', cls: 'status-failed', text: succeeded + '/' + total + ' successful, ' + failed + ' failed' };
      }
      if (inProgress > 0) {
        return { emoji: '⏳', cls: 'status-running', text: succeeded + '/' + total + ' successful, ' + inProgress + ' in progress' };
      }
      if (succeeded === total) {
        return { emoji: '✅', cls: 'status-succeeded', text: succeeded + '/' + total + ' successful' };
      }
      return { emoji: '🟡', cls: 'status-queued', text: succeeded + '/' + total + ' successful' };
    }

    function jobGroupLabel(jobs) {
      if (!jobs || jobs.length === 0) return 'pipeline run';
      const first = jobs[0] || {};
      const meta = first.metadata || {};
      const projectName = String(meta.project || '').trim();
      const pipelineID = String(meta.pipeline_id || '').trim();
      let earliest = '';
      jobs.forEach(job => {
        const ts = String(job.created_utc || '').trim();
        if (!ts) return;
        if (!earliest || ts < earliest) {
          earliest = ts;
        }
      });
      const when = earliest ? formatTimestamp(earliest) : '';
      const parts = [];
      if (projectName) parts.push(projectName);
      if (pipelineID) parts.push(pipelineID);
      const base = parts.join(' ');
      if (base && when) return base + ' ' + when;
      if (base) return base;
      if (when) return when;
      return 'job';
    }

    function buildStaticJobGroupRow(job, opts, columnCount) {
      const status = summarizeJobGroup([job]);
      const groupTitle = jobGroupLabel([job]);
      const tr = document.createElement('tr');
      tr.className = 'ciwi-job-group-row';
      const td = document.createElement('td');
      td.colSpan = columnCount;

      const card = document.createElement('div');
      card.className = 'ciwi-job-group-card';
      const head = document.createElement('div');
      head.className = 'ciwi-job-group-head';
      head.innerHTML = '<span class="ciwi-job-group-main"><span class="ciwi-job-group-emoji" aria-hidden="true">' + status.emoji +
        '</span><span class="ciwi-job-group-title">' + escapeHtml(groupTitle) +
        '</span></span><span class="ciwi-job-group-status ' + status.cls + '">' + escapeHtml(status.text) + '</span>';
      card.appendChild(head);

      const table = document.createElement('table');
      table.className = 'ciwi-job-group-table';
      const body = document.createElement('tbody');
      body.appendChild(buildJobExecutionRow(job, opts));
      table.appendChild(body);
      card.appendChild(table);

      td.appendChild(card);
      tr.appendChild(td);
      return tr;
    }

    function renderGroupedJobs(tbody, jobs, opts, viewKey, columnCount, emptyText) {
      tbody.innerHTML = '';
      if (!jobs || jobs.length === 0) {
        const tr = document.createElement('tr');
        tr.className = 'ciwi-empty-row';
        tr.innerHTML = '<td colspan="' + String(columnCount) + '" class="muted">' + escapeHtml(emptyText) + '</td>';
        tbody.appendChild(tr);
        return;
      }
      const jobsByRun = new Map();
      jobs.forEach(job => {
        const runID = String((job.metadata && job.metadata.pipeline_run_id) || '').trim();
        if (!runID) return;
        if (!jobsByRun.has(runID)) jobsByRun.set(runID, []);
        jobsByRun.get(runID).push(job);
      });

      const consumed = new Set();
      jobs.forEach(job => {
        const jobID = String(job.id || '');
        if (consumed.has(jobID)) return;

        const runID = String((job.metadata && job.metadata.pipeline_run_id) || '').trim();
        const runJobs = runID ? jobsByRun.get(runID) : null;
        if (!runID || !runJobs || runJobs.length <= 1) {
          consumed.add(jobID);
          tbody.appendChild(buildStaticJobGroupRow(job, opts, columnCount));
          return;
        }

        runJobs.forEach(j => consumed.add(String(j.id || '')));
        const groupTitle = jobGroupLabel(runJobs);
        const status = summarizeJobGroup(runJobs);
        const groupKey = viewKey + ':' + runID;

        const tr = document.createElement('tr');
        tr.className = 'ciwi-job-group-row';
        const td = document.createElement('td');
        td.colSpan = columnCount;
        const details = document.createElement('details');
        details.className = 'ciwi-job-group-details';
        if (expandedJobGroups.has(groupKey)) {
          details.open = true;
        }
        const summary = document.createElement('summary');
        summary.innerHTML = '<span class="ciwi-job-group-main"><span class="ciwi-job-group-emoji" aria-hidden="true">' + status.emoji +
          '</span><span class="ciwi-job-group-title">' + escapeHtml(groupTitle) +
          '</span></span><span class="ciwi-job-group-status ' + status.cls + '">' + escapeHtml(status.text) + '</span><span class="ciwi-job-group-toggle" aria-hidden="true"></span>';
        details.appendChild(summary);

        const innerTable = document.createElement('table');
        innerTable.className = 'ciwi-job-group-table';
        const innerBody = document.createElement('tbody');
        runJobs.forEach(j => {
          innerBody.appendChild(buildJobExecutionRow(j, opts));
        });
        innerTable.appendChild(innerBody);
        details.appendChild(innerTable);
        details.addEventListener('toggle', () => {
          if (details.open) {
            expandedJobGroups.add(groupKey);
          } else {
            expandedJobGroups.delete(groupKey);
          }
          saveStringSet(JOB_GROUPS_STORAGE_KEY, expandedJobGroups);
        });

        td.appendChild(details);
        tr.appendChild(td);
        tbody.appendChild(tr);
      });
    }

    async function refreshJobs() {
      const epoch = ++jobsRenderEpoch;
      const queuedBody = document.getElementById('queuedJobsBody');
      const historyBody = document.getElementById('historyJobsBody');
      const summary = await apiJSON('/api/v1/jobs?view=summary&max=' + String(JOBS_WINDOW));
      if (epoch !== jobsRenderEpoch) return;

      const queuedTotal = Number(summary.queued_count || 0);
      const historyTotal = Number(summary.history_count || 0);
      const queuedGroupTotal = Number(summary.queued_group_count || queuedTotal);
      const historyGroupTotal = Number(summary.history_group_count || historyTotal);
      const queuedGroups = normalizeSummaryGroups(summary.queued_groups, queuedGroupTotal);
      const historyGroups = normalizeSummaryGroups(summary.history_groups, historyGroupTotal);
      if (!tbodyHasConcreteRows(queuedBody)) {
        allocateGroupedSkeletonRows(queuedBody, queuedGroups, 8, 'No queued jobs.', 'queued');
      }
      if (!tbodyHasConcreteRows(historyBody)) {
        allocateGroupedSkeletonRows(historyBody, historyGroups, 7, 'No job history yet.', 'history');
      }

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

      const [queuedJobs, historyJobs] = await Promise.all([
        fetchJobList('queued', queuedTotal, epoch),
        fetchJobList('history', historyTotal, epoch),
      ]);
      if (epoch !== jobsRenderEpoch || queuedJobs === null || historyJobs === null) return;
      const queuedSig = jobsSignature(queuedJobs);
      const historySig = jobsSignature(historyJobs);
      if (queuedSig !== lastQueuedJobsSignature) {
        renderGroupedJobs(queuedBody, queuedJobs, queuedOpts, 'queued', 8, 'No queued jobs.');
        lastQueuedJobsSignature = queuedSig;
      }
      if (historySig !== lastHistoryJobsSignature) {
        renderGroupedJobs(historyBody, historyJobs, historyOpts, 'history', 7, 'No job history yet.');
        lastHistoryJobsSignature = historySig;
      }
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
