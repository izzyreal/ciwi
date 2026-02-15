package server

const agentsHTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="UTF-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>ciwi agents</title>
  <link rel="icon" type="image/png" href="/ciwi-favicon.png" />
  <style>
` + uiPageChromeCSS + `
    .row { display:flex; align-items:center; justify-content:space-between; gap:8px; flex-wrap:wrap; }
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
    .badge-error { background:#ffeded; color:#8f1f1f; }
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
      <a class="nav-btn" href="/">Back to Projects <span class="nav-emoji" aria-hidden="true">↩</span></a>
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
    let refreshInFlight = false;
    const refreshGuard = createRefreshGuard(5000);

    function formatUpdatePrimaryText(a) {
      if (!a || !a.update_requested) return '';
      const target = escapeHtml(a.update_target || '');
      if (a.job_in_progress) {
        return '<div class="badge badge-warn">Pending update → ' + target + ' (agent busy)</div>';
      }
      if (a.update_in_progress) {
        return '<div class="badge">Update → ' + target + ' in progress</div>';
      }
      return '<div class="badge">Update requested → ' + target + '</div>';
    }

    function formatUpdateRetryText(a) {
      if (!a || !a.update_requested || a.job_in_progress || a.update_in_progress || !a.update_next_retry_utc) return '';
      const attempt = Number(a.update_attempts || 0);
      if (attempt <= 0) return '';
      const reason = String(a.update_last_error || '').trim();
      const reasonSuffix = reason ? ': ' + escapeHtml(reason) : '';
      return '<div class="badge badge-error">Backoff until ' + escapeHtml(formatTimestamp(a.update_next_retry_utc)) + ' (attempt ' + String(attempt) + ')' + reasonSuffix + '</div>';
    }

    async function refreshAgents() {
      if (refreshInFlight || refreshGuard.shouldPause()) {
        return;
      }
      refreshInFlight = true;
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
          const updateBtn = (a.update_requested && !a.update_in_progress)
            ? '<button data-action="update" data-agent-id="' + escapeHtml(a.agent_id || '') + '">Retry Now</button>'
            : ((a.needs_update && s.label !== 'offline')
              ? '<button data-action="update" data-agent-id="' + escapeHtml(a.agent_id || '') + '">Update</button>'
              : '');
          const refreshBtn = (s.label !== 'offline')
            ? '<button data-action="refresh-tools" data-agent-id="' + escapeHtml(a.agent_id || '') + '">Refresh Tools</button>'
            : '';
          const primaryUpdateText = formatUpdatePrimaryText(a);
          const retryText = formatUpdateRetryText(a);
          const versionCell = escapeHtml(a.version || '') +
            primaryUpdateText +
            retryText;
          tr.innerHTML =
            '<td><a href="/agents/' + encodeURIComponent(a.agent_id || '') + '">' + escapeHtml(a.agent_id || '') + '</a></td>' +
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
              const reqAction = action === 'refresh-tools' ? 'refresh-tools' : 'update';
              const res = await fetch('/api/v1/agents/' + encodeURIComponent(id) + '/actions', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ action: reqAction }) });
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
      } finally {
        refreshInFlight = false;
      }
    }
    document.getElementById('refreshBtn').onclick = refreshAgents;
    refreshGuard.bindSelectionListener();
    refreshAgents();
    setInterval(refreshAgents, 3000);
  </script>
</body>
</html>`
