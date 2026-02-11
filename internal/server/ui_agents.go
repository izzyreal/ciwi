package server

const agentsHTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="UTF-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>ciwi agents</title>
  <link rel="icon" type="image/png" href="/ciwi-favicon.png" />
  <style>
    :root { --bg:#f2f7f4; --bg2:#d9efe2; --card:#fff; --ink:#1f2a24; --muted:#5f6f67; --ok:#1f8a4c; --bad:#b23a48; --accent:#157f66; --line:#c4ddd0; }
    * { box-sizing:border-box; }
    body { margin:0; font-family:"Avenir Next","Segoe UI",sans-serif; color:var(--ink); background:radial-gradient(circle at 20% 0%, var(--bg2), var(--bg)); }
    main { max-width:1100px; margin:24px auto; padding:0 16px; }
    .card { background:var(--card); border:1px solid var(--line); border-radius:12px; padding:16px; margin-bottom:16px; box-shadow:0 8px 24px rgba(21,127,102,.08); }
    .row { display:flex; align-items:center; justify-content:space-between; gap:8px; flex-wrap:wrap; }
    .brand { display:flex; align-items:center; gap:12px; }
    .brand img { width:110px; height:91px; object-fit:contain; display:block; image-rendering:crisp-edges; image-rendering:pixelated; }
    .muted { color:var(--muted); font-size:13px; }
    table { width:100%; border-collapse:collapse; font-size:13px; table-layout:fixed; }
    th, td { border-bottom:1px solid var(--line); text-align:left; padding:8px 6px; vertical-align:top; overflow-wrap:anywhere; word-break:break-word; }
    .logbox {
      margin: 0;
      white-space: pre-wrap;
      font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", "Courier New", monospace;
      font-size: 11px;
      line-height: 1.35;
      max-height: 120px;
      overflow: auto;
      user-select: text;
      cursor: text;
      background: #f7fcf9;
      border: 1px solid var(--line);
      border-radius: 6px;
      padding: 6px;
    }
    .ok { color:var(--ok); font-weight:600; }
    .stale { color:#a56a00; font-weight:600; }
    .offline { color:var(--bad); font-weight:600; }
    .badge {
      display: inline-block;
      font-size: 11px;
      padding: 2px 7px;
      border-radius: 999px;
      border: 1px solid var(--line);
      background: #edf8f2;
      color: #26644b;
      margin-top: 4px;
    }
    .badge-warn { background:#fff6e6; color:#8a5a00; }
    button { border:1px solid var(--line); border-radius:8px; padding:8px 10px; font-size:14px; cursor:pointer; background:#fff; color:var(--accent); }
    a { color: var(--accent); text-decoration:none; } a:hover { text-decoration:underline; }
  </style>
</head>
<body>
  <main>
    <div class="card row">
      <div class="brand">
        <img src="/ciwi-logo.png" alt="ciwi logo" />
        <div>
          <div style="font-size:24px;font-weight:700;">Agents</div>
          <div class="muted">Available execution agents and heartbeat status</div>
        </div>
      </div>
      <a href="/">Back to Projects</a>
    </div>
    <div class="card">
      <div class="row" style="margin-bottom:10px;">
        <div class="muted" id="summary">Loading...</div>
        <button id="refreshBtn">Refresh</button>
      </div>
      <table>
        <thead>
          <tr><th>Agent ID</th><th>Host</th><th>Platform</th><th>Version</th><th>Last Seen</th><th>Health</th><th>Capabilities</th><th>Actions</th><th>Recent Log</th></tr>
        </thead>
        <tbody id="rows"></tbody>
      </table>
    </div>
  </main>
  <script src="/ui/shared.js"></script>
  <script>
    function statusForLastSeen(ts) {
      if (!ts) return { label: 'unknown', cls: 'offline' };
      const d = new Date(ts);
      if (isNaN(d.getTime())) return { label: 'unknown', cls: 'offline' };
      const ageMs = Date.now() - d.getTime();
      if (ageMs <= 20000) return { label: 'online', cls: 'ok' };
      if (ageMs <= 60000) return { label: 'stale', cls: 'stale' };
      return { label: 'offline', cls: 'offline' };
    }
    function formatCapabilities(caps) {
      if (!caps) return '';
      const entries = Object.entries(caps);
      if (entries.length === 0) return '';
      return entries.map(([k,v]) => k + '=' + v).join(', ');
    }
    async function refreshAgents() {
      const rows = document.getElementById('rows');
      const summary = document.getElementById('summary');
      try {
        const res = await fetch('/api/v1/agents');
        if (!res.ok) throw new Error('HTTP ' + res.status);
        const data = await res.json();
        const agents = data.agents || [];
        rows.innerHTML = '';
        if (agents.length === 0) {
          rows.innerHTML = '<tr><td colspan="9" class="muted">No agents have sent heartbeats yet.</td></tr>';
          summary.textContent = '0 agents';
          return;
        }
        agents.sort((a, b) => String(a.agent_id || '').localeCompare(String(b.agent_id || '')));
        for (const a of agents) {
          const s = statusForLastSeen(a.last_seen_utc || '');
          const tr = document.createElement('tr');
          const updateBtn = (a.update_requested)
            ? '<button data-action="update" data-agent-id="' + escapeHtml(a.agent_id || '') + '">Retry Now</button>'
            : ((a.needs_update && s.label !== 'offline')
              ? '<button data-action="update" data-agent-id="' + escapeHtml(a.agent_id || '') + '">Update</button>'
              : '');
          const refreshBtn = (s.label !== 'offline')
            ? '<button data-action="refresh-tools" data-agent-id="' + escapeHtml(a.agent_id || '') + '">Refresh Tools</button>'
            : '';
          const retryText = (a.update_requested && a.update_next_retry_utc)
            ? ('<div class="badge badge-warn">Backoff until ' + escapeHtml(formatTimestamp(a.update_next_retry_utc)) + ' (attempt ' + String(a.update_attempts || 0) + ')</div>')
            : '';
          const versionCell = escapeHtml(a.version || '') +
            (a.update_requested ? ('<div class="badge">Update requested → ' + escapeHtml(a.update_target || '') + '</div>') : '') +
            retryText;
          tr.innerHTML =
            '<td>' + escapeHtml(a.agent_id || '') + '</td>' +
            '<td>' + escapeHtml(a.hostname || '') + '</td>' +
            '<td>' + escapeHtml((a.os || '') + '/' + (a.arch || '')) + '</td>' +
            '<td>' + versionCell + '</td>' +
            '<td>' + escapeHtml(formatTimestamp(a.last_seen_utc)) + '</td>' +
            '<td class="' + s.cls + '">' + s.label + '</td>' +
            '<td>' + escapeHtml(formatCapabilities(a.capabilities || {})) + '</td>' +
            '<td>' + updateBtn + ' ' + refreshBtn + '</td>' +
            '<td><div class="logbox">' + escapeHtml((a.recent_log || []).join('\n')) + '</div></td>';
          rows.appendChild(tr);
        }
        rows.querySelectorAll('button[data-action="update"], button[data-action="refresh-tools"]').forEach(btn => {
          btn.addEventListener('click', async () => {
            const id = btn.getAttribute('data-agent-id') || '';
            if (!id) return;
            const action = btn.getAttribute('data-action') || '';
            btn.disabled = true;
            try {
              const suffix = action === 'refresh-tools' ? 'refresh-tools' : 'update';
              const res = await fetch('/api/v1/agents/' + encodeURIComponent(id) + '/' + suffix, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: '{}' });
              if (!res.ok) throw new Error(await res.text());
              await refreshAgents();
            } catch (e) {
              alert('Request failed: ' + e.message);
            } finally {
              btn.disabled = false;
            }
          });
        });
        const online = agents.filter(a => statusForLastSeen(a.last_seen_utc || '').label === 'online').length;
        summary.textContent = online + '/' + agents.length + ' online';
      } catch (e) {
        rows.innerHTML = '<tr><td colspan="9" class="offline">Could not load agents</td></tr>';
        summary.textContent = 'Failed to load agents';
      }
    }
    document.getElementById('refreshBtn').onclick = refreshAgents;
    refreshAgents();
    setInterval(refreshAgents, 3000);
  </script>
</body>
</html>`
