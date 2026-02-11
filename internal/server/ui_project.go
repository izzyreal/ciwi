package server

const projectHTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="UTF-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>ciwi project</title>
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
    main { max-width: 1150px; margin: 24px auto; padding: 0 16px; }
    .card {
      background: var(--card);
      border: 1px solid var(--line);
      border-radius: 12px;
      padding: 16px;
      margin-bottom: 16px;
      box-shadow: 0 8px 24px rgba(21,127,102,.08);
    }
    .top { display:flex; justify-content:space-between; align-items:center; gap:8px; flex-wrap:wrap; }
    .brand { display:flex; align-items:center; gap:12px; }
    .brand img {
      width: 110px;
      height: 91px;
      object-fit: contain;
      display:block;
      image-rendering: crisp-edges;
      image-rendering: pixelated;
    }
    .row { display:flex; gap:8px; align-items:center; flex-wrap:wrap; }
    .muted { color: var(--muted); font-size: 13px; }
    .pill { font-size: 12px; padding: 2px 8px; border-radius: 999px; background: #edf8f2; color: #26644b; }
    button { border: 1px solid var(--accent); border-radius: 8px; padding: 7px 11px; background: var(--accent); color:#fff; cursor:pointer; }
    button.secondary { background: #fff; color: var(--accent); border-color: var(--line); }
    table { width:100%; border-collapse: collapse; font-size: 13px; table-layout: fixed; }
    th, td { border-bottom: 1px solid var(--line); text-align: left; padding: 8px 6px; vertical-align: top; }
    td code { white-space: pre-wrap; max-height: 80px; overflow: auto; display: block; max-width: 100%; overflow-wrap: anywhere; word-break: break-word; }
    .status-succeeded { color: var(--ok); font-weight: 600; }
    .status-failed { color: var(--bad); font-weight: 600; }
    .status-running { color: #a56a00; font-weight: 600; }
    .status-queued, .status-leased { color: var(--muted); }
    a { color: var(--accent); text-decoration:none; }
    a:hover { text-decoration:underline; }
    .pipeline { border-top: 1px solid var(--line); padding-top: 10px; margin-top: 10px; }
    .jobbox { margin: 8px 0 0 8px; padding: 8px; border-left: 2px solid var(--line); }
    .matrix-list { display:flex; flex-wrap:wrap; gap:6px; margin-top: 6px; }
    .matrix-item { border:1px solid var(--line); border-radius:8px; padding:6px; background:#fbfefd; }
  </style>
</head>
<body>
  <main>
    <div class="card top">
      <div class="brand">
        <img src="/ciwi-logo.png" alt="ciwi logo" />
        <div>
          <div id="title" style="font-size:22px;font-weight:700;">Project</div>
          <div id="subtitle" class="muted">Loading...</div>
        </div>
      </div>
      <div><a href="/">Back to Projects</a></div>
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
          <tr><th>Job Execution</th><th>Status</th><th>Pipeline</th><th>Build</th><th>Agent</th><th>Created</th></tr>
        </thead>
        <tbody id="historyBody"></tbody>
      </table>
    </div>
  </main>

  <script src="/ui/shared.js"></script>
  <script src="/ui/pages.js"></script>
  <script>
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

      const structure = document.getElementById('structure');
      if (!p.pipelines || p.pipelines.length === 0) {
        structure.innerHTML = '<div class="muted">No pipelines</div>';
        return;
      }

      structure.innerHTML = '';
      p.pipelines.forEach(pl => {
        const container = document.createElement('div');
        container.className = 'pipeline';
        const head = document.createElement('div');
        head.className = 'row';
        const deps = (pl.depends_on || []).join(', ');
        head.innerHTML = '<strong>Pipeline: <code>' + escapeHtml(pl.pipeline_id) + '</code></strong>' +
          (deps ? ('<span class="muted">depends_on: ' + escapeHtml(deps) + '</span>') : '');
        const runAll = document.createElement('button');
        runAll.textContent = 'Run Pipeline';
        runAll.className = 'secondary';
        runAll.onclick = async () => {
          runAll.disabled = true;
          try {
            await apiJSON('/api/v1/pipelines/' + pl.id + '/run', { method: 'POST', body: '{}' });
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
            await apiJSON('/api/v1/pipelines/' + pl.id + '/run', { method: 'POST', body: JSON.stringify({ dry_run: true }) });
            await loadHistory();
          } catch (e) {
            alert('Dry run failed: ' + e.message);
          } finally {
            dryAll.disabled = false;
          }
        };
        head.appendChild(runAll);
        head.appendChild(dryAll);
        container.appendChild(head);

        (pl.jobs || []).forEach(j => {
          const jb = document.createElement('div');
          jb.className = 'jobbox';
          const runsOn = Object.entries(j.runs_on || {}).map(kv => kv[0] + '=' + kv[1]).join(', ');
          jb.innerHTML =
            '<div><strong>Job: ' + escapeHtml(j.id || '') + '</strong> <span class="muted">timeout=' + (j.timeout_seconds || 0) + 's</span></div>' +
            '<div class="muted">runs_on: ' + escapeHtml(runsOn) + '</div>';

          const matrixList = document.createElement('div');
          matrixList.className = 'matrix-list';
          const includes = (j.matrix_includes && j.matrix_includes.length > 0) ? j.matrix_includes : [{ index: 0, name: '', vars: {} }];
          includes.forEach(mi => {
            const name = (mi.name || '').trim() || ('index-' + mi.index);
            const item = document.createElement('div');
            item.className = 'matrix-item';
            const vars = Object.entries(mi.vars || {}).map(kv => kv[0] + '=' + kv[1]).join(', ');
            item.innerHTML = '<div><code>' + escapeHtml(name) + '</code></div><div class="muted">' + escapeHtml(vars) + '</div>';
            const btn = document.createElement('button');
            btn.textContent = 'Run';
            btn.className = 'secondary';
            btn.style.marginTop = '6px';
            btn.onclick = async () => {
              btn.disabled = true;
              try {
                await apiJSON('/api/v1/pipelines/' + pl.id + '/run-selection', {
                  method: 'POST',
                  body: JSON.stringify({ pipeline_job_id: j.id, matrix_index: mi.index })
                });
                await loadHistory();
              } catch (e) {
                alert('Run selection failed: ' + e.message);
              } finally {
                btn.disabled = false;
              }
            };
            const dryBtn = document.createElement('button');
            dryBtn.textContent = 'Dry Run';
            dryBtn.className = 'secondary';
            dryBtn.style.marginTop = '6px';
            dryBtn.onclick = async () => {
              dryBtn.disabled = true;
              try {
                await apiJSON('/api/v1/pipelines/' + pl.id + '/run-selection', {
                  method: 'POST',
                  body: JSON.stringify({ pipeline_job_id: j.id, matrix_index: mi.index, dry_run: true })
                });
                await loadHistory();
              } catch (e) {
                alert('Dry run selection failed: ' + e.message);
              } finally {
                dryBtn.disabled = false;
              }
            };
            item.appendChild(btn);
            item.appendChild(dryBtn);
            matrixList.appendChild(item);
          });
          jb.appendChild(matrixList);
          container.appendChild(jb);
        });

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

    async function loadHistory() {
      const data = await apiJSON('/api/v1/jobs');
      const body = document.getElementById('historyBody');
      body.innerHTML = '';
      const rows = (data.job_executions || data.jobs || []).filter(j => ((j.metadata && j.metadata.project) || '') === currentProjectName).slice(0, 120);
      rows.forEach(job => {
        const tr = buildJobExecutionRow(job, {
          includeActions: false,
          backPath: window.location.pathname || '/'
        });
        body.appendChild(tr);
      });
    }

    async function tick() {
      try {
        await loadProject();
        await loadVaultSection();
        await loadHistory();
      } catch (e) {
        document.getElementById('subtitle').textContent = 'Failed to load project: ' + e.message;
      }
    }

    tick();
    setInterval(loadHistory, 4000);
  </script>
</body>
</html>`
