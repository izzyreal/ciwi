package server

const jobExecutionDataJS = `
    let refreshInFlight = false;
    let lastRenderedOutput = null;
    let lastOutputRaw = '';
    let lastStructuredEvents = [];
    let lastEventID = 0;
    let supplementalLoaded = false;
    let continuePolling = true;
    let pollTimer = null;
    let terminalSyncPasses = 0;
    let logStepOpenState = Object.create(null);
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

    function activeStepIndexFromCurrentStep(currentStep) {
      const text = String(currentStep || '').trim();
      if (!text) return -1;
      const m = text.match(/^Step\s+(\d+)(?:\/\d+)?\s*:/i);
      if (!m) return -1;
      const idx = Number.parseInt(String(m[1] || '').trim(), 10);
      if (!Number.isFinite(idx) || idx <= 0) return -1;
      return idx - 1;
    }

    function subtitleStepDetail(job) {
      const stepPlan = Array.isArray(job && job.step_plan) ? job.step_plan : [];
      const idx = activeStepIndexFromCurrentStep(job && job.current_step);
      if (idx < 0 || idx >= stepPlan.length) return '';
      const step = stepPlan[idx] || {};
      const script = String(step.script || '').trim();
      if (script) return script.replace(/\s+/g, ' ');
      const kind = String(step.kind || '').trim();
      const testName = String(step.test_name || '').trim();
      if (kind === 'test' && testName) return 'test ' + testName;
      if (kind === 'dryrun_skip') return 'skipped during dry run';
      return '';
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
            if (navigator.clipboard && typeof navigator.clipboard.writeText === 'function') {
              await navigator.clipboard.writeText(text);
            } else {
              throw new Error('navigator.clipboard unavailable');
            }
            copyBtn.textContent = 'Copied';
          } catch (primaryErr) {
            try {
              const ta = document.createElement('textarea');
              ta.value = text;
              ta.setAttribute('readonly', '');
              ta.style.position = 'fixed';
              ta.style.left = '-9999px';
              ta.style.top = '0';
              document.body.appendChild(ta);
              ta.focus();
              ta.select();
              const ok = document.execCommand && document.execCommand('copy');
              document.body.removeChild(ta);
              if (!ok) throw new Error('execCommand copy returned false');
              copyBtn.textContent = 'Copied';
            } catch (fallbackErr) {
              console.warn('Copy output failed', primaryErr, fallbackErr);
              copyBtn.textContent = 'Copy failed';
            }
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
      bindLogInfoTooltips();
      setTailingEnabled(tailingEnabled);
    }

    function bindLogInfoTooltips() {
      document.querySelectorAll('.log-info[data-log-info]').forEach(el => {
        if (el.__ciwiHoverTooltip) return;
        const kind = String(el.getAttribute('data-log-info') || '').trim();
        const tooltipHTML = kind === 'raw'
          ? '<strong>Raw log</strong><br />Downloads the redacted structured event stream with ANSI escape sequences preserved.'
          : '<strong>Clean log</strong><br />Downloads an editor-friendly plain text log generated from structured events. ANSI escape sequences and terminal control characters are stripped.';
        createHoverTooltip(el, { html: tooltipHTML, lingerMs: 2000, owner: 'log-info-' + kind });
      });
    }

    function bindLogStepToggles() {
      const logBox = document.getElementById('logBox');
      if (!logBox) return;
      logBox.querySelectorAll('details.log-step[data-step-key]').forEach(d => {
        const key = String(d.getAttribute('data-step-key') || '').trim();
        if (!key || d.__ciwiStepToggleBound) return;
        d.__ciwiStepToggleBound = true;
        d.addEventListener('toggle', () => {
          logStepOpenState[key] = !!d.open;
        });
      });
    }

    async function loadJobExecution(force) {
      if (refreshInFlight || (!force && refreshGuard.shouldPause())) {
        scheduleJobExecutionRefresh(1000);
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
        bindCiwiProgress(document.getElementById('jobHeaderCard'), job);
        let newEvents = [];
        let eventRefreshSucceeded = false;
        try {
          const eventsURL = '/api/v1/jobs/' + encodeURIComponent(jobId) + '/events?after_id=' + encodeURIComponent(String(lastEventID));
          const evRes = await fetch(eventsURL, { cache: 'no-store' });
          if (evRes.ok) {
            const evData = await evRes.json();
            newEvents = Array.isArray(evData.events) ? evData.events : [];
            if (newEvents.length) {
              lastStructuredEvents = lastStructuredEvents.concat(newEvents);
            }
            const nextEventID = Number(evData.next_event_id || 0);
            if (Number.isFinite(nextEventID) && nextEventID >= lastEventID) {
              lastEventID = nextEventID;
            }
            eventRefreshSucceeded = true;
          }
        } catch (_) {}
        const events = lastStructuredEvents;
        const cleanBtn = document.getElementById('downloadCleanLogBtn');
        const rawBtn = document.getElementById('downloadRawLogBtn');
        if (cleanBtn) cleanBtn.href = '/api/v1/jobs/' + encodeURIComponent(jobId) + '/log?format=clean';
        if (rawBtn) rawBtn.href = '/api/v1/jobs/' + encodeURIComponent(jobId) + '/log?format=raw';

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
        renderToolRequirements(job.required_capabilities, job.runtime_capabilities, job.status, job.unmet_requirements);

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

        const eventSignature = JSON.stringify(events.map(ev => [ev.id, ev.type, ev.timestamp_utc, ev.step && ev.step.index, ev.message, ev.output, ev.error, ev.duration_ms]));
        const hasStructured = hasStructuredLogEvents(events);
        const renderSignature = 'structured:' + eventSignature + ':' + String(job.current_step || '') + ':' + String(job.status || '');
        lastOutputRaw = plainTextFromStructuredEvents(job, events);
        if (renderSignature !== lastRenderedOutput) {
          document.getElementById('logBox').innerHTML = renderStructuredOutputLog(job, events);
          bindLogStepToggles();
          if (hasStructured) bindStructuredStepProgress(job, events);
          lastRenderedOutput = renderSignature;
          if (logSearchController && typeof logSearchController.refresh === 'function') {
            logSearchController.refresh();
          }
          if (tailingEnabled) {
            requestAnimationFrame(scrollLogToBottom);
          }
        }
        const stepDescription = String(job.current_step || '').trim();
        let subtitle = 'Status: <span class="' + statusClassForJob(job) + '">' + escapeHtml(formatJobStatus(job)) + '</span>';
        const waitingReason = jobWaitingReason(job);
        if (waitingReason) {
          subtitle += '<div class="job-subtitle-detail">' + escapeHtml(waitingReason) + '</div>';
        }
        if (stepDescription) {
          subtitle += ' <span class="label"> - ' + escapeHtml(stepDescription) + '</span>';
        }
        const stepDetail = subtitleStepDetail(job);
        if (stepDetail) {
          subtitle += '<div class="job-subtitle-detail">Command: <code>' + escapeHtml(stepDetail) + '</code></div>';
        }
        document.getElementById('subtitle').innerHTML = subtitle;

      const forceBtn = document.getElementById('forceFailBtn');
      const active = isActiveJobStatus(job.status);
      if (active) terminalSyncPasses = 0;
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
      const rerunBlockedLink = document.getElementById('rerunBlockedLink');
      const hasStarted = !!String(job.started_utc || '').trim();
      rerunBtn.disabled = !hasStarted;
      rerunBtn.title = hasStarted ? '' : 'Job must have started at least once';
      if (rerunBlockedLink) {
        rerunBlockedLink.style.display = 'none';
        rerunBlockedLink.removeAttribute('href');
        rerunBlockedLink.textContent = 'Open failed dependency';
      }
      if (!hasStarted && isDependencyBlockedJob(job) && rerunBlockedLink) {
		rerunBtn.disabled = false;
		rerunBtn.title = '';
        try {
          const bres = await fetch('/api/v1/jobs/' + encodeURIComponent(jobId) + '/blocked-by', { cache: 'no-store' });
          if (bres.ok) {
            const bdata = await bres.json();
            const dep = (bdata && bdata.dependency) || null;
            const depID = String((dep && dep.job_execution_id) || '').trim();
            if (depID) {
              const backTo = encodeURIComponent(window.location.pathname + window.location.search);
              rerunBlockedLink.href = '/jobs/' + encodeURIComponent(depID) + '?back=' + backTo;
              rerunBlockedLink.style.display = 'inline';
              const depJob = String((dep && dep.pipeline_job_id) || '').trim();
              const depMatrix = String((dep && dep.matrix_name) || '').trim();
              let label = depJob;
              if (depMatrix) {
                label = depJob ? (depJob + ' / ' + depMatrix) : depMatrix;
              }
              if (label) {
                rerunBlockedLink.textContent = 'Open failed dependency: ' + label;
              }
            }
          }
        } catch (_) {}
      }
      const rerunInfo = document.getElementById('rerunInfo');
      if (rerunInfo && !rerunInfo.__ciwiHoverTooltip) {
        const tooltipHTML = '' +
          '<strong>What Run Job Again does</strong><br />' +
          'It enqueues a new attempt with the same script, requirements, source repo/ref, and step plan as this run. Pipeline and chain jobs remain part of their original run and refresh their upstream artifact bindings, allowing failed runs to be repaired in place.<br /><br />' +
          '<strong>Source checkout behavior</strong><br />' +
          'Rerun keeps the same pinned source commit as the original queued job.<br /><br />' +
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

      const shouldLoadSupplemental = !supplementalLoaded || !active;
      let supplementalSucceeded = true;
      if (shouldLoadSupplemental) try {
        const ares = await fetch('/api/v1/jobs/' + encodeURIComponent(jobId) + '/artifacts', { cache: 'no-store' });
        if (!ares.ok) {
          throw new Error('artifact request failed');
        }
        const adata = await ares.json();
        const box = document.getElementById('artifactsBox');
        const items = adata.artifacts || [];
        renderArtifacts(box, jobId, items);
      } catch (_) {
        supplementalSucceeded = false;
        document.getElementById('artifactsBox').textContent = 'Could not load artifacts';
      }

      if (shouldLoadSupplemental) try {
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
          renderTestReport(report, job);
          lastTestReportSignature = testSignature;
        }
      } catch (_) {
        supplementalSucceeded = false;
        document.getElementById('testReportBox').textContent = 'Could not load test report';
        document.getElementById('coverageReportBox').textContent = 'Could not load coverage report';
        lastCoverageSignature = null;
        lastTestReportSignature = '';
      }
      if (shouldLoadSupplemental && supplementalSucceeded) supplementalLoaded = true;
      if (active) {
        continuePolling = true;
      } else if (eventRefreshSucceeded && supplementalSucceeded) {
        terminalSyncPasses += 1;
        continuePolling = terminalSyncPasses < 2;
      } else {
        continuePolling = true;
      }
      } finally {
        refreshInFlight = false;
        if (continuePolling) scheduleJobExecutionRefresh(1000);
      }
    }

    function scheduleJobExecutionRefresh(delayMs) {
      if (!continuePolling || pollTimer !== null) return;
      pollTimer = setTimeout(() => {
        pollTimer = null;
        loadJobExecution(false);
      }, delayMs);
    }
    setBackLink();
    wireLogControls();
    refreshGuard.bindSelectionListener();
    loadJobExecution(true);

`
