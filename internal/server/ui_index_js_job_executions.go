package server

const uiIndexJobExecutionsJS = `
    function normalizeSummaryGroups(rawGroups, fallbackCount) {
      if (Array.isArray(rawGroups) && rawGroups.length > 0) {
        return rawGroups.map((g, idx) => {
          const runID = String((g && g.run_id) || '').trim();
          const key = String((g && g.key) || (runID ? ('run:' + runID) : ('idx:' + idx))).trim();
          const jobCount = Math.max(1, Number((g && g.job_count) || 1) || 1);
          const collapsible = !!((g && g.collapsible) || (runID && jobCount > 1));
          return { key: key, run_id: runID, job_count: jobCount, collapsible: collapsible };
        });
      }
      const count = Math.max(0, Number(fallbackCount || 0));
      const out = [];
      for (let i = 0; i < count; i += 1) {
        out.push({ key: 'fallback:' + i, run_id: '', job_count: 1, collapsible: false });
      }
      return out;
    }

    function buildJobGroupSkeletonRow(spec, viewKey, columnCount) {
      ensureJobSkeletonStyles();
      const tr = document.createElement('tr');
      tr.className = 'ciwi-job-group-row ciwi-job-skeleton-row';
      const td = document.createElement('td');
      td.colSpan = columnCount;

      const isCollapsible = !!spec.collapsible;
      const runID = String(spec.run_id || '').trim();
      const groupKey = runID ? (viewKey + ':' + runID) : '';
      const expanded = isCollapsible && groupKey !== '' && expandedJobGroups.has(groupKey);
      const root = document.createElement('div');
      root.className = isCollapsible ? 'ciwi-job-group-details' : 'ciwi-job-group-card';
      const head = document.createElement('div');
      head.className = isCollapsible ? 'ciwi-job-group-skel-head' : 'ciwi-job-group-head';
      head.innerHTML =
        '<span class="ciwi-job-group-main">' +
          (isCollapsible ? '<span class="ciwi-job-group-toggle" aria-hidden="true">' + (expanded ? '▾' : '▸') + '</span>' : '') +
          '<span class="ciwi-job-group-emoji" aria-hidden="true">⏳</span>' +
          '<span class="ciwi-job-group-title"><span class="ciwi-job-skeleton-bar" style="width:180px;display:inline-block;"></span></span>' +
        '</span>' +
        '<span class="ciwi-job-group-status status-queued"><span class="ciwi-job-skeleton-bar ciwi-job-skeleton-bar-short" style="width:110px;display:inline-block;"></span></span>';
      root.appendChild(head);

      const showBody = !isCollapsible || expanded;
      if (showBody) {
        const body = document.createElement('div');
        body.className = 'ciwi-job-group-skel-body';
        const rows = Math.max(1, Number(spec.job_count || 1) || 1);
        for (let i = 0; i < rows; i += 1) {
          const line = document.createElement('div');
          line.className = 'ciwi-job-skeleton-lines';
          line.innerHTML = '<div class="ciwi-job-skeleton-bar"></div><div class="ciwi-job-skeleton-bar ciwi-job-skeleton-bar-short"></div>';
          body.appendChild(line);
        }
        root.appendChild(body);
      }

      td.appendChild(root);
      tr.appendChild(td);
      return tr;
    }

    function allocateGroupedSkeletonRows(tbody, groups, columnCount, emptyText, viewKey) {
      const specs = Array.isArray(groups) ? groups : [];
      if (specs.length === 0) {
        const existing = tbody.querySelector('.ciwi-empty-row');
        if (existing && tbody.children.length === 1) return;
        tbody.innerHTML = '';
        const tr = document.createElement('tr');
        tr.className = 'ciwi-empty-row';
        tr.innerHTML = '<td colspan="' + String(columnCount) + '" class="muted">' + escapeHtml(emptyText) + '</td>';
        tbody.appendChild(tr);
        return;
      }
      tbody.innerHTML = '';
      specs.forEach(spec => {
        tbody.appendChild(buildJobGroupSkeletonRow(spec, viewKey, columnCount));
      });
    }

    function tbodyHasConcreteRows(tbody) {
      if (!tbody) return false;
      const rows = Array.from(tbody.children || []);
      if (rows.length === 0) return false;
      return rows.some(row => {
        if (!row || !row.classList) return true;
        return !row.classList.contains('ciwi-job-skeleton-row') && !row.classList.contains('ciwi-empty-row');
      });
    }

    function jobsSignature(jobs) {
      if (!Array.isArray(jobs) || jobs.length === 0) return '';
      return jobs.map(job => {
        const runID = String((job && job.metadata && job.metadata.pipeline_run_id) || '').trim();
        return jobRowRenderKey(job) + '\x1e' + runID;
      }).join('\x1f');
    }

    async function fetchJobList(view, total, epoch) {
      if (total <= 0) return [];
      const out = [];
      for (let offset = 0; offset < total; offset += JOBS_BATCH_SIZE) {
        if (epoch !== jobsRenderEpoch) return null;
        const data = await apiJSON('/api/v1/jobs?view=' + encodeURIComponent(view) +
          '&max=' + String(JOBS_WINDOW) +
          '&offset=' + String(offset) +
          '&limit=' + String(JOBS_BATCH_SIZE));
        if (epoch !== jobsRenderEpoch) return null;
        const jobs = data.job_executions || [];
        out.push(...jobs);
      }
      return out;
    }

    function summarizeJobGroup(jobs) {
      const total = jobs.length;
      let succeeded = 0;
      let failed = 0;
      let inProgress = 0;
      jobs.forEach(job => {
        const status = normalizedJobStatus(job.status);
        if (isSucceededJobStatus(status)) succeeded += 1;
        else if (isFailedJobStatus(status)) failed += 1;
        else if (isActiveJobStatus(status)) inProgress += 1;
      });
      if (failed > 0) {
        return { emoji: '❌', cls: 'status-failed', text: succeeded + '/' + total + ' successful, ' + failed + ' failed' };
      }
      if (inProgress > 0) {
        return { emoji: '⏳', cls: 'status-running', text: succeeded + '/' + total + ' successful, ' + inProgress + ' in progress' };
      }
      if (succeeded === total) {
        return { emoji: '✅', cls: 'status-succeeded', text: succeeded + '/' + total + ' successful' };
      }
      return { emoji: '🟡', cls: 'status-queued', text: succeeded + '/' + total + ' successful' };
    }

    function jobGroupLabel(jobs) {
      if (!jobs || jobs.length === 0) return 'pipeline run';
      const first = jobs[0] || {};
      const meta = first.metadata || {};
      const projectName = String(meta.project || '').trim();
      const pipelineID = String(meta.pipeline_id || '').trim();
      let buildVersion = '';
      let earliest = '';
      jobs.forEach(job => {
        if (!buildVersion) {
          const version = String((job && job.metadata && job.metadata.build_version) || '').trim();
          if (version) buildVersion = version;
        }
        const ts = String(job.created_utc || '').trim();
        if (!ts) return;
        if (!earliest || ts < earliest) {
          earliest = ts;
        }
      });
      const when = earliest ? formatTimestamp(earliest) : '';
      const parts = [];
      if (projectName) parts.push(projectName);
      if (pipelineID) parts.push(pipelineID);
      const base = parts.join(' ');
      const titledParts = [];
      if (base) titledParts.push(base);
      if (when) titledParts.push(when);
      if (buildVersion) titledParts.push(buildVersion);
      if (titledParts.length > 0) return titledParts.join(' ');
      return 'job';
    }

    function buildStaticJobGroupRow(job, opts, columnCount) {
      const status = summarizeJobGroup([job]);
      const groupTitle = jobGroupLabel([job]);
      const tr = document.createElement('tr');
      tr.className = 'ciwi-job-group-row';
      const td = document.createElement('td');
      td.colSpan = columnCount;

      const card = document.createElement('div');
      card.className = 'ciwi-job-group-card';
      const head = document.createElement('div');
      head.className = 'ciwi-job-group-head';
      head.innerHTML = '<span class="ciwi-job-group-main"><span class="ciwi-job-group-emoji" aria-hidden="true">' + status.emoji +
        '</span><span class="ciwi-job-group-title">' + escapeHtml(groupTitle) +
        '</span></span><span class="ciwi-job-group-status ' + status.cls + '">' + escapeHtml(status.text) + '</span>';
      card.appendChild(head);

      const table = document.createElement('table');
      table.className = 'ciwi-job-group-table';
      const body = document.createElement('tbody');
      body.appendChild(buildJobExecutionRow(job, opts));
      table.appendChild(body);
      card.appendChild(table);

      td.appendChild(card);
      tr.appendChild(td);
      return tr;
    }

    function renderGroupedJobs(tbody, jobs, opts, viewKey, columnCount, emptyText) {
      tbody.innerHTML = '';
      if (!jobs || jobs.length === 0) {
        const tr = document.createElement('tr');
        tr.className = 'ciwi-empty-row';
        tr.innerHTML = '<td colspan="' + String(columnCount) + '" class="muted">' + escapeHtml(emptyText) + '</td>';
        tbody.appendChild(tr);
        return;
      }
      const jobsByRun = new Map();
      jobs.forEach(job => {
        const runID = String((job.metadata && job.metadata.pipeline_run_id) || '').trim();
        if (!runID) return;
        if (!jobsByRun.has(runID)) jobsByRun.set(runID, []);
        jobsByRun.get(runID).push(job);
      });

      const consumed = new Set();
      jobs.forEach(job => {
        const jobID = String(job.id || '');
        if (consumed.has(jobID)) return;

        const runID = String((job.metadata && job.metadata.pipeline_run_id) || '').trim();
        const runJobs = runID ? jobsByRun.get(runID) : null;
        if (!runID || !runJobs || runJobs.length <= 1) {
          consumed.add(jobID);
          tbody.appendChild(buildStaticJobGroupRow(job, opts, columnCount));
          return;
        }

        runJobs.forEach(j => consumed.add(String(j.id || '')));
        const groupTitle = jobGroupLabel(runJobs);
        const status = summarizeJobGroup(runJobs);
        const groupKey = viewKey + ':' + runID;

        const tr = document.createElement('tr');
        tr.className = 'ciwi-job-group-row';
        const td = document.createElement('td');
        td.colSpan = columnCount;
        const details = document.createElement('details');
        details.className = 'ciwi-job-group-details';
        if (expandedJobGroups.has(groupKey)) {
          details.open = true;
        }
        const summary = document.createElement('summary');
        summary.innerHTML = '<span class="ciwi-job-group-main"><span class="ciwi-job-group-emoji" aria-hidden="true">' + status.emoji +
          '</span><span class="ciwi-job-group-title">' + escapeHtml(groupTitle) +
          '</span></span><span class="ciwi-job-group-status ' + status.cls + '">' + escapeHtml(status.text) + '</span><span class="ciwi-job-group-toggle" aria-hidden="true"></span>';
        details.appendChild(summary);

        const innerTable = document.createElement('table');
        innerTable.className = 'ciwi-job-group-table';
        const innerBody = document.createElement('tbody');
        runJobs.forEach(j => {
          innerBody.appendChild(buildJobExecutionRow(j, opts));
        });
        innerTable.appendChild(innerBody);
        details.appendChild(innerTable);
        details.addEventListener('toggle', () => {
          if (details.open) {
            expandedJobGroups.add(groupKey);
          } else {
            expandedJobGroups.delete(groupKey);
          }
          saveStringSet(JOB_GROUPS_STORAGE_KEY, expandedJobGroups);
        });

        td.appendChild(details);
        tr.appendChild(td);
        tbody.appendChild(tr);
      });
    }

    async function refreshJobs() {
      const epoch = ++jobsRenderEpoch;
      const queuedBody = document.getElementById('queuedJobsBody');
      const historyBody = document.getElementById('historyJobsBody');
      const summary = await apiJSON('/api/v1/jobs?view=summary&max=' + String(JOBS_WINDOW));
      if (epoch !== jobsRenderEpoch) return;

      const queuedTotal = Number(summary.queued_count || 0);
      const historyTotal = Number(summary.history_count || 0);
      const queuedGroupTotal = Number(summary.queued_group_count || queuedTotal);
      const historyGroupTotal = Number(summary.history_group_count || historyTotal);
      const queuedGroups = normalizeSummaryGroups(summary.queued_groups, queuedGroupTotal);
      const historyGroups = normalizeSummaryGroups(summary.history_groups, historyGroupTotal);
      if (!tbodyHasConcreteRows(queuedBody)) {
        allocateGroupedSkeletonRows(queuedBody, queuedGroups, 8, 'No queued jobs.', 'queued');
      }
      if (!tbodyHasConcreteRows(historyBody)) {
        allocateGroupedSkeletonRows(historyBody, historyGroups, 7, 'No job history yet.', 'history');
      }

      const queuedOpts = {
        includeActions: true,
        includeReason: true,
        fixedLines: 2,
        backPath: window.location.pathname || '/',
        linkClass: 'job-link',
        projectIconURL: projectIconURLForJob,
        onRemove: async (j) => {
          try {
            await apiJSON('/api/v1/jobs/' + j.id, { method: 'DELETE' });
            await refreshJobs();
          } catch (e) {
            alert('Remove failed: ' + e.message);
          }
        }
      };
      const historyOpts = {
        includeActions: false,
        includeReason: true,
        fixedLines: 2,
        backPath: window.location.pathname || '/',
        linkClass: 'job-link',
        projectIconURL: projectIconURLForJob
      };

      const [queuedJobs, historyJobs] = await Promise.all([
        fetchJobList('queued', queuedTotal, epoch),
        fetchJobList('history', historyTotal, epoch),
      ]);
      if (epoch !== jobsRenderEpoch || queuedJobs === null || historyJobs === null) return;
      const queuedSig = jobsSignature(queuedJobs);
      const historySig = jobsSignature(historyJobs);
      const queuedNeedsRender = !tbodyHasConcreteRows(queuedBody) || queuedSig !== lastQueuedJobsSignature;
      const historyNeedsRender = !tbodyHasConcreteRows(historyBody) || historySig !== lastHistoryJobsSignature;
      if (queuedNeedsRender) {
        renderGroupedJobs(queuedBody, queuedJobs, queuedOpts, 'queued', 8, 'No queued jobs.');
        lastQueuedJobsSignature = queuedSig;
      }
      if (historyNeedsRender) {
        renderGroupedJobs(historyBody, historyJobs, historyOpts, 'history', 7, 'No job history yet.');
        lastHistoryJobsSignature = historySig;
      }
    }

    document.getElementById('clearQueueBtn').onclick = async () => {
      if (!confirm('Clear all queued/leased jobs?')) {
        return;
      }
      try {
        await apiJSON('/api/v1/jobs/clear-queue', { method: 'POST', body: '{}' });
        await refreshJobs();
      } catch (e) {
        alert('Clear queue failed: ' + e.message);
      }
    };

    document.getElementById('flushHistoryBtn').onclick = async () => {
      if (!confirm('Flush all finished jobs from history?')) {
        return;
      }
      try {
        await apiJSON('/api/v1/jobs/flush-history', { method: 'POST', body: '{}' });
        await refreshJobs();
      } catch (e) {
        alert('Flush history failed: ' + e.message);
      }
    };
`
