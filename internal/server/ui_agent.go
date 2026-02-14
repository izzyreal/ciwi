package server

const agentHTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="UTF-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>ciwi agent</title>
  <link rel="icon" type="image/png" href="/ciwi-favicon.png" />
  <style>
` + uiPageChromeCSS + `
    .row { display:flex; align-items:center; justify-content:space-between; gap:8px; flex-wrap:wrap; }
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
    .badge-error { background:#ffeded; color:#8f1f1f; }
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
    .adhoc-modal-body {
      display: grid;
      grid-template-columns: 1fr 1fr;
      gap: 10px;
      min-height: 0;
    }
    .adhoc-modal-pane {
      border: 1px solid var(--line);
      border-radius: 10px;
      min-width: 0;
      min-height: 0;
      display: grid;
      grid-template-rows: auto 1fr;
      background: #fff;
    }
    .adhoc-modal-pane-head {
      border-bottom: 1px solid var(--line);
      padding: 8px 10px;
      color: var(--muted);
      font-size: 12px;
      font-weight: 600;
      letter-spacing: 0.02em;
    }
    #adhocScriptInput {
      width: 100%;
      height: 100%;
      border: 0;
      outline: none;
      resize: none;
      padding: 10px;
      font-family: ui-monospace,SFMono-Regular,Menlo,Monaco,Consolas,"Liberation Mono","Courier New",monospace;
      font-size: 12px;
      line-height: 1.45;
    }
    #adhocOutput {
      margin: 0;
      padding: 10px;
      overflow: auto;
      white-space: pre-wrap;
      font-family: ui-monospace,SFMono-Regular,Menlo,Monaco,Consolas,"Liberation Mono","Courier New",monospace;
      font-size: 12px;
      line-height: 1.45;
      background: #f7fcf9;
    }
    @media (max-width: 900px) {
      .adhoc-modal-body { grid-template-columns: 1fr; }
    }
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
        <a class="nav-btn" href="/agents">Back to Agents <span class="nav-emoji" aria-hidden="true">↩</span></a>
        <button id="refreshBtn">Refresh</button>
      </div>
    </div>

    <div class="card">
      <div class="row" style="margin-bottom:10px;">
        <div id="statusText" class="muted"></div>
        <div>
          <button id="updateBtn" style="display:none;">Update</button>
          <button id="refreshToolsBtn" style="display:none;">Refresh Tools</button>
          <button id="runAdhocBtn" style="display:none;">Run Adhoc Script</button>
        </div>
      </div>
      <div id="meta" class="grid"></div>
    </div>

    <div class="card">
      <div style="font-weight:600; margin-bottom:8px;">Recent Log</div>
      <pre id="logBox" class="logbox"></pre>
    </div>
  </main>

  <div id="adhocModalOverlay" class="ciwi-modal-overlay" aria-hidden="true">
    <div class="ciwi-modal" role="dialog" aria-modal="true" aria-label="Run ad-hoc script">
      <div class="ciwi-modal-head">
        <div style="font-weight:700;">Run Adhoc Script</div>
        <div class="row" style="gap:8px;">
          <label for="adhocShellSelect" class="muted" style="font-weight:600;">Shell</label>
          <select id="adhocShellSelect"></select>
          <button id="adhocRunBtn">Run</button>
          <button id="adhocCloseBtn">Close</button>
        </div>
      </div>
      <div class="ciwi-modal-body adhoc-modal-body">
        <div class="adhoc-modal-pane">
          <div class="adhoc-modal-pane-head">Script</div>
          <textarea id="adhocScriptInput" spellcheck="false"></textarea>
        </div>
        <div class="adhoc-modal-pane">
          <div class="adhoc-modal-pane-head">Output</div>
          <pre id="adhocOutput"></pre>
        </div>
      </div>
    </div>
  </div>
  <script src="/ui/shared.js"></script>
  <script>
    const agentID = decodeURIComponent(location.pathname.replace(/^\/agents\//, '').replace(/\/+$/, ''));
    const adhocModalOverlay = document.getElementById('adhocModalOverlay');
    const adhocShellSelect = document.getElementById('adhocShellSelect');
    const adhocScriptInput = document.getElementById('adhocScriptInput');
    const adhocOutput = document.getElementById('adhocOutput');
    const adhocRunBtn = document.getElementById('adhocRunBtn');
    let refreshInFlight = false;
    const refreshGuard = createRefreshGuard(5000);
    let adhocShells = [];
    let adhocActiveJobID = '';
    let adhocPollTimer = null;

    function formatUpdatePrimaryText(a) {
      if (!a || !a.update_requested) return '';
      const target = escapeHtml(a.update_target || '');
      if (a.job_in_progress) {
        return '<span class="badge badge-warn">Pending update → ' + target + ' (agent busy)</span>';
      }
      if (a.update_in_progress) {
        return '<span class="badge">Update → ' + target + ' in progress</span>';
      }
      return '<span class="badge">Update requested → ' + target + '</span>';
    }

    function formatUpdateRetryText(a) {
      if (!a || !a.update_requested || a.job_in_progress || a.update_in_progress || !a.update_next_retry_utc) return '';
      const attempt = Number(a.update_attempts || 0);
      if (attempt <= 0) return '';
      const reason = String(a.update_last_error || '').trim();
      const reasonSuffix = reason ? ': ' + escapeHtml(reason) : '';
      return '<span class="badge badge-error">Backoff until ' + escapeHtml(formatTimestamp(a.update_next_retry_utc)) + ' (attempt ' + String(attempt) + ')' + reasonSuffix + '</span>';
    }
    let lastSuggestedScript = '';

    function metaRow(k, v) {
      return '<div class="label">' + escapeHtml(k) + '</div><div class="value">' + v + '</div>';
    }

    function parseAgentShells(caps) {
      const executor = String((caps && caps.executor) || '').trim().toLowerCase();
      if (executor !== 'script') return [];
      const raw = String((caps && caps.shells) || '').trim();
      if (!raw) return [];
      const unique = {};
      const out = [];
      raw.split(',').forEach(part => {
        const v = String(part || '').trim().toLowerCase();
        if (!v || unique[v]) return;
        unique[v] = true;
        out.push(v);
      });
      return out;
    }

    function exampleScriptForShell(shell) {
      if (shell === 'cmd') {
        return [
          'ver',
          'echo Hello from ciwi ad-hoc cmd',
          'echo Date: %DATE%',
          'echo Time: %TIME%',
        ].join('\n');
      }
      if (shell === 'powershell') {
        return [
          '$ErrorActionPreference = "Stop"',
          'Write-Host "Hello from ciwi ad-hoc (PowerShell)"',
          'Write-Host ("PSVersion: " + $PSVersionTable.PSVersion.ToString())',
          'Write-Host ("Date: " + (Get-Date -Format "yyyy-MM-dd HH:mm:ss"))',
        ].join('\n');
      }
      return [
        'set -eu',
        'echo "Hello from ciwi ad-hoc (POSIX)"',
        'uname -a',
        'date',
      ].join('\n');
    }

    function clearAdhocPoll() {
      if (adhocPollTimer) {
        clearTimeout(adhocPollTimer);
        adhocPollTimer = null;
      }
    }

    function openAdhocModal() {
      if (adhocShells.length === 0) return;
      adhocShellSelect.innerHTML = '';
      adhocShells.forEach(shell => {
        const opt = document.createElement('option');
        opt.value = shell;
        opt.textContent = shell;
        adhocShellSelect.appendChild(opt);
      });
      const suggested = exampleScriptForShell(adhocShellSelect.value || adhocShells[0]);
      adhocScriptInput.value = suggested;
      lastSuggestedScript = suggested;
      if (!adhocActiveJobID) {
        adhocOutput.textContent = 'Pick a shell, tweak the example script, then click Run.';
      }
      adhocRunBtn.disabled = false;
      adhocRunBtn.textContent = 'Run';
      openModalOverlay(adhocModalOverlay, '90vw', '90vh');
      setTimeout(() => adhocScriptInput.focus(), 0);
    }

    function closeAdhocModal() {
      closeModalOverlay(adhocModalOverlay);
      clearAdhocPoll();
      adhocActiveJobID = '';
      adhocRunBtn.disabled = false;
      adhocRunBtn.textContent = 'Run';
    }

    function renderJobOutput(job) {
      const lines = [];
      lines.push('[job] ' + String(job.id || ''));
      lines.push('[status] ' + String(job.status || ''));
      if (job.created_utc) lines.push('[created] ' + formatTimestamp(job.created_utc));
      if (job.started_utc) lines.push('[started] ' + formatTimestamp(job.started_utc));
      if (job.finished_utc) lines.push('[finished] ' + formatTimestamp(job.finished_utc));
      if (job.exit_code !== undefined && job.exit_code !== null) lines.push('[exit_code] ' + String(job.exit_code));
      let body = lines.join('\n');
      if (job.output) body += '\n\n' + String(job.output);
      if (job.error) body += '\n\n[error]\n' + String(job.error);
      adhocOutput.textContent = body;
      adhocOutput.scrollTop = adhocOutput.scrollHeight;
    }

    async function pollAdhocJob(jobID) {
      if (!jobID || jobID !== adhocActiveJobID) return;
      try {
        const res = await fetch('/api/v1/jobs/' + encodeURIComponent(jobID));
        if (!res.ok) throw new Error('HTTP ' + res.status);
        const data = await res.json();
        const job = data.job_execution || {};
        renderJobOutput(job);
        const terminal = isTerminalJobStatus(job.status);
        if (terminal) {
          adhocRunBtn.disabled = false;
          adhocRunBtn.textContent = 'Run';
          adhocActiveJobID = '';
          clearAdhocPoll();
          return;
        }
      } catch (e) {
        adhocOutput.textContent += '\n\n[poll error] ' + String(e.message || e);
      }
      adhocPollTimer = setTimeout(() => pollAdhocJob(jobID), 900);
    }

    async function runAdhocScript() {
      const shell = String(adhocShellSelect.value || '').trim();
      const script = String(adhocScriptInput.value || '');
      if (!shell) {
        alert('Pick a shell first.');
        return;
      }
      if (!script.trim()) {
        alert('Script is empty.');
        return;
      }
      adhocRunBtn.disabled = true;
      adhocRunBtn.textContent = 'Running...';
      adhocOutput.textContent = 'Queueing ad-hoc job...';
      clearAdhocPoll();
      adhocActiveJobID = '';
      try {
        const res = await fetch('/api/v1/agents/' + encodeURIComponent(agentID) + '/run-script', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ shell: shell, script: script, timeout_seconds: 600 }),
        });
        if (!res.ok) throw new Error(await res.text());
        const data = await res.json();
        const jobID = String(data.job_execution_id || '').trim();
        if (!jobID) throw new Error('server response missing job_execution_id');
        adhocActiveJobID = jobID;
        showJobStartedSnackbar('Adhoc script started', jobID);
        adhocOutput.textContent = '[queued] job_execution_id=' + jobID + '\n[poll] waiting for agent output...';
        pollAdhocJob(jobID);
      } catch (e) {
        adhocRunBtn.disabled = false;
        adhocRunBtn.textContent = 'Run';
        adhocOutput.textContent = '[run failed] ' + String(e.message || e);
      }
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
      if (refreshInFlight || (!force && refreshGuard.shouldPause())) {
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
        const runAdhocButton = document.getElementById('runAdhocBtn');
        adhocShells = parseAgentShells(a.capabilities || {});
        const showUpdate = (!a.update_in_progress) && (!!a.update_requested || (!!a.needs_update && s.label !== 'offline'));
        updateButton.style.display = showUpdate ? 'inline-block' : 'none';
        updateButton.textContent = a.update_requested ? 'Retry Now' : 'Update';
        refreshToolsButton.style.display = s.label !== 'offline' ? 'inline-block' : 'none';
        runAdhocButton.style.display = adhocShells.length > 0 ? 'inline-block' : 'none';

        let updateState = '';
        if (a.update_requested) {
          updateState = formatUpdatePrimaryText(a);
          const retryText = formatUpdateRetryText(a);
          if (retryText) {
            updateState += ' ' + retryText;
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
    document.getElementById('runAdhocBtn').onclick = () => {
      if (adhocShells.length === 0) {
        alert('Agent does not advertise script shell capabilities.');
        return;
      }
      openAdhocModal();
    };
    wireModalCloseBehavior(adhocModalOverlay, closeAdhocModal);
    adhocShellSelect.onchange = () => {
      const suggested = exampleScriptForShell(String(adhocShellSelect.value || ''));
      adhocScriptInput.value = suggested;
      lastSuggestedScript = suggested;
    };
    adhocRunBtn.onclick = () => runAdhocScript();
    document.getElementById('adhocCloseBtn').onclick = () => closeAdhocModal();
    refreshGuard.bindSelectionListener();
    refreshAgent(true);
    setInterval(() => refreshAgent(false), 3000);
  </script>
</body>
</html>`
