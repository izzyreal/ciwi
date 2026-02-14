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
      <div style="display:flex;gap:8px;align-items:center;flex-wrap:wrap;">
        <button id="forceFailBtn" class="copy-btn" style="display:none;">Force Fail</button>
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
    let refreshInFlight = false;
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

        const pipeline = String(metaSource.pipeline_id || '').trim();
        const buildVersion = buildVersionLabel(job);
        const rows = [
          { label: 'Job Execution ID', value: escapeHtml(job.id || '') },
          { label: 'Project', value: escapeHtml(projectName) },
          { label: 'Job ID', value: escapeHtml(pipelineJobID) },
          { label: 'Pipeline', value: escapeHtml(pipeline) },
          { label: 'Build', value: escapeHtml(buildVersion) },
          { label: 'Agent', value: escapeHtml(job.leased_by_agent_id || '') },
          { label: 'Created', value: escapeHtml(formatTimestamp(job.created_utc)) },
          { label: 'Started', value: escapeHtml(formatTimestamp(job.started_utc)) },
          { label: 'Duration', value: escapeHtml(formatJobExecutionDuration(job.started_utc, job.finished_utc, job.status)) },
          { label: 'Exit Code', value: (job.exit_code === null || job.exit_code === undefined) ? '' : String(job.exit_code) },
        ];

        const meta = document.getElementById('metaGrid');
        meta.innerHTML = rows.map(r =>
          '<div class="label">' + r.label + '</div><div' + (r.valueId ? ' id="' + r.valueId + '"' : '') + '>' + r.value + '</div>'
        ).join('');

        const output = (job.error ? ('ERR: ' + job.error + '\n') : '') + (job.output || '');
        document.getElementById('logBox').value = output || '<no output yet>';
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
          if (!confirm('Force-fail this active job?')) return;
          forceBtn.disabled = true;
          try {
            const fres = await fetch('/api/v1/jobs/' + encodeURIComponent(jobId) + '/force-fail', {
              method: 'POST',
              headers: { 'Content-Type': 'application/json' },
              body: '{}'
            });
            if (!fres.ok) {
              throw new Error(await fres.text() || ('HTTP ' + fres.status));
            }
            await loadJobExecution(true);
          } catch (e) {
            alert('Force fail failed: ' + e.message);
          } finally {
            forceBtn.disabled = false;
          }
        };
      } else {
        forceBtn.style.display = 'none';
      }

        renderReleaseSummary(job);

      try {
        const ares = await fetch('/api/v1/jobs/' + encodeURIComponent(jobId) + '/artifacts', { cache: 'no-store' });
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
        const tres = await fetch('/api/v1/jobs/' + encodeURIComponent(jobId) + '/tests', { cache: 'no-store' });
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
      if (m.release_created) lines.push('<div><strong>GitHub release:</strong> ' + escapeHtml(m.release_created) + '</div>');
      if (m.artifacts) lines.push('<div><strong>Assets:</strong> ' + escapeHtml(m.artifacts) + '</div>');
      if (m.next_version) lines.push('<div><strong>Next version:</strong> ' + escapeHtml(m.next_version) + '</div>');
      if (m.auto_bump_branch) lines.push('<div><strong>Auto bump branch:</strong> ' + escapeHtml(m.auto_bump_branch) + '</div>');
      if (lines.length === 1) lines.push('<div class="label">No release metadata reported yet.</div>');

      box.innerHTML = lines.join('');
      card.style.display = '';
    }

    setBackLink();
    refreshGuard.bindSelectionListener();
    loadJobExecution(true);
    setInterval(() => loadJobExecution(false), 500);
  </script>
</body>
</html>`
