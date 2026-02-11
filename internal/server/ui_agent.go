package server

const agentHTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="UTF-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>ciwi agent</title>
  <link rel="icon" type="image/png" href="/ciwi-favicon.png" />
  <style>
    :root { --bg:#f2f7f4; --bg2:#d9efe2; --card:#fff; --ink:#1f2a24; --muted:#5f6f67; --ok:#1f8a4c; --bad:#b23a48; --accent:#157f66; --line:#c4ddd0; }
    * { box-sizing:border-box; }
    :where(body, main, .card, p, div, span, table, thead, tbody, tr, th, td, code, pre, input, textarea, a) {
      -webkit-user-select: text;
      user-select: text;
    }
    :where(button) {
      -webkit-user-select: none;
      user-select: none;
    }
    body { margin:0; font-family:"Avenir Next","Segoe UI",sans-serif; color:var(--ink); background:radial-gradient(circle at 20% 0%, var(--bg2), var(--bg)); }
    main { max-width:1100px; margin:24px auto; padding:0 16px; }
    .card { background:var(--card); border:1px solid var(--line); border-radius:12px; padding:16px; margin-bottom:16px; box-shadow:0 8px 24px rgba(21,127,102,.08); }
    .row { display:flex; align-items:center; justify-content:space-between; gap:8px; flex-wrap:wrap; }
    .brand { display:flex; align-items:center; gap:12px; }
    .brand img { width:110px; height:91px; object-fit:contain; display:block; image-rendering:crisp-edges; image-rendering:pixelated; }
    .muted { color:var(--muted); font-size:13px; }
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
    }
    .badge-warn { background:#fff6e6; color:#8a5a00; }
    .grid { display:grid; grid-template-columns:180px minmax(0,1fr); gap:6px 10px; font-size:13px; }
    .label { color:var(--muted); font-weight:600; }
    .value { overflow-wrap:anywhere; word-break:break-word; }
    .logbox {
      margin:0;
      white-space:pre-wrap;
      font-family:ui-monospace,SFMono-Regular,Menlo,Monaco,Consolas,"Liberation Mono","Courier New",monospace;
      font-size:12px;
      line-height:1.4;
      min-height:220px;
      max-height:520px;
      overflow:auto;
      user-select:text;
      cursor:text;
      background:#f7fcf9;
      border:1px solid var(--line);
      border-radius:8px;
      padding:10px;
    }
    button { border:1px solid var(--line); border-radius:8px; padding:8px 10px; font-size:14px; cursor:pointer; background:#fff; color:var(--accent); }
    a { color:var(--accent); text-decoration:none; } a:hover { text-decoration:underline; }
  </style>
</head>
<body>
  <main>
    <div class="card row">
      <div class="brand">
        <img src="/ciwi-logo.png" alt="ciwi logo" />
        <div>
          <div style="font-size:24px;font-weight:700;">Agent Detail</div>
          <div class="muted" id="subtitle">Loading...</div>
        </div>
      </div>
      <div class="row" style="gap:8px;">
        <a href="/agents">Back to Agents</a>
        <button id="refreshBtn">Refresh</button>
      </div>
    </div>

    <div class="card">
      <div class="row" style="margin-bottom:10px;">
        <div id="statusText" class="muted"></div>
        <div>
          <button id="updateBtn" style="display:none;">Update</button>
          <button id="refreshToolsBtn" style="display:none;">Refresh Tools</button>
        </div>
      </div>
      <div id="meta" class="grid"></div>
    </div>

    <div class="card">
      <div style="font-weight:600; margin-bottom:8px;">Recent Log</div>
      <pre id="logBox" class="logbox"></pre>
    </div>
  </main>
  <script src="/ui/shared.js"></script>
  <script>
    const agentID = decodeURIComponent(location.pathname.replace(/^\/agents\//, '').replace(/\/+$/, ''));
    let refreshInFlight = false;
    let refreshPausedUntil = 0;

    function hasActiveTextSelection() {
      const sel = window.getSelection && window.getSelection();
      if (!sel) return false;
      const text = (sel.toString() || '').trim();
      return text.length > 0;
    }

    function shouldPauseRefresh() {
      if (hasActiveTextSelection()) {
        refreshPausedUntil = Date.now() + 5000;
        return true;
      }
      return Date.now() < refreshPausedUntil;
    }

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

    function metaRow(k, v) {
      return '<div class="label">' + escapeHtml(k) + '</div><div class="value">' + v + '</div>';
    }

    async function postAction(suffix) {
      const res = await fetch('/api/v1/agents/' + encodeURIComponent(agentID) + '/' + suffix, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: '{}',
      });
      if (!res.ok) throw new Error(await res.text());
    }

    async function refreshAgent(force) {
      if (refreshInFlight || (!force && shouldPauseRefresh())) {
        return;
      }
      refreshInFlight = true;
      try {
        const res = await fetch('/api/v1/agents/' + encodeURIComponent(agentID));
        if (!res.ok) {
          if (res.status === 404) throw new Error('Agent not found');
          throw new Error('HTTP ' + res.status);
        }
        const data = await res.json();
        const a = data.agent || {};
        const s = statusForLastSeen(a.last_seen_utc || '');
        document.getElementById('subtitle').textContent = a.agent_id || agentID;
        document.getElementById('statusText').innerHTML = 'Health: <span class="' + s.cls + '">' + s.label + '</span>';

        const updateButton = document.getElementById('updateBtn');
        const refreshToolsButton = document.getElementById('refreshToolsBtn');
        const showUpdate = !!a.update_requested || (!!a.needs_update && s.label !== 'offline');
        updateButton.style.display = showUpdate ? 'inline-block' : 'none';
        updateButton.textContent = a.update_requested ? 'Retry Now' : 'Update';
        refreshToolsButton.style.display = s.label !== 'offline' ? 'inline-block' : 'none';

        let updateState = '';
        if (a.update_requested) {
          updateState = '<span class="badge">Update requested → ' + escapeHtml(a.update_target || '') + '</span>';
          if (a.update_next_retry_utc) {
            updateState += ' <span class="badge badge-warn">Backoff until ' + escapeHtml(formatTimestamp(a.update_next_retry_utc)) + ' (attempt ' + String(a.update_attempts || 0) + ')</span>';
          }
        }

        const metaHTML =
          metaRow('Agent ID', escapeHtml(a.agent_id || agentID)) +
          metaRow('Hostname', escapeHtml(a.hostname || '')) +
          metaRow('Platform', escapeHtml((a.os || '') + '/' + (a.arch || ''))) +
          metaRow('Version', escapeHtml(a.version || '')) +
          metaRow('Last Seen', escapeHtml(formatTimestamp(a.last_seen_utc || ''))) +
          metaRow('Capabilities', escapeHtml(formatCapabilities(a.capabilities || {}))) +
          metaRow('Update', updateState || '<span class="muted">none</span>');
        document.getElementById('meta').innerHTML = metaHTML;
        document.getElementById('logBox').textContent = (a.recent_log || []).join('\n');
      } catch (e) {
        document.getElementById('subtitle').textContent = String(e.message || e);
        document.getElementById('statusText').textContent = 'Failed to load agent';
      } finally {
        refreshInFlight = false;
      }
    }

    document.getElementById('refreshBtn').onclick = () => refreshAgent(true);
    document.getElementById('updateBtn').onclick = async () => {
      try {
        await postAction('update');
        await refreshAgent(true);
      } catch (e) {
        alert('Update request failed: ' + e.message);
      }
    };
    document.getElementById('refreshToolsBtn').onclick = async () => {
      try {
        await postAction('refresh-tools');
        await refreshAgent(true);
      } catch (e) {
        alert('Refresh tools request failed: ' + e.message);
      }
    };
    document.addEventListener('selectionchange', () => {
      if (hasActiveTextSelection()) {
        refreshPausedUntil = Date.now() + 5000;
      }
    });
    refreshAgent(true);
    setInterval(() => refreshAgent(false), 3000);
  </script>
</body>
</html>`
