package server

import (
	"net/http"
)

func (s *stateStore) uiHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(indexHTML))
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
    .pipeline { display: flex; justify-content: space-between; gap: 8px; padding: 8px 0; }
    .pill { font-size: 12px; padding: 2px 8px; border-radius: 999px; background: #edf8f2; color: #26644b; }
    table { width: 100%; border-collapse: collapse; font-size: 13px; }
    th, td { border-bottom: 1px solid var(--line); text-align: left; padding: 8px 6px; vertical-align: top; }
    td code { white-space: pre-wrap; max-height: 80px; overflow: auto; display: block; }
    .status-succeeded { color: var(--ok); font-weight: 600; }
    .status-failed { color: var(--bad); font-weight: 600; }
    .status-running { color: #a56a00; font-weight: 600; }
    .status-queued, .status-leased { color: var(--muted); }
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
      <h2>Jobs</h2>
      <table>
        <thead>
          <tr><th>ID</th><th>Status</th><th>Pipeline</th><th>Agent</th><th>Created</th><th>Output/Error</th></tr>
        </thead>
        <tbody id="jobsBody"></tbody>
      </table>
    </div>
  </main>

  <script>
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
        top.innerHTML = '<strong>' + project.name + '</strong> <span class="pill">' + (project.repo_url || '') + '</span> <span class="pill">' + (project.config_file || project.config_path || '') + '</span>';
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
      const body = document.getElementById('jobsBody');
      body.innerHTML = '';
      (data.jobs || []).slice(0, 80).forEach(job => {
        const tr = document.createElement('tr');
        const pipeline = (job.metadata && job.metadata.pipeline_id) || '';
        const output = (job.error ? ('ERR: ' + job.error + '\n') : '') + (job.output || '');
        tr.innerHTML =
          '<td><code>' + job.id + '</code></td>' +
          '<td class="' + statusClass(job.status) + '">' + job.status + '</td>' +
          '<td>' + pipeline + '</td>' +
          '<td>' + (job.leased_by_agent_id || '') + '</td>' +
          '<td>' + (job.created_utc || '') + '</td>' +
          '<td><code>' + escapeHtml(output).slice(-800) + '</code></td>';
        body.appendChild(tr);
      });
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
