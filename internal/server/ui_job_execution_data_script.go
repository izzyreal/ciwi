package server

const jobExecutionDataJS = `
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
    let logSearchController = null;
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
      if (!logSearchController) {
        logSearchController = createTextSearchController({
          scopeEl: document.getElementById('logBox'),
          inputEl: document.getElementById('logSearchInput'),
          prevBtn: document.getElementById('logSearchPrevBtn'),
          nextBtn: document.getElementById('logSearchNextBtn'),
          countEl: document.getElementById('logSearchCount'),
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
        renderCacheStats(job.cache_stats);
        renderToolRequirements(job.required_capabilities, job.runtime_capabilities, job.status);

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
          if (logSearchController && typeof logSearchController.refresh === 'function') {
            logSearchController.refresh();
          }
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
    setBackLink();
    wireLogControls();
    refreshGuard.bindSelectionListener();
    loadJobExecution(true);
    setInterval(() => loadJobExecution(false), 500);

`
