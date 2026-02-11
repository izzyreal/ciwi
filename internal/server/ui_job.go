package server

const jobHTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="UTF-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>ciwi job execution</title>
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
    :where(body, main, .card, p, h3, div, span, table, thead, tbody, tr, th, td, code, pre, input, textarea, a) {
      -webkit-user-select: text;
      user-select: text;
    }
    :where(button) {
      -webkit-user-select: none;
      user-select: none;
    }
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
    .meta-grid { display:grid; grid-template-columns: 160px 1fr; gap:8px 12px; font-size:14px; }
    .label { color: var(--muted); }
    .status-succeeded { color: var(--ok); font-weight: 700; }
    .status-failed { color: var(--bad); font-weight: 700; }
    .status-running { color: #a56a00; font-weight: 700; }
    .status-queued, .status-leased { color: var(--muted); font-weight: 700; }
    textarea.log {
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
      resize: vertical;
      font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", "Courier New", monospace;
    }
    a { color: var(--accent); text-decoration:none; }
    a:hover { text-decoration:underline; }
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
      border: 1px solid var(--line);
      background: white;
      color: var(--accent);
      border-radius: 6px;
      padding: 2px 8px;
      font-size: 12px;
      cursor: pointer;
    }
  </style>
</head>
<body>
  <main>
    <div class="card top">
      <div class="brand">
        <img src="/ciwi-logo.png" alt="ciwi logo" />
        <div>
          <div style="font-size:20px;font-weight:700;" id="jobTitle">Job Execution</div>
          <div style="color:#5f6f67;" id="subtitle">Loading...</div>
        </div>
      </div>
      <div><a id="backLink" href="/">Back to Job Executions</a></div>
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
      <textarea id="logBox" class="log" readonly spellcheck="false"></textarea>
    </div>

    <div class="card">
      <h3 style="margin:0 0 10px;">Artifacts</h3>
      <div id="artifactsBox" style="font-size:14px;color:#5f6f67;">Loading...</div>
    </div>
    <div class="card">
      <h3 style="margin:0 0 10px;">Test Report</h3>
      <div id="testReportBox" style="font-size:14px;color:#5f6f67;">Loading...</div>
    </div>
  </main>

  <script src="/ui/shared.js"></script>
  <script>

    function jobIdFromPath() {
      const parts = window.location.pathname.split('/').filter(Boolean);
      return parts.length >= 2 ? decodeURIComponent(parts[1]) : '';
    }

    function setBackLink() {
      const link = document.getElementById('backLink');
      if (!link) return;
      const params = new URLSearchParams(window.location.search || '');
      const back = params.get('back') || '';
      if (back && back.startsWith('/')) {
        link.href = back;
        link.textContent = back.startsWith('/projects/') ? 'Back to Project' : 'Back to Job Executions';
        return;
      }
      link.href = '/';
      link.textContent = 'Back to Job Executions';
    }

    async function loadJob() {
      const jobId = jobIdFromPath();
      if (!jobId) {
        document.getElementById('subtitle').textContent = 'Missing job id';
        return;
      }

      const res = await fetch('/api/v1/jobs/' + encodeURIComponent(jobId));
      if (!res.ok) {
        document.getElementById('subtitle').textContent = 'Failed to load job';
        return;
      }
      const data = await res.json();
      const job = data.job_execution || data.job;

      const desc = jobDescription(job);
      document.getElementById('jobTitle').textContent = desc;
      document.getElementById('subtitle').innerHTML = 'Status: <span class="' + statusClass(job.status) + '">' + escapeHtml(formatJobStatus(job)) + '</span>';

      const pipeline = (job.metadata && job.metadata.pipeline_id) || '';
      const buildVersion = buildVersionLabel(job);
      const rows = [
        ['Job Execution ID', escapeHtml(job.id || '')],
        ['Pipeline', escapeHtml(pipeline)],
        ['Build', escapeHtml(buildVersion)],
        ['Agent', escapeHtml(job.leased_by_agent_id || '')],
        ['Created', escapeHtml(formatTimestamp(job.created_utc))],
        ['Started', escapeHtml(formatTimestamp(job.started_utc))],
        ['Duration', escapeHtml(formatDuration(job.started_utc, job.finished_utc, job.status))],
        ['Exit Code', (job.exit_code === null || job.exit_code === undefined) ? '' : String(job.exit_code)],
      ];

      const meta = document.getElementById('metaGrid');
      meta.innerHTML = rows.map(r => '<div class="label">' + r[0] + '</div><div>' + r[1] + '</div>').join('');

      const output = (job.error ? ('ERR: ' + job.error + '\n') : '') + (job.output || '');
      document.getElementById('logBox').value = output || '<no output yet>';

      renderReleaseSummary(job, output);

      try {
        const ares = await fetch('/api/v1/jobs/' + encodeURIComponent(jobId) + '/artifacts');
        if (!ares.ok) {
          throw new Error('artifact request failed');
        }
        const adata = await ares.json();
        const box = document.getElementById('artifactsBox');
        const items = adata.artifacts || [];
        if (items.length === 0) {
          box.textContent = 'No artifacts';
        } else {
          box.innerHTML = items.map((a, idx) =>
            '<div class="artifact-row">' +
              '<span class="artifact-path">' + escapeHtml(a.path) + '</span>' +
              '<span>(' + formatBytes(a.size_bytes) + ')</span>' +
              '<a href=\"' + a.url + '\" target=\"_blank\" rel=\"noopener\">Download</a>' +
              '<button class="copy-btn" data-artifact-index="' + String(idx) + '">Copy</button>' +
            '</div>'
          ).join('');
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
        }
      } catch (_) {
        document.getElementById('artifactsBox').textContent = 'Could not load artifacts';
      }

      try {
        const tres = await fetch('/api/v1/jobs/' + encodeURIComponent(jobId) + '/tests');
        if (!tres.ok) throw new Error('test report request failed');
        const tdata = await tres.json();
        const report = tdata.report || {};
        const box = document.getElementById('testReportBox');
        const suites = report.suites || [];
        if (!suites.length) {
          box.textContent = 'No parsed test report';
        } else {
          const header = '<div><strong>Total:</strong> ' + (report.total || 0) +
            ' | <strong>Passed:</strong> ' + (report.passed || 0) +
            ' | <strong>Failed:</strong> ' + (report.failed || 0) +
            ' | <strong>Skipped:</strong> ' + (report.skipped || 0) + '</div>';
          const suiteHtml = suites.map(s => {
            const cases = (s.cases || []).map(c =>
              '<tr>' +
              '<td>' + escapeHtml(c.package || '') + '</td>' +
              '<td>' + escapeHtml(c.name || '') + '</td>' +
              '<td>' + escapeHtml(c.status || '') + '</td>' +
              '<td>' + (c.duration_seconds || 0).toFixed(3) + 's</td>' +
              '</tr>'
            ).join('');
            return '<div style="margin-top:10px;">' +
              '<div><strong>' + escapeHtml(s.name || 'suite') + '</strong> (' + escapeHtml(s.format || '') + ')</div>' +
              '<div style="font-size:13px;color:#5f6f67;">total=' + (s.total || 0) + ', passed=' + (s.passed || 0) + ', failed=' + (s.failed || 0) + ', skipped=' + (s.skipped || 0) + '</div>' +
              '<table style="width:100%;border-collapse:collapse;margin-top:6px;font-size:12px;">' +
              '<thead><tr><th style="text-align:left;border-bottom:1px solid #c4ddd0;">Package</th><th style="text-align:left;border-bottom:1px solid #c4ddd0;">Test</th><th style="text-align:left;border-bottom:1px solid #c4ddd0;">Status</th><th style="text-align:left;border-bottom:1px solid #c4ddd0;">Duration</th></tr></thead>' +
              '<tbody>' + cases + '</tbody></table></div>';
          }).join('');
          box.innerHTML = header + suiteHtml;
        }
      } catch (_) {
        document.getElementById('testReportBox').textContent = 'Could not load test report';
      }
    }

    function parseReleaseSummary(output) {
      const out = {};
      (output || '').split('\n').forEach(line => {
        const m = line.match(/^__CIWI_RELEASE_SUMMARY__\s+([a-zA-Z0-9_]+)=(.*)$/);
        if (!m) return;
        out[m[1]] = m[2];
      });
      return out;
    }

    function renderReleaseSummary(job, output) {
      const card = document.getElementById('releaseSummaryCard');
      const box = document.getElementById('releaseSummaryBox');
      if (!card || !box) return;

      const m = (job && job.metadata) || {};
      const isReleasePipeline = (m.pipeline_id || '') === 'release';
      const parsed = parseReleaseSummary(output);
      if (!isReleasePipeline && Object.keys(parsed).length === 0) {
        card.style.display = 'none';
        box.innerHTML = '';
        return;
      }

      const dryRun = (m.dry_run || '') === '1';
      const lines = [];
      lines.push('<div><strong>Mode:</strong> ' + (dryRun ? 'dry-run' : 'live') + '</div>');
      if (parsed.version) lines.push('<div><strong>Version:</strong> ' + escapeHtml(parsed.version) + '</div>');
      if (parsed.tag) lines.push('<div><strong>Tag:</strong> ' + escapeHtml(parsed.tag) + '</div>');
      if (parsed.release_created) lines.push('<div><strong>GitHub release:</strong> ' + escapeHtml(parsed.release_created) + '</div>');
      if (parsed.artifacts) lines.push('<div><strong>Assets:</strong> ' + escapeHtml(parsed.artifacts) + '</div>');
      if (parsed.next_version) lines.push('<div><strong>Next version:</strong> ' + escapeHtml(parsed.next_version) + '</div>');
      if (lines.length === 1) lines.push('<div class="label">No release markers emitted yet.</div>');

      box.innerHTML = lines.join('');
      card.style.display = '';
    }

    setBackLink();
    loadJob();
    setInterval(loadJob, 500);
  </script>
</body>
</html>`
