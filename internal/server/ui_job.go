package server

const jobHTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="UTF-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>ciwi job</title>
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
  </style>
</head>
<body>
  <main>
    <div class="card top">
      <div>
        <div style="font-size:20px;font-weight:700;" id="jobTitle">Job</div>
        <div style="color:#5f6f67;" id="subtitle">Loading...</div>
      </div>
      <div><a href="/">Back to Queue</a></div>
    </div>

    <div class="card">
      <div class="meta-grid" id="metaGrid"></div>
    </div>

    <div class="card">
      <h3 style="margin:0 0 10px;">Output / Error</h3>
      <textarea id="logBox" class="log" readonly spellcheck="false"></textarea>
    </div>

    <div class="card">
      <h3 style="margin:0 0 10px;">Artifacts</h3>
      <div id="artifactsBox" style="font-size:14px;color:#5f6f67;">Loading...</div>
    </div>
  </main>

  <script src="/ui/shared.js"></script>
  <script>

    function jobIdFromPath() {
      const parts = window.location.pathname.split('/').filter(Boolean);
      return parts.length >= 2 ? decodeURIComponent(parts[1]) : '';
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
      const job = data.job;

      const desc = jobDescription(job);
      document.getElementById('jobTitle').textContent = desc;
      document.getElementById('subtitle').innerHTML = 'Status: <span class="' + statusClass(job.status) + '">' + escapeHtml(job.status || '') + '</span>';

      const pipeline = (job.metadata && job.metadata.pipeline_id) || '';
      const rows = [
        ['Job ID', escapeHtml(job.id || '')],
        ['Status', '<span class="' + statusClass(job.status) + '">' + escapeHtml(job.status || '') + '</span>'],
        ['Pipeline', escapeHtml(pipeline)],
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
          box.innerHTML = items.map(a =>
            '<div><a href=\"' + a.url + '\" target=\"_blank\" rel=\"noopener\">' + escapeHtml(a.path) + '</a> (' + a.size_bytes + ' bytes)</div>'
          ).join('');
        }
      } catch (_) {
        document.getElementById('artifactsBox').textContent = 'Could not load artifacts';
      }
    }

    loadJob();
    setInterval(loadJob, 2000);
  </script>
</body>
</html>`
