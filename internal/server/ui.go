package server

import (
	"net/http"
	"strings"
)

func (s *stateStore) uiHandler(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == "/":
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(indexHTML))
		return
	case strings.HasPrefix(r.URL.Path, "/jobs/"):
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(jobHTML))
		return
	default:
		http.NotFound(w, r)
	}
}

const indexHTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="UTF-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>ciwi</title>
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
    .project { border-top: 1px solid var(--line); padding-top: 10px; margin-top: 10px; }
    .project-head { display:flex; justify-content: space-between; gap:10px; align-items:center; flex-wrap:wrap; }
    .pipeline { display: flex; justify-content: space-between; gap: 8px; padding: 8px 0; }
    .pill { font-size: 12px; padding: 2px 8px; border-radius: 999px; background: #edf8f2; color: #26644b; }
    table { width: 100%; border-collapse: collapse; font-size: 13px; }
    th, td { border-bottom: 1px solid var(--line); text-align: left; padding: 8px 6px; vertical-align: top; }
    td code { white-space: pre-wrap; max-height: 80px; overflow: auto; display: block; }
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
      <h1>ciwi</h1>
      <p>Projects, pipelines and jobs</p>
      <div class="row">
        <input id="repoUrl" placeholder="https://github.com/you/project.git" style="width:380px" />
        <input id="repoRef" placeholder="ref (optional: main, tag, sha)" />
        <input id="configFile" value="ciwi-project.yaml" />
        <button id="importProjectBtn">Add Project</button>
        <span id="importResult"></span>
      </div>
    </div>

    <div class="card">
      <h2>Projects</h2>
      <div id="projects"></div>
    </div>

    <div class="card">
      <h2>Queued Jobs</h2>
      <div class="row" style="margin-bottom:10px;">
        <button id="clearQueueBtn" class="secondary">Clear Queue</button>
      </div>
      <table>
        <thead>
          <tr><th>Description</th><th>Status</th><th>Pipeline</th><th>Agent</th><th>Created</th><th>Output/Error</th><th>Actions</th></tr>
        </thead>
        <tbody id="queuedJobsBody"></tbody>
      </table>
    </div>

    <div class="card">
      <h2>Job History</h2>
      <div class="row" style="margin-bottom:10px;">
        <button id="flushHistoryBtn" class="secondary">Flush History</button>
      </div>
      <table>
        <thead>
          <tr><th>Description</th><th>Status</th><th>Pipeline</th><th>Agent</th><th>Created</th><th>Output/Error</th></tr>
        </thead>
        <tbody id="historyJobsBody"></tbody>
      </table>
    </div>
  </main>

  <script>
    const projectReloadState = new Map();

    async function api(path, opts = {}) {
      const res = await fetch(path, { headers: { 'Content-Type': 'application/json' }, ...opts });
      if (!res.ok) {
        const text = await res.text();
        throw new Error(text || ('HTTP ' + res.status));
      }
      return await res.json();
    }

    function statusClass(status) {
      return 'status-' + (status || '').toLowerCase();
    }

    function jobDescription(job) {
      const m = job.metadata || {};
      const matrix = (m.matrix_name || '').trim();
      const pipelineJob = (m.pipeline_job_id || '').trim();
      const pipeline = (m.pipeline_id || '').trim();
      if (matrix && pipelineJob) return pipelineJob + ' / ' + matrix;
      if (matrix) return matrix;
      if (pipelineJob && pipeline) return pipeline + ' / ' + pipelineJob;
      if (pipelineJob) return pipelineJob;
      if (pipeline) return pipeline;
      return 'Job';
    }

    function setProjectReloadState(projectId, text, color) {
      projectReloadState.set(String(projectId), { text, color });
    }

    async function refreshProjects() {
      const data = await api('/api/v1/projects');
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
        topInfo.innerHTML = '<strong>' + project.name + '</strong> <span class="pill">' + (project.repo_url || '') + '</span> <span class="pill">' + (project.config_file || project.config_path || '') + '</span>';
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
            await api('/api/v1/projects/' + project.id + '/reload', { method: 'POST', body: '{}' });
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
          const info = document.createElement('div');
          info.innerHTML = '<div><code>' + p.pipeline_id + '</code></div><div style="color:#5f6f67;font-size:12px;">' + (p.source_repo || '') + '</div>';
          const btn = document.createElement('button');
          btn.className = 'secondary';
          btn.textContent = 'Run';
          btn.onclick = async () => {
            btn.disabled = true;
            try {
              await api('/api/v1/pipelines/' + p.id + '/run', { method: 'POST', body: '{}' });
              await refreshJobs();
            } catch (e) {
              alert('Run failed: ' + e.message);
            } finally {
              btn.disabled = false;
            }
          };
          row.appendChild(info);
          row.appendChild(btn);
          wrap.appendChild(row);
        });

        root.appendChild(wrap);
      });
    }

    async function refreshJobs() {
      const data = await api('/api/v1/jobs');
      const queuedBody = document.getElementById('queuedJobsBody');
      const historyBody = document.getElementById('historyJobsBody');
      queuedBody.innerHTML = '';
      historyBody.innerHTML = '';

      const queuedStatuses = new Set(['queued', 'leased', 'running']);
      const jobs = (data.jobs || []).slice(0, 150);
      const queuedJobs = jobs.filter(job => queuedStatuses.has((job.status || '').toLowerCase()));
      const historyJobs = jobs.filter(job => !queuedStatuses.has((job.status || '').toLowerCase()));

      queuedJobs.forEach(job => queuedBody.appendChild(renderJobRow(job, true)));
      historyJobs.forEach(job => historyBody.appendChild(renderJobRow(job, false)));
    }

    function renderJobRow(job, includeActions) {
      const tr = document.createElement('tr');
      const pipeline = (job.metadata && job.metadata.pipeline_id) || '';
      const output = (job.error ? ('ERR: ' + job.error + '\n') : '') + (job.output || '');
      const description = jobDescription(job);

      tr.innerHTML =
        '<td><a class="job-link" href="/jobs/' + encodeURIComponent(job.id) + '">' + escapeHtml(description) + '</a></td>' +
        '<td class="' + statusClass(job.status) + '">' + (job.status || '') + '</td>' +
        '<td>' + pipeline + '</td>' +
        '<td>' + (job.leased_by_agent_id || '') + '</td>' +
        '<td>' + (job.created_utc || '') + '</td>' +
        '<td><code>' + escapeHtml(output).slice(-800) + '</code></td>';

      if (includeActions) {
        const actionTd = document.createElement('td');
        if ((job.status || '').toLowerCase() === 'queued') {
          const btn = document.createElement('button');
          btn.className = 'secondary';
          btn.textContent = 'Remove';
          btn.onclick = async () => {
            btn.disabled = true;
            try {
              await api('/api/v1/jobs/' + job.id, { method: 'DELETE' });
              await refreshJobs();
            } catch (e) {
              alert('Remove failed: ' + e.message);
            } finally {
              btn.disabled = false;
            }
          };
          actionTd.appendChild(btn);
        }
        tr.appendChild(actionTd);
      }

      return tr;
    }

    function escapeHtml(s) {
      return (s || '').replace(/[&<>"']/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
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
        await api('/api/v1/projects/import', {
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
      if (!confirm('Clear all queued jobs?')) {
        return;
      }
      try {
        await api('/api/v1/jobs/clear-queue', { method: 'POST', body: '{}' });
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
        await api('/api/v1/jobs/flush-history', { method: 'POST', body: '{}' });
        await refreshJobs();
      } catch (e) {
        alert('Flush history failed: ' + e.message);
      }
    };

    async function tick() {
      try {
        await Promise.all([refreshProjects(), refreshJobs()]);
      } catch (e) {
        console.error(e);
      }
    }

    tick();
    setInterval(refreshJobs, 3000);
    setInterval(refreshProjects, 7000);
  </script>
</body>
</html>`

const jobHTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="UTF-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>ciwi job</title>
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
    .top { display:flex; justify-content:space-between; align-items:center; gap:8px; flex-wrap:wrap; }
    .meta-grid { display:grid; grid-template-columns: 160px 1fr; gap:8px 12px; font-size:14px; }
    .label { color: var(--muted); }
    .status-succeeded { color: var(--ok); font-weight: 700; }
    .status-failed { color: var(--bad); font-weight: 700; }
    .status-running { color: #a56a00; font-weight: 700; }
    .status-queued, .status-leased { color: var(--muted); font-weight: 700; }
    textarea.log {
      margin: 0;
      background: #0f1412;
      color: #cde7dc;
      border-radius: 8px;
      border: 1px solid #22352d;
      padding: 12px;
      width: 100%;
      max-height: 65vh;
      min-height: 320px;
      overflow: auto;
      font-size: 12px;
      line-height: 1.35;
      resize: vertical;
      font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", "Courier New", monospace;
    }
    a { color: var(--accent); text-decoration:none; }
    a:hover { text-decoration:underline; }
  </style>
</head>
<body>
  <main>
    <div class="card top">
      <div>
        <div style="font-size:20px;font-weight:700;" id="jobTitle">Job</div>
        <div style="color:#5f6f67;" id="subtitle">Loading...</div>
      </div>
      <div><a href="/">Back to Queue</a></div>
    </div>

    <div class="card">
      <div class="meta-grid" id="metaGrid"></div>
    </div>

    <div class="card">
      <h3 style="margin:0 0 10px;">Output / Error</h3>
      <textarea id="logBox" class="log" readonly spellcheck="false"></textarea>
    </div>

    <div class="card">
      <h3 style="margin:0 0 10px;">Artifacts</h3>
      <div id="artifactsBox" style="font-size:14px;color:#5f6f67;">Loading...</div>
    </div>
  </main>

  <script>
    function escapeHtml(s) {
      return (s || '').replace(/[&<>"']/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
    }

    function statusClass(status) {
      return 'status-' + (status || '').toLowerCase();
    }

    function jobDescription(job) {
      const m = job.metadata || {};
      const matrix = (m.matrix_name || '').trim();
      const pipelineJob = (m.pipeline_job_id || '').trim();
      const pipeline = (m.pipeline_id || '').trim();
      if (matrix && pipelineJob) return pipelineJob + ' / ' + matrix;
      if (matrix) return matrix;
      if (pipelineJob && pipeline) return pipeline + ' / ' + pipelineJob;
      if (pipelineJob) return pipelineJob;
      if (pipeline) return pipeline;
      return 'Job';
    }

    function jobIdFromPath() {
      const parts = window.location.pathname.split('/').filter(Boolean);
      return parts.length >= 2 ? decodeURIComponent(parts[1]) : '';
    }

    async function loadJob() {
      const jobId = jobIdFromPath();
      if (!jobId) {
        document.getElementById('subtitle').textContent = 'Missing job id';
        return;
      }

      const res = await fetch('/api/v1/jobs/' + encodeURIComponent(jobId));
      if (!res.ok) {
        document.getElementById('subtitle').textContent = 'Failed to load job';
        return;
      }
      const data = await res.json();
      const job = data.job;

      const desc = jobDescription(job);
      document.getElementById('jobTitle').textContent = desc;
      document.getElementById('subtitle').innerHTML = 'Status: <span class="' + statusClass(job.status) + '">' + escapeHtml(job.status || '') + '</span>';

      const pipeline = (job.metadata && job.metadata.pipeline_id) || '';
      const rows = [
        ['Job ID', escapeHtml(job.id || '')],
        ['Status', '<span class="' + statusClass(job.status) + '">' + escapeHtml(job.status || '') + '</span>'],
        ['Pipeline', escapeHtml(pipeline)],
        ['Agent', escapeHtml(job.leased_by_agent_id || '')],
        ['Created', escapeHtml(job.created_utc || '')],
        ['Started', escapeHtml(job.started_utc || '')],
        ['Finished', escapeHtml(job.finished_utc || '')],
        ['Exit Code', (job.exit_code === null || job.exit_code === undefined) ? '' : String(job.exit_code)],
      ];

      const meta = document.getElementById('metaGrid');
      meta.innerHTML = rows.map(r => '<div class="label">' + r[0] + '</div><div>' + r[1] + '</div>').join('');

      const output = (job.error ? ('ERR: ' + job.error + '\n') : '') + (job.output || '');
      document.getElementById('logBox').value = output || '<no output yet>';

      try {
        const ares = await fetch('/api/v1/jobs/' + encodeURIComponent(jobId) + '/artifacts');
        if (!ares.ok) {
          throw new Error('artifact request failed');
        }
        const adata = await ares.json();
        const box = document.getElementById('artifactsBox');
        const items = adata.artifacts || [];
        if (items.length === 0) {
          box.textContent = 'No artifacts';
        } else {
          box.innerHTML = items.map(a =>
            '<div><a href=\"' + a.url + '\" target=\"_blank\" rel=\"noopener\">' + escapeHtml(a.path) + '</a> (' + a.size_bytes + ' bytes)</div>'
          ).join('');
        }
      } catch (_) {
        document.getElementById('artifactsBox').textContent = 'Could not load artifacts';
      }
    }

    loadJob();
    setInterval(loadJob, 2000);
  </script>
</body>
</html>`
