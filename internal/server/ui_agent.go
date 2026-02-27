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
    .cap-code {
      display: inline-block;
      margin: 0 6px 6px 0;
      padding: 1px 6px;
      border: 1px solid var(--line);
      border-radius: 6px;
      background: #f7fcf9;
      font-family: ui-monospace,SFMono-Regular,Menlo,Monaco,Consolas,"Liberation Mono","Courier New",monospace;
      font-size: 12px;
      line-height: 1.4;
    }
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
    .agent-history-table {
      width:100%;
      border-collapse:collapse;
      font-size:13px;
      table-layout:fixed;
    }
    .agent-history-table th,
    .agent-history-table td {
      border-bottom:1px solid var(--line);
      padding:8px 6px;
      vertical-align:top;
      text-align:left;
      overflow-wrap:anywhere;
      word-break:break-word;
    }
    .agent-history-table th { color:var(--muted); font-weight:600; }
    .agent-history-empty { color:var(--muted); font-size:13px; padding:8px 0; }
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
          <button id="activationBtn">Deactivate</button>
          <button id="updateBtn" style="display:none;">Update</button>
          <button id="restartBtn" style="display:none;">Restart Agent</button>
          <button id="wipeCacheBtn" style="display:none;">Wipe Cache</button>
          <button id="flushAgentHistoryBtn">Flush Job History</button>
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

    <div class="card">
      <div class="row" style="margin-bottom:8px;">
        <div style="font-weight:600;">Job History</div>
        <div id="jobHistoryMeta" class="muted">Loading...</div>
      </div>
      <div id="jobHistoryEmpty" class="agent-history-empty" style="display:none;"></div>
      <table id="jobHistoryTable" class="agent-history-table" style="display:none;">
        <thead>
          <tr>
            <th>Description</th>
            <th>Status</th>
            <th>Pipeline</th>
            <th>Build</th>
            <th>Created</th>
            <th>Finished</th>
          </tr>
        </thead>
        <tbody id="jobHistoryBody"></tbody>
      </table>
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
` + agentHelpersJS + `
` + agentRuntimeJS + `
  </script>
</body>
</html>`
