package server

const jobExecutionHTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="UTF-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>ciwi job execution</title>
  <link rel="icon" type="image/png" href="/ciwi-favicon.png" />
  <script src="/ui/theme.js"></script>
  <style>
` + uiPageChromeCSS + `
    .top { display:flex; justify-content:space-between; align-items:center; gap:16px; flex-wrap:nowrap; }
    .detail-split { display:grid; grid-template-columns: 1fr 1fr; gap:12px; margin-bottom: 16px; }
    .detail-split > .card { margin-bottom: 0; }
    .meta-grid { display:grid; grid-template-columns: 160px 1fr; gap:8px 12px; font-size:14px; }
    .label { color: var(--muted); }
    .mode-value { display:inline-flex; align-items:center; gap:8px; }
    .mode-info {
      display: inline-block;
      color: var(--accent-strong);
      font-size: 14px;
      font-weight: 700;
      line-height: 1;
      cursor: help;
    }
    .mode-info > span {
      display: block;
      line-height: 1;
    }
    .job-actions {
      display: flex;
      gap: 8px;
      align-items: center;
      flex-wrap: wrap;
      justify-content: flex-end;
      flex: 0 0 auto;
    }
    .rerun-action-wrap {
      position: relative;
      display: inline-flex;
      align-items: flex-start;
      flex-direction: column;
      align-self: center;
    }
    .rerun-action-wrap .mode-info {
      position: absolute;
      top: -18px;
      left: 50%;
      transform: translateX(-50%);
    }
    .rerun-blocked-link {
      margin-top: 6px;
      font-size: 12px;
    }
    .status-succeeded { color: var(--ok); font-weight: 700; }
    .status-failed { color: var(--bad); font-weight: 700; }
    .status-blocked { color: var(--warn); font-weight: 700; }
    .status-running { color: var(--warn); font-weight: 700; }
    .status-queued, .status-leased, .status-waiting { color: var(--muted); font-weight: 700; }
    .job-subtitle-detail {
      margin-top: 4px;
      font-size: 12px;
      color: var(--muted);
      display: flex;
      align-items: center;
      gap: 6px;
      max-width: min(90vw, 960px);
      min-width: 0;
    }
    .job-subtitle-detail code {
      display: block;
      flex: 1 1 auto;
      min-width: 0;
      white-space: nowrap;
      overflow: hidden;
      text-overflow: ellipsis;
      font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", "Courier New", monospace;
      font-size: 11px;
      background: var(--code-bg);
      border: 1px solid var(--code-line);
      border-radius: 4px;
      padding: 1px 5px;
      color: var(--ink);
    }
    .log {
      margin: 0;
      background: var(--console-bg);
      color: var(--console-ink);
      border-radius: 8px;
      border: 1px solid var(--console-line);
      padding: 12px;
      width: 100%;
      max-height: 65vh;
      min-height: 320px;
      overflow: auto;
      font-size: 12px;
      line-height: 1.35;
      white-space: pre-wrap;
      font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", "Courier New", monospace;
    }
    .log-line { display:block; }
    .log-line.phase-meta { color: var(--console-blue); }
    .log-line.phase-checkout { color: var(--console-green); }
    .log-line.phase-run { color: var(--console-yellow); }
    .log-line.shell-trace { color: var(--console-muted); }
    .log-line.risky-cmd { color: var(--console-warn); }
    .log-dryskip {
      border-left: 3px solid var(--console-yellow);
      background: color-mix(in srgb, var(--console-yellow) 10%, transparent);
      padding: 6px 8px;
      margin: 4px 0;
      border-radius: 4px;
    }
    .log-dryskip-head { color: var(--console-yellow); font-weight: 700; }
    .log-dryskip-body { margin-top: 3px; color: var(--console-ink); white-space: pre-wrap; }
    details.log-fold {
      margin: 6px 0;
      border-left: 3px solid var(--console-line);
      background: color-mix(in srgb, var(--console-surface) 70%, transparent);
      border-radius: 4px;
      padding: 4px 8px;
    }
    details.log-fold > summary { cursor: pointer; color: var(--console-accent); }
    details.log-fold pre {
      margin: 8px 0 2px;
      white-space: pre-wrap;
      color: var(--console-muted);
      font: inherit;
    }
    details.log-step {
      margin: 8px 0;
      border-left: 3px solid var(--console-accent);
      background: var(--console-surface);
      border-radius: 6px;
      padding: 6px 10px;
    }
    .log-system-message {
      margin: 8px 0;
      padding: 8px 12px;
      border-left: 3px solid var(--console-accent);
      color: var(--console-muted);
    }
    details.log-step > summary {
      cursor: pointer;
      color: var(--console-ink);
      font-weight: 700;
      display: flex;
      align-items: baseline;
      gap: 8px;
      min-width: 0;
      white-space: nowrap;
      margin: -6px -10px;
      padding: 6px 10px;
      border-radius: 5px;
      --ciwi-progress-color: color-mix(in srgb, var(--console-green) 18%, transparent);
    }
    details.log-step[open] > summary { margin-bottom: 6px; }
    .log-step-summary-title { flex: 0 0 auto; }
    .log-step-summary-command {
      flex: 1 1 auto;
      min-width: 0;
      overflow: hidden;
      text-overflow: ellipsis;
      color: var(--console-muted);
      font-size: 11px;
      font-weight: 600;
    }
    details.log-step-unreached {
      border-left-color: var(--console-muted);
      background: color-mix(in srgb, var(--console-surface) 55%, transparent);
    }
    details.log-step-unreached > summary {
      color: var(--console-muted);
      --ciwi-progress-color: transparent;
    }
    .log-step-status {
      flex: 0 0 auto;
      color: var(--console-muted);
      font-size: 11px;
      font-weight: 700;
    }
    .log-step-collapse-btn {
      position: sticky;
      top: 8px;
      z-index: 3;
      float: right;
      display: inline-flex;
      align-items: center;
      margin: 2px 0 8px 10px;
      border: 1px solid var(--console-line);
      background: var(--console-surface);
      color: var(--console-ink);
      box-shadow: 0 2px 8px var(--shadow);
    }
    .log-step-collapse-btn:hover {
      background: color-mix(in srgb, var(--console-surface) 80%, var(--console-accent));
    }
    .log-step-collapse-btn[hidden] {
      display: none;
    }
    .log-step-meta {
      display: flex;
      flex-wrap: wrap;
      gap: 8px;
      margin: 8px 0;
      color: var(--console-muted);
      font-size: 11px;
    }
    .log-step-label {
      color: var(--console-accent);
      font-weight: 700;
      margin: 8px 0 3px;
    }
    .log-step pre {
      margin: 0 0 8px;
      white-space: pre-wrap;
      color: var(--console-ink);
      font: inherit;
    }
    .tok-version { color: var(--console-yellow); font-weight: 700; }
    .tok-sha { color: var(--console-blue); }
    .tok-duration { color: var(--console-green); font-weight: 700; }
    .tok-url { color: var(--console-blue); }
    .log-empty { color: var(--console-muted); }
    .artifact-row {
      display: flex;
      align-items: center;
      gap: 8px;
      flex-wrap: wrap;
      margin-bottom: 6px;
    }
    .artifact-path {
      font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", "Courier New", monospace;
      user-select: text;
      cursor: text;
      color: var(--ink);
    }
    .copy-btn {
      font-weight: 600;
    }
    .card-head-row {
      display: flex;
      justify-content: space-between;
      align-items: center;
      margin: 0 0 10px;
    }
    .artifact-tree,
    .artifact-tree ul {
      list-style: none;
      margin: 0;
      padding-left: 14px;
    }
    .artifact-tree {
      padding-left: 0;
    }
    .artifact-tree summary {
      cursor: pointer;
      user-select: none;
      color: var(--ink);
      font-weight: 600;
    }
    .artifact-dir-download {
      margin-left: 8px;
      font-size: 12px;
      font-weight: 500;
    }
    .artifact-leaf {
      margin: 4px 0;
    }
    .test-summary-row {
      display: flex;
      gap: 8px;
      flex-wrap: wrap;
      margin: 8px 0;
    }
    .test-pill {
      display: inline-block;
      border-radius: 999px;
      padding: 4px 10px;
      font-size: 12px;
      font-weight: 700;
      border: 1px solid var(--line);
      background: var(--surface-soft);
      color: var(--ink);
    }
    .test-pill-pass { background: var(--ok-bg); border-color: var(--ok-line); color: var(--ok); }
    .test-pill-fail { background: var(--bad-bg); border-color: var(--bad-line); color: var(--bad); }
    .test-pill-skip { background: var(--warn-bg); border-color: var(--warn-line); color: var(--warn); }
    .test-filter-row {
      display: flex;
      gap: 6px;
      flex-wrap: wrap;
      margin: 6px 0 10px;
    }
    .test-filter-btn {
      border: 1px solid var(--line);
      background: var(--input-bg);
      color: var(--ink);
      border-radius: 6px;
      padding: 4px 8px;
      font-size: 12px;
      cursor: pointer;
    }
    .test-filter-btn.active {
      background: var(--surface-hover);
      border-color: var(--accent);
      color: var(--accent-strong);
      font-weight: 700;
    }
    .log-toolbar {
      display: flex;
      gap: 8px;
      margin: 0 0 10px;
      flex-wrap: wrap;
      align-items: center;
    }
    .log-download-wrap {
      display: inline-flex;
      align-items: center;
      gap: 4px;
    }
    .log-search-input {
      min-width: 180px;
      height: 32px;
      padding: 4px 8px;
      border: 1px solid var(--line);
      background: var(--input-bg);
      color: var(--ink);
      border-radius: 6px;
      font-size: 13px;
    }
    .log-search-count {
      min-width: 44px;
      text-align: center;
      color: var(--muted);
      font-size: 12px;
      font-weight: 600;
      align-self: center;
    }
    .tail-on {
      border-color: var(--ok-line);
      background: var(--ok-bg);
      color: var(--ok);
    }
    .tail-off {
      border-color: var(--warn-line);
      background: var(--warn-bg);
      color: var(--warn);
    }
    .job-header-icon {
      width: 100px;
      height: 100px;
      object-fit: contain;
      border: none;
      background: transparent;
      image-rendering: pixelated;
      image-rendering: crisp-edges;
    }
    .cache-stats-empty { color:var(--muted); font-size:14px; }
    .cache-stats-list { display:flex; flex-direction:column; gap:8px; }
    .cache-stat-item { border:1px solid var(--line); border-radius:8px; padding:8px 10px; background:var(--surface-subtle); }
    .cache-stat-head { display:flex; gap:6px; align-items:center; flex-wrap:wrap; margin-bottom:4px; }
    .cache-stat-title { font-weight:700; }
    .cache-stat-pill { font-size:11px; border:1px solid var(--line); border-radius:999px; padding:1px 6px; color:var(--pill-ink); background:var(--pill-bg); }
    .cache-stat-row { font-size:12px; color:var(--ink); margin-top:2px; }
    .cache-stat-metrics { margin-top:6px; font-size:12px; color:var(--muted); }
    .cache-stat-metrics code { font-size:11px; }
    .req-empty { color:var(--muted); font-size:13px; }
    .req-ok { padding:8px 10px; border:1px solid var(--ok-line); background:var(--ok-bg); border-radius:8px; color:var(--ok); font-size:13px; }
    .req-issues { padding:8px 10px; border:1px solid var(--bad-line); background:var(--bad-bg); border-radius:8px; color:var(--bad); font-size:13px; }
    .req-issues ul { margin:6px 0 0 18px; padding:0; }
    @media (max-width: 980px) {
      .detail-split { grid-template-columns: 1fr; }
      .top { flex-wrap: wrap; }
      .job-actions {
        width: 100%;
        justify-content: flex-start;
      }
    }
` + uiGraphCSS + `
` + jobExecutionGraphCSS + `
  </style>
</head>
<body>
  <main>
    <div class="card top" id="jobHeaderCard">
      <div class="brand">
        <img id="jobProjectIcon" class="job-header-icon" alt="" style="display:none;" />
        <div>
          <div style="font-size:20px;font-weight:700;" id="jobTitle">Job Execution</div>
          <div class="muted" id="subtitle">Loading...</div>
        </div>
      </div>
      <div class="job-actions">
        <button id="forceFailBtn" class="copy-btn" style="display:none;">Cancel</button>
        <span class="rerun-action-wrap">
          <span id="rerunInfo" class="mode-info" tabindex="0" aria-label="Run Job Again info">
            <span aria-hidden="true"><svg class="ciwi-icon" focusable="false"><use href="/ui/icons.svg#icon-info-circle"></use></svg></span>
          </span>
          <button id="rerunBtn" class="copy-btn" type="button" disabled>Run Job Again</button>
          <a id="rerunBlockedLink" class="rerun-blocked-link" href="#" style="display:none;">Open failed dependency</a>
        </span>
        <a id="backLink" class="nav-btn" href="/"><span class="nav-emoji" aria-hidden="true"><svg class="ciwi-icon" focusable="false"><use href="/ui/icons.svg#icon-arrow-left"></use></svg></span> Back to Job Executions</a>
      </div>
    </div>

    <div class="detail-split">
      <div class="card">
        <h3 style="margin:0 0 10px;">Job Properties</h3>
        <div class="meta-grid" id="metaGrid"></div>
      </div>
      <div class="card">
        <h3 style="margin:0 0 10px;">Cache Statistics</h3>
        <div id="cacheStatsBox" class="cache-stats-empty">No cache statistics reported for this job.</div>
      </div>
      <div class="card">
        <h3 style="margin:0 0 10px;">Host Tool Requirements</h3>
        <div id="hostToolReqBox" class="req-empty">No tool requirements declared for this job.</div>
      </div>
      <div class="card">
        <h3 style="margin:0 0 10px;">Container Tool Requirements</h3>
        <div id="containerToolReqBox" class="req-empty">No container tool requirements declared for this job.</div>
      </div>
    </div>
    <div class="card" id="releaseSummaryCard" style="display:none;">
      <h3 style="margin:0 0 10px;">Release Summary</h3>
      <div id="releaseSummaryBox" style="font-size:14px;color:var(--ink);"></div>
    </div>

    <div class="card" id="runContextCard" style="display:none;">
      <div class="run-context-card-head">
        <h3 style="margin:0;">Run Context</h3>
        <button id="runContextToggleBtn" class="copy-btn" type="button">Collapse</button>
      </div>
      <div id="runContextBody" class="run-context-body"></div>
    </div>

    <div class="card">
      <h3 style="margin:0 0 10px;">Output / Error</h3>
      <div class="log-toolbar">
        <button id="tailToggleBtn" class="copy-btn tail-on" type="button">Tailing: On</button>
        <button id="copyOutputBtn" class="copy-btn" type="button">Copy Output</button>
        <span class="log-download-wrap">
          <a id="downloadCleanLogBtn" class="copy-btn nav-btn" href="#">Download Clean Log</a>
          <span class="mode-info log-info" tabindex="0" aria-label="Clean log info" data-log-info="clean"><span aria-hidden="true"><svg class="ciwi-icon" focusable="false"><use href="/ui/icons.svg#icon-info-circle"></use></svg></span></span>
        </span>
        <span class="log-download-wrap">
          <a id="downloadRawLogBtn" class="copy-btn nav-btn" href="#">Download Raw Log</a>
          <span class="mode-info log-info" tabindex="0" aria-label="Raw log info" data-log-info="raw"><span aria-hidden="true"><svg class="ciwi-icon" focusable="false"><use href="/ui/icons.svg#icon-info-circle"></use></svg></span></span>
        </span>
        <input id="logSearchInput" class="log-search-input" type="search" placeholder="Search output" aria-label="Search output" />
        <button id="logSearchPrevBtn" class="copy-btn ciwi-icon-only" type="button" aria-label="Previous match" title="Previous match"><svg class="ciwi-icon" aria-hidden="true" focusable="false"><use href="/ui/icons.svg#icon-chevron-up"></use></svg></button>
        <button id="logSearchNextBtn" class="copy-btn ciwi-icon-only" type="button" aria-label="Next match" title="Next match"><svg class="ciwi-icon" aria-hidden="true" focusable="false"><use href="/ui/icons.svg#icon-chevron-down"></use></svg></button>
        <span id="logSearchCount" class="log-search-count">0/0</span>
      </div>
      <div id="executionStepNavigator" class="execution-step-navigator" style="display:none;"></div>
      <div id="logBox" class="log"></div>
    </div>

    <div class="card">
      <div class="card-head-row">
        <h3 style="margin:0;">Artifacts</h3>
        <a id="artifactsDownloadAllBtn" class="copy-btn nav-btn" href="#" style="display:none;">Download All (.zip)</a>
      </div>
      <div id="artifactsBox" class="muted" style="font-size:14px;">Loading...</div>
    </div>
    <div class="card">
      <h3 style="margin:0 0 10px;">Test Report</h3>
      <div id="testReportBox" class="muted" style="font-size:14px;">Loading...</div>
    </div>
    <div class="card">
      <h3 style="margin:0 0 10px;">Coverage Report</h3>
      <div id="coverageReportBox" class="muted" style="font-size:14px;">Loading...</div>
    </div>
  </main>

  <script src="/ui/shared.js"></script>
  <script src="/ui/pages.js"></script>
  <script>
` + uiGraphJS + `
` + jobExecutionRenderJS + `
` + jobExecutionGraphJS + `
` + jobExecutionDataJS + `
  </script>
</body>
</html>`
