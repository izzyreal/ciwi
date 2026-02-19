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
        return jobRowRenderKey(job) + '\x1e' + pipelineRunGroupID(job);
      }).join('\x1f');
    }

    function pipelineRunGroupID(job) {
      const m = (job && job.metadata) || {};
      const runID = String(m.pipeline_run_id || '').trim();
      if (!runID) return '';
      const projectID = String(m.project_id || m.project || '').trim();
      const pipelineID = String(m.pipeline_id || '').trim();
      return runID + '|' + projectID + '|' + pipelineID;
    }

    async function fetchAndRenderJobList(view, totalHint, epoch, tbody, opts, viewKey, columnCount, emptyText, previousSignature, skeletonGroups, progressive) {
      let signature = String(previousSignature || '');
      const total = Math.max(0, Number(totalHint || 0));
      if (total <= 0) {
        if (signature !== '' || !tbodyHasConcreteRows(tbody)) {
          renderGroupedJobs(tbody, [], opts, viewKey, columnCount, emptyText);
          signature = '';
        }
        return signature;
      }
      const out = [];
      const seenJobIDs = new Set();
      for (let offset = 0; offset < JOBS_WINDOW; offset += JOBS_BATCH_SIZE) {
        if (epoch !== jobsRenderEpoch) return null;
        const data = await apiJSON('/api/v1/jobs?view=' + encodeURIComponent(view) +
          '&max=' + String(JOBS_WINDOW) +
          '&offset=' + String(offset) +
          '&limit=' + String(JOBS_BATCH_SIZE));
        if (epoch !== jobsRenderEpoch) return null;
        const jobs = Array.isArray(data.job_executions) ? data.job_executions : [];
        if (jobs.length === 0) break;
        jobs.forEach(job => {
          const jobID = String((job && job.id) || '').trim();
          if (jobID && seenJobIDs.has(jobID)) return;
          if (jobID) seenJobIDs.add(jobID);
          out.push(job);
        });
        const nextSignature = jobsSignature(out);
        if (progressive && (!tbodyHasConcreteRows(tbody) || nextSignature !== signature)) {
          const groups = collectOrderedJobGroups(out);
          if (!tbodyHasConcreteRows(tbody)) {
            tbody.innerHTML = '';
          }
          const renderedGroups = countConcreteGroupRows(tbody);
          appendProgressiveGroups(tbody, groups, renderedGroups, opts, viewKey, columnCount);
          removeSkeletonRows(tbody);
          appendGroupedSkeletonTail(tbody, skeletonGroups, groups.length, viewKey, columnCount);
          signature = nextSignature;
        }
        const sourceTotal = Math.max(0, Number(data.total || 0));
        if (jobs.length < JOBS_BATCH_SIZE) break;
        if (sourceTotal > 0 && offset+JOBS_BATCH_SIZE >= sourceTotal) break;
      }
      if (epoch !== jobsRenderEpoch) return null;
      const finalSignature = jobsSignature(out);
      // Progressive mode can render a group before all its jobs are loaded
      // (for example first 5 rows of a 6-row history). Always do one final
      // full render at the end to avoid preserving partial grouped rows.
      if (progressive || !tbodyHasConcreteRows(tbody) || finalSignature !== signature) {
        renderGroupedJobs(tbody, out, opts, viewKey, columnCount, emptyText);
      }
      signature = finalSignature;
      return signature;
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
      const iconURLFn = (opts && typeof opts.projectIconURL === 'function') ? opts.projectIconURL : null;
      const iconURL = iconURLFn ? String(iconURLFn(job) || '').trim() : '';
      const iconHTML = iconURL
        ? '<img class="ciwi-job-group-side-icon" src="' + escapeHtml(iconURL) + '" alt="" onerror="this.style.display=&quot;none&quot;" />'
        : '';
      head.innerHTML = '<span class="ciwi-job-group-main">' + iconHTML + '<span class="ciwi-job-group-emoji" aria-hidden="true">' + status.emoji +
        '</span><span class="ciwi-job-group-title">' + escapeHtml(groupTitle) +
        '</span></span><span class="ciwi-job-group-status ' + status.cls + '">' + escapeHtml(status.text) + '</span>';
      card.appendChild(head);

      const table = document.createElement('table');
      table.className = 'ciwi-job-group-table';
      const body = document.createElement('tbody');
      const rowOpts = { ...(opts || {}), projectIconURL: null };
      body.appendChild(buildJobExecutionRow(job, rowOpts));
      table.appendChild(body);
      card.appendChild(table);
      td.appendChild(card);
      tr.appendChild(td);
      return tr;
    }

    function collectOrderedJobGroups(jobs) {
      const out = [];
      if (!Array.isArray(jobs) || jobs.length === 0) return out;
      const jobsByRun = new Map();
      jobs.forEach(job => {
        const runID = pipelineRunGroupID(job);
        if (!runID) return;
        if (!jobsByRun.has(runID)) jobsByRun.set(runID, []);
        jobsByRun.get(runID).push(job);
      });

      const consumed = new Set();
      jobs.forEach(job => {
        const jobID = String(job.id || '');
        if (consumed.has(jobID)) return;

        const runID = pipelineRunGroupID(job);
        const runJobs = runID ? jobsByRun.get(runID) : null;
        if (!runID || !runJobs || runJobs.length <= 1) {
          consumed.add(jobID);
          out.push({
            kind: 'single',
            key: 'single:' + jobID,
            jobs: [job],
          });
          return;
        }

        runJobs.forEach(j => consumed.add(String(j.id || '')));
        out.push({
          kind: 'run',
          key: 'run:' + runID,
          runID: runID,
          jobs: runJobs,
        });
      });
      return out;
    }

    function buildRunGroupRow(group, opts, viewKey, columnCount) {
      const runJobs = group.jobs || [];
      const groupTitle = jobGroupLabel(runJobs);
      const status = summarizeJobGroup(runJobs);
      const groupKey = viewKey + ':' + String(group.runID || '').trim();

      const tr = document.createElement('tr');
      tr.className = 'ciwi-job-group-row';
      tr.dataset.ciwiGroupKey = String(group.key || '');
      const td = document.createElement('td');
      td.colSpan = columnCount;
      const details = document.createElement('details');
      details.className = 'ciwi-job-group-details';
      if (expandedJobGroups.has(groupKey)) {
        details.open = true;
      }
      const iconURLFn = (opts && typeof opts.projectIconURL === 'function') ? opts.projectIconURL : null;
      const iconURL = (iconURLFn && runJobs.length > 0) ? String(iconURLFn(runJobs[0]) || '').trim() : '';
      const iconHTML = iconURL
        ? '<img class="ciwi-job-group-side-icon" src="' + escapeHtml(iconURL) + '" alt="" onerror="this.style.display=&quot;none&quot;" />'
        : '';
      const summary = document.createElement('summary');
      summary.innerHTML = '<span class="ciwi-job-group-main">' + iconHTML + '<span class="ciwi-job-group-emoji" aria-hidden="true">' + status.emoji +
        '</span><span class="ciwi-job-group-title">' + escapeHtml(groupTitle) +
        '</span></span><span class="ciwi-job-group-status ' + status.cls + '">' + escapeHtml(status.text) + '</span><span class="ciwi-job-group-toggle" aria-hidden="true"></span>';
      details.appendChild(summary);

      const innerTable = document.createElement('table');
      innerTable.className = 'ciwi-job-group-table';
      const innerBody = document.createElement('tbody');
      const rowOpts = { ...(opts || {}), projectIconURL: null };
      runJobs.forEach(j => {
        innerBody.appendChild(buildJobExecutionRow(j, rowOpts));
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
      return tr;
    }

    function buildJobGroupRow(group, opts, viewKey, columnCount) {
      if (!group || !Array.isArray(group.jobs) || group.jobs.length === 0) return null;
      if (group.kind === 'run' && group.jobs.length > 1) {
        return buildRunGroupRow(group, opts, viewKey, columnCount);
      }
      const row = buildStaticJobGroupRow(group.jobs[0], opts, columnCount);
      row.dataset.ciwiGroupKey = String(group.key || '');
      return row;
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
      const groups = collectOrderedJobGroups(jobs);
      groups.forEach(group => {
        const row = buildJobGroupRow(group, opts, viewKey, columnCount);
        if (row) tbody.appendChild(row);
      });
    }

    function countRenderedGroups(jobs) {
      return collectOrderedJobGroups(jobs).length;
    }

    function appendGroupedSkeletonTail(tbody, groups, renderedGroupCount, viewKey, columnCount) {
      const specs = Array.isArray(groups) ? groups : [];
      if (specs.length === 0) return;
      const start = Math.max(0, Math.min(specs.length, Number(renderedGroupCount || 0)));
      for (let i = start; i < specs.length; i += 1) {
        tbody.appendChild(buildJobGroupSkeletonRow(specs[i], viewKey, columnCount));
      }
    }

    function removeSkeletonRows(tbody) {
      const rows = Array.from(tbody.querySelectorAll('.ciwi-job-skeleton-row'));
      rows.forEach(row => row.remove());
    }

    function countConcreteGroupRows(tbody) {
      if (!tbody) return 0;
      const rows = Array.from(tbody.children || []);
      return rows.filter(row => row && row.classList && row.classList.contains('ciwi-job-group-row') && !row.classList.contains('ciwi-job-skeleton-row')).length;
    }

    function appendProgressiveGroups(tbody, groups, renderedGroupCount, opts, viewKey, columnCount) {
      const total = Array.isArray(groups) ? groups.length : 0;
      const start = Math.max(0, Math.min(total, Number(renderedGroupCount || 0)));
      for (let i = start; i < total; i += 1) {
        const row = buildJobGroupRow(groups[i], opts, viewKey, columnCount);
        if (row) tbody.appendChild(row);
      }
      return total;
    }

    async function refreshJobs() {
      const epoch = ++jobsRenderEpoch;
      const queuedBody = document.getElementById('queuedJobsBody');
      const historyBody = document.getElementById('historyJobsBody');
      const queuedHadConcreteRows = tbodyHasConcreteRows(queuedBody);
      const historyHadConcreteRows = tbodyHasConcreteRows(historyBody);
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
        includeDuration: true,
        fixedLines: 2,
        backPath: window.location.pathname || '/',
        linkClass: 'job-link',
        projectIconURL: projectIconURLForJob
      };

      const [queuedSig, historySig] = await Promise.all([
        fetchAndRenderJobList('queued', queuedTotal, epoch, queuedBody, queuedOpts, 'queued', 8, 'No queued jobs.', lastQueuedJobsSignature, queuedGroups, !queuedHadConcreteRows),
        fetchAndRenderJobList('history', historyTotal, epoch, historyBody, historyOpts, 'history', 7, 'No job history yet.', lastHistoryJobsSignature, historyGroups, !historyHadConcreteRows),
      ]);
      if (epoch !== jobsRenderEpoch || queuedSig === null || historySig === null) return;
      if (!tbodyHasConcreteRows(queuedBody) || queuedSig !== lastQueuedJobsSignature) {
        lastQueuedJobsSignature = queuedSig;
      }
      if (!tbodyHasConcreteRows(historyBody) || historySig !== lastHistoryJobsSignature) {
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
