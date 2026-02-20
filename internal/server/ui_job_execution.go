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
    .artifacts-toolbar {
      display: flex;
      justify-content: flex-end;
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
    .artifact-leaf {
      margin: 4px 0;
    }
    .log-toolbar {
      display: flex;
      gap: 8px;
      margin: 0 0 10px;
      flex-wrap: wrap;
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
      <div style="display:flex;gap:8px;align-items:center;flex-wrap:wrap;">
        <button id="forceFailBtn" class="copy-btn" style="display:none;">Cancel</button>
        <span id="rerunInfo" class="mode-info" tabindex="0" aria-label="Run Job Again info">
          <span aria-hidden="true">ⓘ</span>
        </span>
        <button id="rerunBtn" class="copy-btn" type="button" disabled>Run Job Again</button>
        <a id="backLink" class="nav-btn" href="/">Back to Job Executions <span class="nav-emoji" aria-hidden="true">↩</span></a>
      </div>
    </div>

    <div class="card">
      <div class="meta-grid" id="metaGrid"></div>
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
      </div>
      <div id="logBox" class="log"></div>
    </div>

    <div class="card">
      <h3 style="margin:0 0 10px;">Artifacts</h3>
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
    let refreshInFlight = false;
    let lastRenderedOutput = null;
    let lastOutputRaw = '';
    let tailingEnabled = true;
    let suppressLogScrollEvent = false;
    let projectIDByNameCache = null;
    let lastCoverageSignature = null;
    let lastTestReportSignature = '';
    let lastArtifactsSignature = '';
    let artifactExpandedPaths = null;
    const refreshGuard = createRefreshGuard(5000);

    function jobExecutionIdFromPath() {
      const parts = window.location.pathname.split('/').filter(Boolean);
      return parts.length >= 2 ? decodeURIComponent(parts[1]) : '';
    }

    function parseOptionalTimestamp(ts) {
      const raw = String(ts || '').trim();
      if (!raw) return null;
      const parsed = new Date(raw);
      if (Number.isNaN(parsed.getTime())) return null;
      return parsed;
    }

    function computeJobExecutionDuration(startTs, finishTs, status) {
      const start = parseOptionalTimestamp(startTs);
      if (!start) return null;
      const running = isRunningJobStatus(status);
      const finish = parseOptionalTimestamp(finishTs);
      const end = (running || !finish) ? new Date() : finish;
      if (Number.isNaN(end.getTime())) return null;
      let ms = end.getTime() - start.getTime();
      if (ms < 0) ms = 0;
      return {
        ms: ms,
        isRunningWithoutFinish: running && !finish,
      };
    }

    function formatJobExecutionDuration(startTs, finishTs, status) {
      const duration = computeJobExecutionDuration(startTs, finishTs, status);
      if (!duration) return '';
      const core = formatDurationMs(duration.ms);
      if (!core) return '';
      if (duration.isRunningWithoutFinish) return core + ' (running)';
      return core;
    }

    function renderModeValue(dryRun) {
      const label = dryRun ? 'Dry run' : 'Ordinary run';
      return '' +
        '<span class="mode-value">' +
          '<span>' + label + '</span>' +
          '<span class="mode-info" tabindex="0" aria-label="Run mode info" data-mode="' + (dryRun ? 'dry' : 'ordinary') + '">' +
            '<span aria-hidden="true">ⓘ</span>' +
          '</span>' +
        '</span>';
    }

    function setBackLink() {
      const link = document.getElementById('backLink');
      if (!link) return;
      const params = new URLSearchParams(window.location.search || '');
      const back = params.get('back') || '';
      if (back && back.startsWith('/')) {
        link.href = back;
        link.innerHTML = (back.startsWith('/projects/') ? 'Back to Project' : 'Back to Job Executions') + ' <span class="nav-emoji" aria-hidden="true">↩</span>';
        return;
      }
      link.href = '/';
      link.innerHTML = 'Back to Job Executions <span class="nav-emoji" aria-hidden="true">↩</span>';
    }

    function buildArtifactTree(items) {
      const root = { dirs: {}, files: [] };
      items.forEach((a, idx) => {
        const raw = String((a && a.path) || '').trim();
        if (!raw) return;
        const parts = raw.split('/').filter(Boolean);
        if (parts.length === 0) return;
        let node = root;
        for (let i = 0; i < parts.length - 1; i += 1) {
          const seg = parts[i];
          if (!node.dirs[seg]) node.dirs[seg] = { dirs: {}, files: [] };
          node = node.dirs[seg];
        }
        node.files.push({ name: parts[parts.length - 1], item: a, idx: idx });
      });
      return root;
    }

    function collectArtifactExpandedPaths(box) {
      const out = new Set();
      if (!box) return out;
      box.querySelectorAll('details[data-artifact-dir]').forEach(d => {
        const p = String(d.getAttribute('data-artifact-dir') || '').trim();
        if (d.open && p) out.add(p);
      });
      return out;
    }

    function renderArtifactTreeNode(node, parentPath, depth, expanded) {
      const dirNames = Object.keys(node.dirs).sort((a, b) => a.localeCompare(b));
      const files = (node.files || []).slice().sort((a, b) => a.name.localeCompare(b.name));
      let html = '<ul class="artifact-tree">';
      dirNames.forEach(name => {
        const path = parentPath ? (parentPath + '/' + name) : name;
        const open = expanded.has(path);
        html += '<li><details data-artifact-dir="' + escapeHtml(path) + '"' + (open ? ' open' : '') + '><summary>' + escapeHtml(name) + '</summary>' + renderArtifactTreeNode(node.dirs[name], path, depth + 1, expanded) + '</details></li>';
      });
      files.forEach(entry => {
        const a = entry.item || {};
        html += '' +
          '<li class="artifact-leaf">' +
            '<div class="artifact-row">' +
              '<span class="artifact-path">' + escapeHtml(entry.name) + '</span>' +
              '<span>(' + formatBytes(a.size_bytes) + ')</span>' +
              '<a href=\"' + a.url + '\" target=\"_blank\" rel=\"noopener\">Download</a>' +
              '<button class="copy-btn" data-artifact-index="' + String(entry.idx) + '">Copy</button>' +
            '</div>' +
          '</li>';
      });
      html += '</ul>';
      return html;
    }

    function renderArtifacts(box, jobId, items) {
      const signature = JSON.stringify(items.map(a => [String(a.path || ''), Number(a.size_bytes || 0), String(a.url || '')]));
      if (signature === lastArtifactsSignature) {
        return;
      }
      const previousExpanded = collectArtifactExpandedPaths(box);
      if (previousExpanded.size > 0) {
        artifactExpandedPaths = previousExpanded;
      }
      if (items.length === 0) {
        box.textContent = 'No artifacts';
        lastArtifactsSignature = signature;
        return;
      }
      const tree = buildArtifactTree(items);
      const expanded = (artifactExpandedPaths && artifactExpandedPaths.size > 0)
        ? new Set(artifactExpandedPaths)
        : new Set();
      if (expanded.size === 0) {
        // Default expansion is one directory level from root.
        Object.keys(tree.dirs || {}).forEach(name => expanded.add(name));
      }
      box.innerHTML =
        '<div class="artifacts-toolbar">' +
          '<a class="copy-btn nav-btn" href="/api/v1/jobs/' + encodeURIComponent(jobId) + '/artifacts/download-all">Download All (.zip)</a>' +
        '</div>' +
        renderArtifactTreeNode(tree, '', 0, expanded);
      box.querySelectorAll('button.copy-btn').forEach(btn => {
        btn.addEventListener('click', async () => {
          const idx = Number(btn.getAttribute('data-artifact-index') || '-1');
          const path = (items[idx] && items[idx].path) || '';
          if (!path) return;
          try {
            await navigator.clipboard.writeText(path);
            const old = btn.textContent;
            btn.textContent = 'Copied';
            setTimeout(() => { btn.textContent = old; }, 1000);
          } catch (_) {
            const old = btn.textContent;
            btn.textContent = 'Copy failed';
            setTimeout(() => { btn.textContent = old; }, 1200);
          }
        });
      });
      box.querySelectorAll('details[data-artifact-dir]').forEach(d => {
        d.addEventListener('toggle', () => {
          const path = String(d.getAttribute('data-artifact-dir') || '').trim();
          if (!path) return;
          if (artifactExpandedPaths == null) artifactExpandedPaths = new Set();
          if (d.open) artifactExpandedPaths.add(path);
          else artifactExpandedPaths.delete(path);
        });
      });
      artifactExpandedPaths = collectArtifactExpandedPaths(box);
      lastArtifactsSignature = signature;
    }

    function coverageTotals(c) {
      const total = Number(c.total_statements || c.total_lines || 0);
      const covered = Number(c.covered_statements || c.covered_lines || 0);
      return { total: total, covered: covered };
    }

    function coverageFileTotals(f) {
      const total = Number(f.total_statements || f.total_lines || 0);
      const covered = Number(f.covered_statements || f.covered_lines || 0);
      return { total: total, covered: covered };
    }

    function pct(covered, total) {
      if (!total) return 0;
      return (100 * covered) / total;
    }

    function renderCoverageReport(coverage) {
      const box = document.getElementById('coverageReportBox');
      if (!box) return;
      const openState = {};
      box.querySelectorAll('details[data-cov-key]').forEach(d => {
        const key = String(d.getAttribute('data-cov-key') || '');
        if (key) openState[key] = !!d.open;
      });
      if (!coverage) {
        box.textContent = 'No parsed coverage report';
        return;
      }
      const files = Array.isArray(coverage.files) ? coverage.files.slice() : [];
      const overall = coverageTotals(coverage);
      const overallPct = Number(coverage.percent || pct(overall.covered, overall.total) || 0);

      const modules = new Map();
      files.forEach(f => {
        const path = String(f.path || '').trim();
        const slash = path.lastIndexOf('/');
        const moduleName = slash > 0 ? path.slice(0, slash) : '.';
        const t = coverageFileTotals(f);
        const prev = modules.get(moduleName) || { total: 0, covered: 0, files: 0 };
        prev.total += t.total;
        prev.covered += t.covered;
        prev.files += 1;
        modules.set(moduleName, prev);
      });
      const moduleRows = Array.from(modules.entries())
        .sort((a, b) => pct(a[1].covered, a[1].total) - pct(b[1].covered, b[1].total))
        .map(([name, m]) =>
          '<tr>' +
          '<td style="padding:4px 6px;border-bottom:1px solid #d7e6dd;"><code>' + escapeHtml(name) + '</code></td>' +
          '<td style="padding:4px 6px;border-bottom:1px solid #d7e6dd;text-align:right;">' + m.files + '</td>' +
          '<td style="padding:4px 6px;border-bottom:1px solid #d7e6dd;text-align:right;">' + m.covered + '/' + m.total + '</td>' +
          '<td style="padding:4px 6px;border-bottom:1px solid #d7e6dd;text-align:right;"><strong>' + pct(m.covered, m.total).toFixed(2) + '%</strong></td>' +
          '</tr>'
        ).join('');

      const fileRows = files
        .slice()
        .sort((a, b) => pct(coverageFileTotals(a).covered, coverageFileTotals(a).total) - pct(coverageFileTotals(b).covered, coverageFileTotals(b).total))
        .map(f => {
          const t = coverageFileTotals(f);
          return '<tr>' +
            '<td style="padding:4px 6px;border-bottom:1px solid #d7e6dd;"><code>' + escapeHtml(String(f.path || '')) + '</code></td>' +
            '<td style="padding:4px 6px;border-bottom:1px solid #d7e6dd;text-align:right;">' + t.covered + '/' + t.total + '</td>' +
            '<td style="padding:4px 6px;border-bottom:1px solid #d7e6dd;text-align:right;"><strong>' + pct(t.covered, t.total).toFixed(2) + '%</strong></td>' +
            '</tr>';
        }).join('');

      const root = { name: '/', children: new Map(), total: 0, covered: 0, isFile: false };
      files.forEach(f => {
        const path = String(f.path || '').trim();
        if (!path) return;
        const t = coverageFileTotals(f);
        const parts = path.split('/').filter(Boolean);
        let node = root;
        node.total += t.total;
        node.covered += t.covered;
        parts.forEach((part, idx) => {
          const key = idx === parts.length - 1 ? 'f:' + part : 'd:' + part;
          if (!node.children.has(key)) {
            node.children.set(key, { name: part, children: new Map(), total: 0, covered: 0, isFile: idx === parts.length - 1 });
          }
          node = node.children.get(key);
          node.total += t.total;
          node.covered += t.covered;
        });
      });

      function nodeHtml(node, prefix) {
        const nodeKey = prefix ? (prefix + '/' + node.name) : node.name;
        const children = Array.from(node.children.values())
          .sort((a, b) => {
            if (a.isFile !== b.isFile) return a.isFile ? 1 : -1;
            return a.name.localeCompare(b.name);
          })
          .map(ch => nodeHtml(ch, nodeKey))
          .join('');
        const label = escapeHtml(node.name) + ' - ' + node.covered + '/' + node.total + ' (' + pct(node.covered, node.total).toFixed(2) + '%)';
        if (!children) {
          return '<li><code>' + label + '</code></li>';
        }
        const isOpen = Object.prototype.hasOwnProperty.call(openState, 'tree:' + nodeKey) ? !!openState['tree:' + nodeKey] : false;
        return '<li><details data-cov-key="tree:' + escapeHtml(nodeKey) + '"' + (isOpen ? ' open' : '') + '><summary><code>' + label + '</code></summary><ul style="margin:6px 0 0 18px;padding:0 0 0 12px;">' + children + '</ul></details></li>';
      }
      const tree = '<ul style="margin:6px 0 0 0;padding:0 0 0 12px;">' + Array.from(root.children.values()).map(ch => nodeHtml(ch, '')).join('') + '</ul>';
      const openModules = Object.prototype.hasOwnProperty.call(openState, 'modules') ? !!openState.modules : true;
      const openFiles = Object.prototype.hasOwnProperty.call(openState, 'files') ? !!openState.files : false;
      const openTree = Object.prototype.hasOwnProperty.call(openState, 'tree') ? !!openState.tree : false;

      box.innerHTML =
        '<div style="margin:0 0 10px;padding:8px;border:1px solid #c4ddd0;border-radius:6px;background:#f6fbf8;">' +
          '<div><strong>Format:</strong> ' + escapeHtml(String(coverage.format || '')) + '</div>' +
          '<div><strong>Overall:</strong> ' + overallPct.toFixed(2) + '% (' + overall.covered + '/' + overall.total + ')</div>' +
          '<div><strong>Files:</strong> ' + files.length + '</div>' +
        '</div>' +
        '<details data-cov-key="modules"' + (openModules ? ' open' : '') + '><summary><strong>By Module</strong></summary>' +
          '<table style="width:100%;border-collapse:collapse;margin-top:6px;font-size:12px;">' +
          '<thead><tr><th style="text-align:left;border-bottom:1px solid #c4ddd0;">Module</th><th style="text-align:right;border-bottom:1px solid #c4ddd0;">Files</th><th style="text-align:right;border-bottom:1px solid #c4ddd0;">Covered/Total</th><th style="text-align:right;border-bottom:1px solid #c4ddd0;">Coverage</th></tr></thead>' +
          '<tbody>' + moduleRows + '</tbody></table>' +
        '</details>' +
        '<details data-cov-key="files"' + (openFiles ? ' open' : '') + '><summary><strong>By File</strong></summary>' +
          '<table style="width:100%;border-collapse:collapse;margin-top:6px;font-size:12px;">' +
          '<thead><tr><th style="text-align:left;border-bottom:1px solid #c4ddd0;">File</th><th style="text-align:right;border-bottom:1px solid #c4ddd0;">Covered/Total</th><th style="text-align:right;border-bottom:1px solid #c4ddd0;">Coverage</th></tr></thead>' +
          '<tbody>' + fileRows + '</tbody></table>' +
        '</details>' +
        '<details data-cov-key="tree"' + (openTree ? ' open' : '') + '><summary><strong>Tree View</strong></summary>' + tree + '</details>';
    }

    function renderTestReport(report) {
      const box = document.getElementById('testReportBox');
      if (!box) return;
      const suites = report && Array.isArray(report.suites) ? report.suites : [];
      if (!suites.length) {
        box.textContent = 'No parsed test report';
        return;
      }
      const header = '<div><strong>Total:</strong> ' + (report.total || 0) +
        ' | <strong>Passed:</strong> ' + (report.passed || 0) +
        ' | <strong>Failed:</strong> ' + (report.failed || 0) +
        ' | <strong>Skipped:</strong> ' + (report.skipped || 0) + '</div>';

      const suiteHtml = suites.map((s, suiteIdx) => {
        const cases = Array.isArray(s.cases) ? s.cases : [];
        const modules = new Map();
        cases.forEach(c => {
          const mod = String(c.package || '').trim() || '(root)';
          if (!modules.has(mod)) modules.set(mod, []);
          modules.get(mod).push(c);
        });
        const moduleHtml = Array.from(modules.entries())
          .sort((a, b) => a[0].localeCompare(b[0]))
          .map(([mod, moduleCases], modIdx) => {
            let mPass = 0;
            let mFail = 0;
            let mSkip = 0;
            moduleCases.forEach(c => {
              const st = String(c.status || '').toLowerCase();
              if (st === 'pass') mPass++;
              else if (st === 'fail') mFail++;
              else if (st === 'skip') mSkip++;
            });
            const rows = moduleCases.map(c =>
              '<tr>' +
              '<td>' + escapeHtml(c.name || '') + '</td>' +
              '<td>' + escapeHtml(c.status || '') + '</td>' +
              '<td>' + (c.duration_seconds || 0).toFixed(3) + 's</td>' +
              '</tr>'
            ).join('');
            return '<details data-test-key="suite:' + suiteIdx + ':mod:' + modIdx + '">' +
              '<summary><code>' + escapeHtml(mod) + '</code> - total=' + moduleCases.length + ', passed=' + mPass + ', failed=' + mFail + ', skipped=' + mSkip + '</summary>' +
              '<table style="width:100%;border-collapse:collapse;margin-top:6px;font-size:12px;">' +
              '<thead><tr><th style="text-align:left;border-bottom:1px solid #c4ddd0;">Test</th><th style="text-align:left;border-bottom:1px solid #c4ddd0;">Status</th><th style="text-align:left;border-bottom:1px solid #c4ddd0;">Duration</th></tr></thead>' +
              '<tbody>' + rows + '</tbody></table>' +
              '</details>';
          }).join('');
        return '<div style="margin-top:10px;">' +
          '<div><strong>' + escapeHtml(s.name || 'suite') + '</strong> (' + escapeHtml(s.format || '') + ')</div>' +
          '<div style="font-size:13px;color:#5f6f67;">total=' + (s.total || 0) + ', passed=' + (s.passed || 0) + ', failed=' + (s.failed || 0) + ', skipped=' + (s.skipped || 0) + '</div>' +
          '<div style="margin-top:6px;display:flex;flex-direction:column;gap:6px;">' + moduleHtml + '</div>' +
          '</div>';
      }).join('');
      box.innerHTML = header + suiteHtml;
    }

    async function resolveProjectIDByName(projectName) {
      const name = String(projectName || '').trim();
      if (!name) return '';
      if (!projectIDByNameCache) {
        projectIDByNameCache = Object.create(null);
        try {
          const data = await apiJSON('/api/v1/projects');
          (data.projects || []).forEach(p => {
            const n = String((p && p.name) || '').trim();
            const id = String((p && p.id) || '').trim();
            if (n && id) projectIDByNameCache[n] = id;
          });
        } catch (_) {}
      }
      return String(projectIDByNameCache[name] || '');
    }

    async function renderProjectIcon(projectID, projectName) {
      const icon = document.getElementById('jobProjectIcon');
      if (!icon) return;
      let id = String(projectID || '').trim();
      if (!id) {
        id = await resolveProjectIDByName(projectName);
      }
      if (!id) {
        icon.style.display = 'none';
        return;
      }
      icon.src = '/api/v1/projects/' + encodeURIComponent(id) + '/icon';
      icon.onload = () => { icon.style.display = 'inline-block'; };
      icon.onerror = () => { icon.style.display = 'none'; };
    }

    function classifyLine(rawLine) {
      if (/^\[meta\]/.test(rawLine)) return 'phase-meta';
      if (/^\[checkout\]/.test(rawLine)) return 'phase-checkout';
      if (/^\[run\]/.test(rawLine)) return 'phase-run';
      if (/^[+]{1,2}\s/.test(rawLine)) return 'shell-trace';
      if (/^[+]{1,2}\s*(git push|gh release create|gh release upload)\b/.test(rawLine)) return 'shell-trace risky-cmd';
      return '';
    }

    function highlightTextTokens(rawText) {
      let out = escapeHtml(rawText);
      out = out.replace(/\b(v\d+\.\d+\.\d+)\b/g, '<span class="tok-version">$1</span>');
      out = out.replace(/\b([0-9a-fA-F]{7,40})\b/g, '<span class="tok-sha">$1</span>');
      out = out.replace(/\bduration=([0-9]+(?:\.[0-9]+)?s)\b/g, 'duration=<span class="tok-duration">$1</span>');
      return out;
    }

    function highlightInline(rawLine) {
      const src = String(rawLine || '');
      const urlRE = /https:\/\/[^\s"']+/g;
      let out = '';
      let last = 0;
      let match;
      while ((match = urlRE.exec(src)) !== null) {
        out += highlightTextTokens(src.slice(last, match.index));
        out += '<span class="tok-url">' + escapeHtml(match[0]) + '</span>';
        last = match.index + match[0].length;
      }
      out += highlightTextTokens(src.slice(last));
      return out;
    }

    function renderDryRunSkippedBlock(lines) {
      const cleaned = lines.filter(l => String(l || '').trim() !== '');
      if (!cleaned.length) return '';
      const head = '<div class="log-dryskip-head">[dry-run] skipped step</div>';
      const body = '<div class="log-dryskip-body">' + cleaned.map(highlightInline).join('\n') + '</div>';
      return '<div class="log-dryskip">' + head + body + '</div>';
    }

    function renderDetachedHeadFold(lines) {
      const text = lines.join('\n');
      return '<details class="log-fold"><summary>git detached HEAD advice (collapsed)</summary><pre>' + escapeHtml(text) + '</pre></details>';
    }

    function renderOutputLog(raw) {
      const text = String(raw || '');
      if (!text) return '<span class="log-empty">&lt;no output yet&gt;</span>';
      const lines = text.replace(/\r\n/g, '\n').replace(/\r/g, '\n').split('\n');
      const html = [];
      for (let i = 0; i < lines.length; i++) {
        const line = lines[i];
        if (/^\[dry-run\]\s+skipped step:/.test(line)) {
          const skipped = [line.replace(/^\[dry-run\]\s+skipped step:\s*/, '')];
          for (let j = i + 1; j < lines.length; j++) {
            const next = lines[j];
            if (/^\[(meta|checkout|run|dry-run)\]/.test(next) || /^[+]{1,2}\s/.test(next)) {
              i = j - 1;
              break;
            }
            if (j === lines.length - 1) i = j;
            if (next.trim() === '') {
              i = j;
              break;
            }
            skipped.push(next);
          }
          html.push(renderDryRunSkippedBlock(skipped));
          continue;
        }

        if (line.indexOf("You are in 'detached HEAD' state.") === 0) {
          const folded = [line];
          for (let j = i + 1; j < lines.length; j++) {
            const next = lines[j];
            folded.push(next);
            if (next.indexOf("Turn off this advice by setting config variable advice.detachedHead to false") === 0) {
              i = j;
              break;
            }
            if (j === lines.length - 1) i = j;
          }
          html.push(renderDetachedHeadFold(folded));
          continue;
        }

        const cls = classifyLine(line);
        const classAttr = cls ? ' class="log-line ' + cls + '"' : ' class="log-line"';
        html.push('<div' + classAttr + '>' + highlightInline(line) + '</div>');
      }
      return html.join('');
    }

    function isNearLogBottom() {
      const el = document.getElementById('logBox');
      if (!el) return true;
      const leewayPx = 48;
      return (el.scrollTop + el.clientHeight) >= (el.scrollHeight - leewayPx);
    }

    function scrollLogToBottom() {
      const el = document.getElementById('logBox');
      if (!el) return;
      suppressLogScrollEvent = true;
      el.scrollTop = el.scrollHeight;
      setTimeout(() => { suppressLogScrollEvent = false; }, 0);
    }

    function setTailingEnabled(enabled) {
      tailingEnabled = !!enabled;
      const btn = document.getElementById('tailToggleBtn');
      if (!btn) return;
      btn.textContent = tailingEnabled ? 'Tailing: On' : 'Tailing: Off';
      btn.classList.toggle('tail-on', tailingEnabled);
      btn.classList.toggle('tail-off', !tailingEnabled);
    }

    function wireLogControls() {
      const logBox = document.getElementById('logBox');
      if (logBox && !logBox.__ciwiTailingBound) {
        logBox.__ciwiTailingBound = true;
        logBox.addEventListener('scroll', () => {
          if (suppressLogScrollEvent) return;
          if (isNearLogBottom()) {
            setTailingEnabled(true);
          } else {
            setTailingEnabled(false);
          }
        });
      }

      const tailBtn = document.getElementById('tailToggleBtn');
      if (tailBtn && !tailBtn.__ciwiBound) {
        tailBtn.__ciwiBound = true;
        tailBtn.addEventListener('click', () => {
          setTailingEnabled(!tailingEnabled);
          if (tailingEnabled) {
            scrollLogToBottom();
          }
        });
      }

      const copyBtn = document.getElementById('copyOutputBtn');
      if (copyBtn && !copyBtn.__ciwiBound) {
        copyBtn.__ciwiBound = true;
        copyBtn.addEventListener('click', async () => {
          const text = String(lastOutputRaw || '');
          const old = copyBtn.textContent;
          try {
            await navigator.clipboard.writeText(text);
            copyBtn.textContent = 'Copied';
          } catch (_) {
            copyBtn.textContent = 'Copy failed';
          }
          setTimeout(() => { copyBtn.textContent = old; }, 1200);
        });
      }
      setTailingEnabled(tailingEnabled);
    }

    async function loadJobExecution(force) {
      if (refreshInFlight || (!force && refreshGuard.shouldPause())) {
        return;
      }
      refreshInFlight = true;
      const jobId = jobExecutionIdFromPath();
      if (!jobId) {
        document.getElementById('subtitle').textContent = 'Missing job id';
        refreshInFlight = false;
        return;
      }

      try {
        const res = await fetch('/api/v1/jobs/' + encodeURIComponent(jobId), { cache: 'no-store' });
        if (!res.ok) {
          document.getElementById('subtitle').textContent = 'Failed to load job';
          return;
        }
        const data = await res.json();
        const job = data.job_execution || {};

        const desc = jobDescription(job);
        const metaSource = (job && job.metadata) || {};
        const projectName = String(metaSource.project || '').trim();
        const projectID = String(metaSource.project_id || '').trim();
        const pipelineJobID = String(metaSource.pipeline_job_id || '').trim();
        const matrixName = String(metaSource.matrix_name || '').trim();
        let title = pipelineJobID || desc;
        if (pipelineJobID && matrixName) {
          title = pipelineJobID + ' / ' + matrixName;
        }
        if (projectName) {
          title = projectName + ' / ' + title;
        }
        document.getElementById('jobTitle').textContent = title;
        await renderProjectIcon(projectID, projectName);

        const pipeline = String(metaSource.pipeline_id || '').trim();
        const dryRun = String(metaSource.dry_run || '').trim() === '1';
        const buildVersion = buildVersionLabel(job);
        const rows = [
          { label: 'Job Execution ID', value: escapeHtml(job.id || '') },
          { label: 'Project', value: escapeHtml(projectName) },
          { label: 'Job ID', value: escapeHtml(pipelineJobID) },
          { label: 'Pipeline', value: escapeHtml(pipeline) },
          { label: 'Mode', valueHTML: renderModeValue(dryRun) },
          { label: 'Build', value: escapeHtml(buildVersion) },
          { label: 'Agent', value: escapeHtml(job.leased_by_agent_id || '') },
          { label: 'Created', value: escapeHtml(formatTimestamp(job.created_utc)) },
          { label: 'Started', value: escapeHtml(formatTimestamp(job.started_utc)) },
          { label: 'Duration', value: escapeHtml(formatJobExecutionDuration(job.started_utc, job.finished_utc, job.status)) },
          { label: 'Exit Code', value: (job.exit_code === null || job.exit_code === undefined) ? '' : String(job.exit_code) },
        ];

        const meta = document.getElementById('metaGrid');
        const previousModeInfo = meta.querySelector('.mode-info');
        const previousModeTooltip = previousModeInfo && previousModeInfo.__ciwiHoverTooltip;
        const modeIconHovered = !!(previousModeInfo && previousModeInfo.matches(':hover'));
        const modeTooltipVisible = !!(previousModeTooltip && typeof previousModeTooltip.isVisible === 'function' && previousModeTooltip.isVisible());
        const modeTooltipHovered = !!document.querySelector('.ciwi-hover-tooltip[data-ciwi-tooltip-owner="mode-info"]:hover');
        const holdMetaRefresh = modeIconHovered || modeTooltipVisible || modeTooltipHovered;
        if (!holdMetaRefresh) {
          meta.querySelectorAll('.mode-info').forEach(el => {
            if (el.__ciwiHoverTooltip && typeof el.__ciwiHoverTooltip.destroy === 'function') {
              el.__ciwiHoverTooltip.destroy();
            }
          });
          meta.innerHTML = rows.map(r =>
            '<div class="label">' + r.label + '</div><div' + (r.valueId ? ' id="' + r.valueId + '"' : '') + '>' + (r.valueHTML || r.value || '') + '</div>'
          ).join('');
          const modeInfo = meta.querySelector('.mode-info');
          if (modeInfo) {
            const mode = String(modeInfo.getAttribute('data-mode') || '').trim();
            const tooltipHTML = mode === 'dry'
              ? 'Dry run executes the job plan but skips steps marked <code>skip_dry_run</code>. This is useful to avoid pushing tags, commits, and artifacts to repositories. Ciwi does not automagically detect such writes, so make sure your ciwi YAML files use <code>skip_dry_run</code> where needed. See <a href="https://github.com/izzyreal/ciwi/blob/main/ciwi-project.yaml" target="_blank" rel="noopener">ciwi\'s own YAML</a> for example usage.'
              : 'Ordinary run executes all configured steps, including side-effecting steps such as publish/release.';
            createHoverTooltip(modeInfo, { html: tooltipHTML, lingerMs: 2000, owner: 'mode-info' });
          }
        }

        const output = (job.error ? ('ERR: ' + job.error + '\n') : '') + (job.output || '');
        lastOutputRaw = output;
        if (output !== lastRenderedOutput) {
          document.getElementById('logBox').innerHTML = renderOutputLog(output);
          lastRenderedOutput = output;
          if (tailingEnabled) {
            requestAnimationFrame(scrollLogToBottom);
          }
        }
        const stepDescription = String(job.current_step || '').trim();
        let subtitle = 'Status: <span class="' + statusClass(job.status) + '">' + escapeHtml(formatJobStatus(job)) + '</span>';
        if (stepDescription) {
          subtitle += ' <span class="label"> - ' + escapeHtml(stepDescription) + '</span>';
        }
        document.getElementById('subtitle').innerHTML = subtitle;

      const forceBtn = document.getElementById('forceFailBtn');
      const active = isActiveJobStatus(job.status);
      if (active) {
        forceBtn.style.display = 'inline-block';
        forceBtn.disabled = false;
        forceBtn.onclick = async () => {
          const confirmed = await showConfirmDialog({
            title: 'Cancel Job',
            message: 'Cancel this active job?',
            okLabel: 'Cancel job',
          });
          if (!confirmed) return;
          forceBtn.disabled = true;
          try {
            const fres = await fetch('/api/v1/jobs/' + encodeURIComponent(jobId) + '/cancel', {
              method: 'POST',
              headers: { 'Content-Type': 'application/json' },
              body: '{}'
            });
            if (!fres.ok) {
              throw new Error(await fres.text() || ('HTTP ' + fres.status));
            }
            await loadJobExecution(true);
          } catch (e) {
            await showAlertDialog({ title: 'Cancel failed', message: 'Cancel failed: ' + e.message });
          } finally {
            forceBtn.disabled = false;
          }
        };
      } else {
        forceBtn.style.display = 'none';
      }

      const rerunBtn = document.getElementById('rerunBtn');
      const hasStarted = !!String(job.started_utc || '').trim();
      rerunBtn.disabled = !hasStarted;
      rerunBtn.title = hasStarted ? '' : 'Job must have started at least once';
      const rerunInfo = document.getElementById('rerunInfo');
      if (rerunInfo && !rerunInfo.__ciwiHoverTooltip) {
        const tooltipHTML = '' +
          '<strong>What Run Job Again does</strong><br />' +
          'It enqueues a new job execution with the same script, environment, requirements, source repo/ref, and step plan as this run.<br /><br />' +
          '<strong>Source checkout behavior</strong><br />' +
          'If this job was already pinned to a commit (for example via pipeline version resolution), rerun uses that same pinned commit. ' +
          'If source ref is a moving branch/tag name, rerun fetches that ref again at execution time and may build a newer commit.<br /><br />' +
          '<strong>Artifacts and logs</strong><br />' +
          'Rerun creates a fresh job execution ID with fresh logs and artifact records. Previous job artifacts are kept; they are not replaced.<br /><br />' +
          '<strong>When this is useful</strong><br />' +
          'Use it to quickly retry flaky failures, rerun after agent/tool fixes, or rerun a one-off job without re-enqueueing an entire pipeline.';
        createHoverTooltip(rerunInfo, { html: tooltipHTML, lingerMs: 2000, owner: 'rerun-info' });
      }
      rerunBtn.onclick = async () => {
        if (rerunBtn.disabled) return;
        rerunBtn.disabled = true;
        const old = rerunBtn.textContent;
        rerunBtn.textContent = 'Queueing...';
        try {
          const res = await fetch('/api/v1/jobs/' + encodeURIComponent(jobId) + '/rerun', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: '{}',
          });
          if (!res.ok) {
            throw new Error(await res.text() || ('HTTP ' + res.status));
          }
          const data = await res.json();
          const enqueuedID = String((((data || {}).job_execution || {}).id) || '').trim();
          const metaSource = job.metadata || {};
          const projectName = String(metaSource.project || '').trim() || 'Project';
          const matrixName = String(metaSource.matrix_name || '').trim();
          const pipelineName = String(metaSource.pipeline_id || '').trim();
          const shortName = matrixName || pipelineName || 'job';
          showJobStartedSnackbar(projectName + ' ' + shortName + ' started', enqueuedID);
        } catch (e) {
          await showAlertDialog({ title: 'Run again failed', message: 'Run again failed: ' + String(e.message || e) });
        } finally {
          rerunBtn.textContent = old;
          rerunBtn.disabled = !hasStarted;
        }
      };

        renderReleaseSummary(job);

      try {
        const ares = await fetch('/api/v1/jobs/' + encodeURIComponent(jobId) + '/artifacts', { cache: 'no-store' });
        if (!ares.ok) {
          throw new Error('artifact request failed');
        }
        const adata = await ares.json();
        const box = document.getElementById('artifactsBox');
        const items = adata.artifacts || [];
        renderArtifacts(box, jobId, items);
      } catch (_) {
        document.getElementById('artifactsBox').textContent = 'Could not load artifacts';
      }

      try {
        const tres = await fetch('/api/v1/jobs/' + encodeURIComponent(jobId) + '/tests', { cache: 'no-store' });
        if (!tres.ok) throw new Error('test report request failed');
        const tdata = await tres.json();
        const report = tdata.report || {};
        const coverage = report.coverage || null;
        const coverageSignature = coverage ? JSON.stringify(coverage) : '';
        if (coverageSignature !== lastCoverageSignature) {
          renderCoverageReport(coverage);
          lastCoverageSignature = coverageSignature;
        }
        const testSignature = JSON.stringify(report);
        if (testSignature !== lastTestReportSignature) {
          renderTestReport(report);
          lastTestReportSignature = testSignature;
        }
      } catch (_) {
        document.getElementById('testReportBox').textContent = 'Could not load test report';
        document.getElementById('coverageReportBox').textContent = 'Could not load coverage report';
        lastCoverageSignature = null;
        lastTestReportSignature = '';
      }
      } finally {
        refreshInFlight = false;
      }
    }

    function renderReleaseSummary(job) {
      const card = document.getElementById('releaseSummaryCard');
      const box = document.getElementById('releaseSummaryBox');
      if (!card || !box) return;

      const m = (job && job.metadata) || {};
      const isReleasePipeline = (m.pipeline_id || '') === 'release';
      if (!isReleasePipeline) {
        card.style.display = 'none';
        box.innerHTML = '';
        return;
      }

      const dryRun = (m.dry_run || '') === '1';
      const versionLabel = String(m.version || m.pipeline_version_raw || '').trim();
      const tagLabel = String(m.tag || m.pipeline_version || '').trim();
      const lines = [];
      lines.push('<div><strong>Mode:</strong> ' + (dryRun ? 'dry-run' : 'live') + '</div>');
      if (versionLabel) lines.push('<div><strong>Version:</strong> ' + escapeHtml(versionLabel) + '</div>');
      if (tagLabel) lines.push('<div><strong>Tag:</strong> ' + escapeHtml(tagLabel) + '</div>');
      if (m.artifacts) lines.push('<div><strong>Assets:</strong> ' + escapeHtml(m.artifacts) + '</div>');
      if (m.next_version) lines.push('<div><strong>Next version:</strong> ' + escapeHtml(m.next_version) + '</div>');
      if (m.auto_bump_branch) lines.push('<div><strong>Auto bump branch:</strong> ' + escapeHtml(m.auto_bump_branch) + '</div>');
      if (lines.length === 1) lines.push('<div class="label">No release metadata reported yet.</div>');

      box.innerHTML = lines.join('');
      card.style.display = '';
    }

    setBackLink();
    wireLogControls();
    refreshGuard.bindSelectionListener();
    loadJobExecution(true);
    setInterval(() => loadJobExecution(false), 500);
  </script>
</body>
</html>`
