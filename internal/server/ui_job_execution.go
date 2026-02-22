package server

const jobExecutionHTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="UTF-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>ciwi job execution</title>
  <link rel="icon" type="image/png" href="/ciwi-favicon.png" />
  <style>
` + uiPageChromeCSS + `
    .top { display:flex; justify-content:space-between; align-items:center; gap:8px; flex-wrap:wrap; }
    .detail-split { display:grid; grid-template-columns: 1fr 1fr; gap:12px; margin-bottom: 16px; }
    .detail-split > .card { margin-bottom: 0; }
    .meta-grid { display:grid; grid-template-columns: 160px 1fr; gap:8px 12px; font-size:14px; }
    .label { color: var(--muted); }
    .mode-value { display:inline-flex; align-items:center; gap:8px; }
    .mode-info {
      display: inline-block;
      color: #28503f;
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
    }
    .rerun-action-wrap {
      position: relative;
      display: inline-flex;
      align-items: center;
      align-self: center;
    }
    .rerun-action-wrap .mode-info {
      position: absolute;
      top: -18px;
      left: 50%;
      transform: translateX(-50%);
    }
    .status-succeeded { color: var(--ok); font-weight: 700; }
    .status-failed { color: var(--bad); font-weight: 700; }
    .status-running { color: #a56a00; font-weight: 700; }
    .status-queued, .status-leased { color: var(--muted); font-weight: 700; }
    .log {
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
      white-space: pre-wrap;
      font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", "Courier New", monospace;
    }
    .log-line { display:block; }
    .log-line.phase-meta { color: #8fd8ff; }
    .log-line.phase-checkout { color: #a6e3a1; }
    .log-line.phase-run { color: #f9d88d; }
    .log-line.shell-trace { color: #c2d7cc; }
    .log-line.risky-cmd { color: #ffd7a8; }
    .log-dryskip {
      border-left: 3px solid #b48a47;
      background: rgba(180, 138, 71, 0.1);
      padding: 6px 8px;
      margin: 4px 0;
      border-radius: 4px;
    }
    .log-dryskip-head { color: #ffd68c; font-weight: 700; }
    .log-dryskip-body { margin-top: 3px; color: #f3dfba; white-space: pre-wrap; }
    details.log-fold {
      margin: 6px 0;
      border-left: 3px solid #365547;
      background: rgba(54, 85, 71, 0.2);
      border-radius: 4px;
      padding: 4px 8px;
    }
    details.log-fold > summary { cursor: pointer; color: #9bc4b1; }
    details.log-fold pre {
      margin: 8px 0 2px;
      white-space: pre-wrap;
      color: #b7d3c7;
      font: inherit;
    }
    .tok-version { color: #ffd68c; font-weight: 700; }
    .tok-sha { color: #8fd8ff; }
    .tok-duration { color: #a6e3a1; font-weight: 700; }
    .tok-url { color: #87c7ff; }
    .log-empty { color: #8ea89d; }
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
      color: #1f2a24;
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
      color: #1f2a24;
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
      border: 1px solid #c4ddd0;
      background: #f6fbf8;
      color: #234338;
    }
    .test-pill-pass { background: #e9f8ef; border-color: #9fd3b2; color: #1f6b3f; }
    .test-pill-fail { background: #fde9e8; border-color: #f0b3af; color: #9b2c2c; }
    .test-pill-skip { background: #fff5e6; border-color: #e8c98f; color: #8a5a14; }
    .test-filter-row {
      display: flex;
      gap: 6px;
      flex-wrap: wrap;
      margin: 6px 0 10px;
    }
    .test-filter-btn {
      border: 1px solid #c4ddd0;
      background: #ffffff;
      color: #2e4b3d;
      border-radius: 6px;
      padding: 4px 8px;
      font-size: 12px;
      cursor: pointer;
    }
    .test-filter-btn.active {
      background: #e6f2eb;
      border-color: #8db8a2;
      color: #1f3d31;
      font-weight: 700;
    }
    .log-toolbar {
      display: flex;
      gap: 8px;
      margin: 0 0 10px;
      flex-wrap: wrap;
    }
    .log-search-input {
      min-width: 180px;
      height: 32px;
      padding: 4px 8px;
      border: 1px solid #c4ddd0;
      border-radius: 6px;
      font-size: 13px;
    }
    .log-search-count {
      min-width: 44px;
      text-align: center;
      color: #42574b;
      font-size: 12px;
      font-weight: 600;
      align-self: center;
    }
    .tail-on {
      border-color: #3f7a5a;
      background: #e9f6ef;
      color: #1f4e37;
    }
    .tail-off {
      border-color: #8a7448;
      background: #f7f2e8;
      color: #614f2c;
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
    .cache-stats-empty { color:#5f6f67; font-size:14px; }
    .cache-stats-list { display:flex; flex-direction:column; gap:8px; }
    .cache-stat-item { border:1px solid #d9e7df; border-radius:8px; padding:8px 10px; background:#f8fcfa; }
    .cache-stat-head { display:flex; gap:6px; align-items:center; flex-wrap:wrap; margin-bottom:4px; }
    .cache-stat-title { font-weight:700; }
    .cache-stat-pill { font-size:11px; border:1px solid #c4ddd0; border-radius:999px; padding:1px 6px; color:#2a5a45; background:#edf8f2; }
    .cache-stat-row { font-size:12px; color:#1f2a24; margin-top:2px; }
    .cache-stat-metrics { margin-top:6px; font-size:12px; color:#30463b; }
    .cache-stat-metrics code { font-size:11px; }
    .req-empty { color:#5f6f67; font-size:13px; }
    .req-ok { padding:8px 10px; border:1px solid #cfe8d8; background:#f3fbf6; border-radius:8px; color:#21553a; font-size:13px; }
    .req-issues { padding:8px 10px; border:1px solid #e8cfcf; background:#fff5f5; border-radius:8px; color:#7a2f2f; font-size:13px; }
    .req-issues ul { margin:6px 0 0 18px; padding:0; }
    @media (max-width: 980px) {
      .detail-split { grid-template-columns: 1fr; }
    }
  </style>
</head>
<body>
  <main>
    <div class="card top">
      <div class="brand">
        <img id="jobProjectIcon" class="job-header-icon" alt="" style="display:none;" />
        <div>
          <div style="font-size:20px;font-weight:700;" id="jobTitle">Job Execution</div>
          <div style="color:#5f6f67;" id="subtitle">Loading...</div>
        </div>
      </div>
      <div class="job-actions">
        <button id="forceFailBtn" class="copy-btn" style="display:none;">Cancel</button>
        <span class="rerun-action-wrap">
          <span id="rerunInfo" class="mode-info" tabindex="0" aria-label="Run Job Again info">
            <span aria-hidden="true">ⓘ</span>
          </span>
          <button id="rerunBtn" class="copy-btn" type="button" disabled>Run Job Again</button>
        </span>
        <a id="backLink" class="nav-btn" href="/">Back to Job Executions <span class="nav-emoji" aria-hidden="true">↩</span></a>
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
      <div id="releaseSummaryBox" style="font-size:14px;color:#1f2a24;"></div>
    </div>

    <div class="card">
      <h3 style="margin:0 0 10px;">Output / Error</h3>
      <div class="log-toolbar">
        <button id="tailToggleBtn" class="copy-btn tail-on" type="button">Tailing: On</button>
        <button id="copyOutputBtn" class="copy-btn" type="button">Copy Output</button>
        <input id="logSearchInput" class="log-search-input" type="search" placeholder="Search output" aria-label="Search output" />
        <button id="logSearchPrevBtn" class="copy-btn" type="button" aria-label="Previous match">▲</button>
        <button id="logSearchNextBtn" class="copy-btn" type="button" aria-label="Next match">▼</button>
        <span id="logSearchCount" class="log-search-count">0/0</span>
      </div>
      <div id="logBox" class="log"></div>
    </div>

    <div class="card">
      <div class="card-head-row">
        <h3 style="margin:0;">Artifacts</h3>
        <a id="artifactsDownloadAllBtn" class="copy-btn nav-btn" href="#" style="display:none;">Download All (.zip)</a>
      </div>
      <div id="artifactsBox" style="font-size:14px;color:#5f6f67;">Loading...</div>
    </div>
    <div class="card">
      <h3 style="margin:0 0 10px;">Test Report</h3>
      <div id="testReportBox" style="font-size:14px;color:#5f6f67;">Loading...</div>
    </div>
    <div class="card">
      <h3 style="margin:0 0 10px;">Coverage Report</h3>
      <div id="coverageReportBox" style="font-size:14px;color:#5f6f67;">Loading...</div>
    </div>
  </main>

  <script src="/ui/shared.js"></script>
  <script>
` + jobExecutionRenderJS + `
` + jobExecutionDataJS + `
  </script>
</body>
</html>`
