
    let refreshInFlight = false;
    const refreshGuard = createRefreshGuard(5000);
    let jobsRenderEpoch = 0;
    let lastProjectsSignature = '';
    let lastQueuedJobsSignature = '';
    let lastHistoryJobsSignature = '';
    let lastQueuedLayoutSignature = '';
    let lastHistoryLayoutSignature = '';
    const queueCardDetailsByKey = Object.create(null);
    const historyCardDetailsByKey = Object.create(null);
    const PROJECT_GROUPS_STORAGE_KEY = 'ciwi.index.projectGroupsCollapsed.v1';
    const JOB_GROUPS_STORAGE_KEY = 'ciwi.index.jobGroupsExpanded.v1';

    function loadStringSet(key) {
      try {
        const raw = localStorage.getItem(key);
        if (!raw) return new Set();
        const parsed = JSON.parse(raw);
        if (!Array.isArray(parsed)) return new Set();
        return new Set(parsed.map(v => String(v || '')));
      } catch (_) {
        return new Set();
      }
    }

    function saveStringSet(key, values) {
      try {
        localStorage.setItem(key, JSON.stringify(Array.from(values || [])));
      } catch (_) {}
    }

    const projectGroupCollapsed = loadStringSet(PROJECT_GROUPS_STORAGE_KEY);
    const expandedJobGroups = loadStringSet(JOB_GROUPS_STORAGE_KEY);
    const JOBS_WINDOW = 150;
    const JOBS_BATCH_SIZE = 5;
    const HISTORY_CARD_WINDOW = 40;
    const HISTORY_CARD_BATCH = HISTORY_CARD_WINDOW;
    function projectIconURLForJob(job) {
      const m = (job && job.metadata) || {};
      const projectID = String(m.project_id || '').trim();
      if (!projectID) return '';
      return '/api/v1/projects/' + encodeURIComponent(projectID) + '/icon';
    }

    function projectListSignature(projects) {
      const list = Array.isArray(projects) ? projects : [];
      return JSON.stringify(list.map(project => ({
        id: project.id,
        name: project.name,
        source_kind: project.source_kind || 'vcs',
        repo_url: project.repo_url,
        repo_ref: project.repo_ref || '',
        config_file: project.config_file || project.config_path || '',
        pipelines: (project.pipelines || []).map(p => ({
          id: p.id,
          pipeline_id: p.pipeline_id,
          trigger: p.trigger || '',
          depends_on: p.depends_on || [],
          source_repo: p.source_repo || '',
          supports_dry_run: !!p.supports_dry_run,
        })),
        pipeline_chains: (project.pipeline_chains || []).map(c => ({
          id: c.id,
          name: c.name,
          pipelines: c.pipelines || [],
          supports_dry_run: !!c.supports_dry_run,
          version_pipeline_id: c.version_pipeline_id || 0,
        })),
      })));
    }

    async function refreshProjects() {
      const data = await apiJSON('/api/v1/projects');
      const root = document.getElementById('projects');
      if (!data.projects || data.projects.length === 0) {
        lastProjectsSignature = '';
        root.innerHTML = '<p>No projects loaded yet.</p>';
        return;
      }
      const signature = projectListSignature(data.projects);
      if (signature === lastProjectsSignature) {
        return;
      }
      lastProjectsSignature = signature;
      root.innerHTML = '';
      data.projects.forEach(project => {
        const projectIconURL = '/api/v1/projects/' + encodeURIComponent(project.id) + '/icon';
        const projectKey = String(project.id || project.name || '');
        const details = document.createElement('details');
        details.className = 'project-group';
        details.open = !projectGroupCollapsed.has(projectKey);
        const summary = document.createElement('summary');
        const top = document.createElement('div');
        top.className = 'project-head';
        const topInfo = document.createElement('div');
        topInfo.innerHTML = '<strong>Project: <a class="job-link" href="/projects/' + project.id + '">' + escapeHtml(project.name) + '</a></strong> ' + projectSourceMetadataHTML(project);
        const topRight = document.createElement('div');
        topRight.innerHTML = '<span class="pill">' + String((project.pipelines || []).length) + ' pipeline(s)</span>';
        top.appendChild(topInfo);
        top.appendChild(topRight);
        summary.appendChild(top);
        const toggle = document.createElement('span');
        toggle.className = 'project-group-toggle';
        toggle.setAttribute('aria-hidden', 'true');
        toggle.appendChild(ciwiIconElement('chevron-right'));
        summary.appendChild(toggle);
        details.appendChild(summary);

        const body = document.createElement('div');
        body.className = 'project-body';
        const layout = document.createElement('div');
        layout.className = 'project-body-layout';
        const iconCol = document.createElement('div');
        iconCol.className = 'project-icon-col';
        const icon = document.createElement('img');
        icon.className = 'project-icon';
        icon.alt = '';
        icon.src = projectIconURL;
        icon.onerror = () => { icon.style.display = 'none'; };
        iconCol.appendChild(icon);
        const listCol = document.createElement('div');
        listCol.className = 'project-pipelines-col';
        (project.pipelines || []).forEach(p => {
          const row = document.createElement('div');
          row.className = 'pipeline';
          const deps = (p.depends_on || []).join(', ');
          const info = document.createElement('div');
          info.innerHTML = '<div><span class="muted">Pipeline:</span> <code>' + p.pipeline_id + '</code></div><div class="muted">' +
            (p.source_repo || '') + (deps ? (' | depends_on: ' + deps) : '') + '</div>';

          const btn = document.createElement('button');
          btn.className = 'secondary';
          btn.textContent = 'Run';
          btn.onclick = async (ev) => {
            btn.disabled = true;
            try {
              const runResult = await runWithOptionalSourceRef(ev, {
                runPath: '/api/v1/pipelines/' + p.id + '/run-selection',
                sourceRefsPath: '/api/v1/pipelines/' + p.id + '/source-refs',
                eligibleAgentsPath: '/api/v1/pipelines/' + p.id + '/eligible-agents',
                payload: {},
                title: 'Run Pipeline With Source Ref',
                subtitle: String(p.pipeline_id || ''),
                runLabel: 'Run',
              });
              if (runResult.cancelled) return;
              showQueuedJobsSnackbar((project.name || 'Project') + ' ' + (p.pipeline_id || 'pipeline') + ' started');
              await refreshJobs();
            } catch (e) {
              await showAlertDialog({ title: 'Run failed', message: 'Run failed: ' + e.message });
            } finally {
              btn.disabled = false;
            }
          };

          const supportsDryRun = !!p.supports_dry_run;
          const dryBtn = document.createElement('button');
          dryBtn.className = 'secondary';
          dryBtn.textContent = 'Dry Run';
          dryBtn.onclick = async (ev) => {
            dryBtn.disabled = true;
            try {
              const runResult = await runWithOptionalSourceRef(ev, {
                runPath: '/api/v1/pipelines/' + p.id + '/run-selection',
                sourceRefsPath: '/api/v1/pipelines/' + p.id + '/source-refs',
                eligibleAgentsPath: '/api/v1/pipelines/' + p.id + '/eligible-agents',
                payload: { dry_run: true },
                title: 'Dry Run Pipeline With Source Ref',
                subtitle: String(p.pipeline_id || ''),
                runLabel: 'Dry Run',
              });
              if (runResult.cancelled) return;
              showQueuedJobsSnackbar((project.name || 'Project') + ' ' + (p.pipeline_id || 'pipeline') + ' started');
              await refreshJobs();
            } catch (e) {
              await showAlertDialog({ title: 'Dry run failed', message: 'Dry run failed: ' + e.message });
            } finally {
              dryBtn.disabled = false;
            }
          };

          const resolveBtn = document.createElement('button');
          resolveBtn.className = 'secondary';
          resolveBtn.textContent = 'Resolve Upcoming Build Version';
          resolveBtn.onclick = () => openVersionResolveModal(p.id, p.pipeline_id);
          const previewBtn = document.createElement('button');
          previewBtn.className = 'secondary';
          previewBtn.textContent = 'Execution Plan';
          previewBtn.onclick = () => {
            openDryRunPreviewModal({
              title: 'Execution Plan',
              subtitle: String(p.pipeline_id || ''),
              previewPath: '/api/v1/pipelines/' + p.id + '/dry-run-preview',
              runPath: '/api/v1/pipelines/' + p.id + '/run-selection',
              sourceRefsPath: '/api/v1/pipelines/' + p.id + '/source-refs',
              eligibleAgentsPath: '/api/v1/pipelines/' + p.id + '/eligible-agents',
              payload: { dry_run: true },
            });
          };

          row.appendChild(info);
          const actions = document.createElement('div');
          actions.className = 'pipeline-actions';
          const btnRow = document.createElement('div');
          btnRow.className = 'row';
          btnRow.appendChild(btn);
          if (supportsDryRun) btnRow.appendChild(dryBtn);
          btnRow.appendChild(previewBtn);
          btnRow.appendChild(resolveBtn);
          actions.appendChild(btnRow);
          row.appendChild(actions);
          listCol.appendChild(row);
        });

        (project.pipeline_chains || []).forEach(c => {
          const row = document.createElement('div');
          row.className = 'pipeline';
          const info = document.createElement('div');
          const chainPipes = pipelineChainSequence(c);
          const chainName = pipelineChainDisplayName(c);
          info.innerHTML = '<div><span class="muted">Chain:</span> ' + pipelineChainDisplayHTML(c) + '</div>' +
            (chainName !== chainPipes ? ('<div class="muted">' + pipelineChainSequenceHTML(c) + '</div>') : '');

          const runBtn = document.createElement('button');
          runBtn.className = 'secondary';
          runBtn.textContent = 'Run';
          runBtn.onclick = async (ev) => {
            runBtn.disabled = true;
            try {
              const runResult = await runWithOptionalSourceRef(ev, {
                runPath: pipelineChainAPIPath(project.id, c.id, 'run'),
                sourceRefsPath: pipelineChainAPIPath(project.id, c.id, 'source-refs'),
                eligibleAgentsPath: pipelineChainAPIPath(project.id, c.id, 'eligible-agents'),
                payload: {},
                title: 'Run Chain With Source Ref',
                subtitle: chainName,
                runLabel: 'Run',
              });
              if (runResult.cancelled) return;
              showQueuedJobsSnackbar((project.name || 'Project') + ' ' + chainName + ' started');
              await refreshJobs();
            } catch (e) {
              await showAlertDialog({ title: 'Run failed', message: 'Run failed: ' + e.message });
            } finally {
              runBtn.disabled = false;
            }
          };

          const dryBtn = document.createElement('button');
          dryBtn.className = 'secondary';
          dryBtn.textContent = 'Dry Run';
          dryBtn.onclick = async (ev) => {
            dryBtn.disabled = true;
            try {
              const runResult = await runWithOptionalSourceRef(ev, {
                runPath: pipelineChainAPIPath(project.id, c.id, 'run'),
                sourceRefsPath: pipelineChainAPIPath(project.id, c.id, 'source-refs'),
                eligibleAgentsPath: pipelineChainAPIPath(project.id, c.id, 'eligible-agents'),
                payload: { dry_run: true },
                title: 'Dry Run Chain With Source Ref',
                subtitle: chainName,
                runLabel: 'Dry Run',
              });
              if (runResult.cancelled) return;
              showQueuedJobsSnackbar((project.name || 'Project') + ' ' + chainName + ' started');
              await refreshJobs();
            } catch (e) {
              await showAlertDialog({ title: 'Dry run failed', message: 'Dry run failed: ' + e.message });
            } finally {
              dryBtn.disabled = false;
            }
          };

          const resolveBtn = document.createElement('button');
          resolveBtn.className = 'secondary';
          resolveBtn.textContent = 'Resolve Upcoming Build Version';
          const versionPipelineID = Number(c.version_pipeline_id || 0);
          if (versionPipelineID > 0) {
            resolveBtn.onclick = () => openVersionResolveModal(versionPipelineID, chainName);
          } else {
            resolveBtn.disabled = true;
          }
          const previewBtn = document.createElement('button');
          previewBtn.className = 'secondary';
          previewBtn.textContent = 'Execution Plan';
          previewBtn.onclick = () => {
            openDryRunPreviewModal({
              title: 'Execution Plan',
              subtitle: chainName,
              previewPath: pipelineChainAPIPath(project.id, c.id, 'dry-run-preview'),
              runPath: pipelineChainAPIPath(project.id, c.id, 'run'),
              sourceRefsPath: pipelineChainAPIPath(project.id, c.id, 'source-refs'),
              eligibleAgentsPath: pipelineChainAPIPath(project.id, c.id, 'eligible-agents'),
              payload: { dry_run: true },
            });
          };

          const actions = document.createElement('div');
          actions.className = 'pipeline-actions';
          const btnRow = document.createElement('div');
          btnRow.className = 'row';
          btnRow.appendChild(runBtn);
          if (c.supports_dry_run) btnRow.appendChild(dryBtn);
          btnRow.appendChild(previewBtn);
          btnRow.appendChild(resolveBtn);
          actions.appendChild(btnRow);
          row.appendChild(info);
          row.appendChild(actions);
          listCol.appendChild(row);
        });

        layout.appendChild(iconCol);
        layout.appendChild(listCol);
        body.appendChild(layout);
        details.appendChild(body);
        details.addEventListener('toggle', () => {
          if (details.open) {
            projectGroupCollapsed.delete(projectKey);
          } else {
            projectGroupCollapsed.add(projectKey);
          }
          saveStringSet(PROJECT_GROUPS_STORAGE_KEY, projectGroupCollapsed);
        });
        root.appendChild(details);
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

    function historyCardSummaryStatus(summary) {
      const total = Math.max(0, Number((summary && summary.total_jobs) || 0));
      const succeeded = Math.max(0, Number((summary && summary.succeeded) || 0));
      const failed = Math.max(0, Number((summary && summary.failed) || 0));
      const inProgress = Math.max(0, Number((summary && summary.in_progress) || 0));
      const waiting = Math.max(0, Number((summary && summary.waiting) || 0));
      const parts = [succeeded + '/' + total + ' successful'];
      if (failed > 0) parts.push(failed + ' failed');
      if (inProgress > 0) parts.push(inProgress + ' in progress');
      if (waiting > 0) parts.push(waiting + ' waiting');
      if (failed > 0) {
        return { icon: 'circle-x', cls: 'status-failed', text: parts.join(', ') };
      }
      if (inProgress > 0) {
        return { icon: 'loader-2', iconClass: 'ciwi-icon-spin', cls: 'status-running', text: parts.join(', ') };
      }
      if (waiting > 0) {
        return { icon: 'clock', cls: 'status-waiting', text: parts.join(', ') };
      }
      if (total > 0 && succeeded === total) {
        return { icon: 'circle-check', cls: 'status-succeeded', text: succeeded + '/' + total + ' successful' };
      }
      return { icon: 'clock', cls: 'status-queued', text: succeeded + '/' + total + ' successful' };
    }

    function historyLayoutSignature(cards) {
      const rows = Array.isArray(cards) ? cards : [];
      return rows.map(card => String((card && card.key) || '').trim()).join('\x1f');
    }

    function historyExpandedRowHint(card) {
      const shape = (card && card.shape) || {};
      const expanded = Number((shape && shape.expanded_rows_hint) || (card && card.expanded_rows_hint) || 0);
      return Math.max(1, expanded || 1);
    }

    function historyCardGroupKey(cardKey) {
      return 'history:' + String(cardKey || '').trim();
    }

    function historyCardIsCollapsible(card) {
      const summary = (card && card.summary) || {};
      const total = Math.max(0, Number(summary.total_jobs || 0));
      return total > 0;
    }

    function buildHistorySkeletonBody(rowCount) {
      const body = document.createElement('div');
      body.className = 'ciwi-job-group-skel-body';
      const rows = Math.max(1, Number(rowCount || 1) || 1);
      for (let i = 0; i < rows; i += 1) {
        const line = document.createElement('div');
        line.className = 'ciwi-job-skeleton-lines';
        line.innerHTML = '<div class="ciwi-job-skeleton-bar"></div><div class="ciwi-job-skeleton-bar ciwi-job-skeleton-bar-short"></div>';
        body.appendChild(line);
      }
      return body;
    }

    function buildHistoryCardSkeletonRow(card, columnCount) {
      ensureJobSkeletonStyles();
      const tr = document.createElement('tr');
      tr.className = 'ciwi-job-group-row ciwi-job-skeleton-row';
      tr.dataset.ciwiHistoryCardKey = String((card && card.key) || '').trim();
      const td = document.createElement('td');
      td.colSpan = columnCount;
      const collapsible = historyCardIsCollapsible(card);
      const groupKey = historyCardGroupKey(card && card.key);
      const expanded = collapsible && expandedJobGroups.has(groupKey);
      if (collapsible) {
        const details = document.createElement('details');
        details.className = 'ciwi-job-group-details';
        details.__ciwiHistoryCardKey = String((card && card.key) || '').trim();
        details.__ciwiHistoryCard = historyCardDetailsByKey[details.__ciwiHistoryCardKey] || card;
        details.__ciwiHistoryOpts = null;
        if (expanded) details.open = true;
        const summary = document.createElement('summary');
        summary.className = 'ciwi-job-group-skel-head';
        summary.innerHTML =
          '<span class="ciwi-job-group-main">' +
            '<span class="ciwi-job-group-status-icon" aria-hidden="true">' + ciwiIconHTML('loader-2', { className: 'ciwi-icon-spin' }) + '</span>' +
            '<span class="ciwi-job-group-title"><span class="ciwi-job-skeleton-bar" style="width:180px;display:inline-block;"></span></span>' +
          '</span>' +
          '<span class="ciwi-job-group-status status-queued"><span class="ciwi-job-skeleton-bar ciwi-job-skeleton-bar-short" style="width:110px;display:inline-block;"></span></span>' +
          '<span class="ciwi-job-group-toggle" aria-hidden="true">' + ciwiIconHTML('chevron-right') + '</span>';
        details.appendChild(summary);
        if (expanded) {
          details.appendChild(buildHistorySkeletonBody(historyExpandedRowHint(card)));
        }
        bindHistoryCardToggle(details, card);
        td.appendChild(details);
      } else {
        const cardEl = document.createElement('div');
        cardEl.className = 'ciwi-job-group-card';
        const head = document.createElement('div');
        head.className = 'ciwi-job-group-head';
        head.innerHTML =
          '<span class="ciwi-job-group-main">' +
            '<span class="ciwi-job-group-status-icon" aria-hidden="true">' + ciwiIconHTML('loader-2', { className: 'ciwi-icon-spin' }) + '</span>' +
            '<span class="ciwi-job-group-title"><span class="ciwi-job-skeleton-bar" style="width:180px;display:inline-block;"></span></span>' +
          '</span>' +
          '<span class="ciwi-job-group-status status-queued"><span class="ciwi-job-skeleton-bar ciwi-job-skeleton-bar-short" style="width:110px;display:inline-block;"></span></span>';
        cardEl.appendChild(head);
        td.appendChild(cardEl);
      }
      tr.appendChild(td);
      return tr;
    }

    function renderHistoryLayoutRows(tbody, cards, columnCount) {
      const rows = Array.isArray(cards) ? cards : [];
      if (typeof destroyOverflowTooltips === 'function') {
        destroyOverflowTooltips(tbody);
      }
      tbody.innerHTML = '';
      if (rows.length === 0) {
        const tr = document.createElement('tr');
        tr.className = 'ciwi-empty-row';
        tr.innerHTML = '<td colspan="' + String(columnCount) + '" class="muted">No job history yet.</td>';
        tbody.appendChild(tr);
        return;
      }
      rows.forEach(card => tbody.appendChild(buildHistoryCardSkeletonRow(card, columnCount)));
    }

    function findHistoryCardRow(tbody, key) {
      if (!tbody) return null;
      const target = String(key || '').trim();
      return Array.from(tbody.children || []).find(row => row && row.dataset && row.dataset.ciwiHistoryCardKey === target) || null;
    }

    function buildHistoryCardHeadHTML(card, opts) {
      const status = historyCardSummaryStatus(card && card.summary);
      const rawTitle = String((card && card.title) || '').trim() || 'job';
      const kind = String((card && card.kind) || '').trim() || 'job';
      const fullTitle = kind + ': ' + rawTitle;
      const title = escapeHtml(fullTitle);
      let iconHTML = '';
      const sections = (card && card.sections) || [];
      if (Array.isArray(sections) && sections.length > 0) {
        const firstSection = sections[0] || {};
        const firstItem = Array.isArray(firstSection.items) && firstSection.items.length > 0 ? firstSection.items[0] : null;
        let job = firstItem && firstItem.job ? firstItem.job : null;
        if (!job && firstItem && Array.isArray(firstItem.items) && firstItem.items.length > 0) {
          job = firstItem.items[0] && firstItem.items[0].job ? firstItem.items[0].job : null;
        }
        const iconURLFn = (opts && typeof opts.projectIconURL === 'function') ? opts.projectIconURL : null;
        const iconURL = (iconURLFn && job) ? String(iconURLFn(job) || '').trim() : '';
        if (iconURL) {
          iconHTML = '<img class="ciwi-job-group-side-icon" src="' + escapeHtml(iconURL) + '" alt="" onerror="this.style.display=&quot;none&quot;" />';
        }
      }
      const canFlush = !!(opts && typeof opts.onFlushCard === 'function' && Array.isArray(card && card.job_execution_ids) && card.job_execution_ids.length > 0);
      const flushButton = canFlush
        ? '<button type="button" class="secondary ciwi-icon-only ciwi-history-card-flush" data-ciwi-history-card-flush aria-label="Delete this execution from history">' + ciwiIconHTML('trash') + '</button>'
        : '';
      return '<span class="ciwi-job-group-main">' + iconHTML + '<span class="ciwi-job-group-status-icon ' + status.cls + '" aria-hidden="true">' + ciwiIconHTML(status.icon, { className: status.iconClass || '' }) +
        '</span><span class="ciwi-job-group-title" data-ciwi-overflow-text="' + escapeHtml(fullTitle) + '">' + title +
        '</span></span><span class="ciwi-job-group-status ' + status.cls + '">' + escapeHtml(status.text) + '</span>' + flushButton;
    }

    function bindHistoryCardFlushButton(element, card, opts) {
      const button = element && element.querySelector('[data-ciwi-history-card-flush]');
      if (!button || button.__ciwiFlushBound || !opts || typeof opts.onFlushCard !== 'function') return;
      button.__ciwiFlushBound = true;
      button.addEventListener('mousedown', event => event.stopPropagation());
      button.addEventListener('click', event => {
        event.preventDefault();
        event.stopPropagation();
        opts.onFlushCard(card, event, button);
      });
      createHoverTooltip(button, {
        html: '<strong>Delete this execution</strong><br />Removes every finished job attempt in this displayed execution, including its server-side logs, events, test results, and stored artifacts. It does not affect queued or running jobs, and does not clear agent caches or agent workspaces.<br /><br />Shift-click to delete without confirmation.',
        showDelayMs: 600,
        hideOnAnchorLeave: true,
        owner: 'history-card-flush',
      });
    }

    function setHistoryCardHeadHTML(element, html, card, opts) {
      if (!element) return;
      if (element.__ciwiCardHeadHTML !== html) {
        element.querySelectorAll('[data-ciwi-history-card-flush]').forEach(button => {
          if (button.__ciwiHoverTooltip && typeof button.__ciwiHoverTooltip.destroy === 'function') {
            button.__ciwiHoverTooltip.destroy();
          }
        });
        if (typeof destroyOverflowTooltips === 'function') {
          destroyOverflowTooltips(element);
        }
        element.innerHTML = html;
        element.__ciwiCardHeadHTML = html;
        if (typeof bindOverflowTooltips === 'function') {
          bindOverflowTooltips(element, { ownerPrefix: 'history-card-title' });
        }
      }
      bindHistoryCardFlushButton(element, card, opts);
    }

    function mergeCardSummaryIntoDetail(summaryCard, detailCard) {
      if (!detailCard) return summaryCard;
      return {
        ...detailCard,
        title: (summaryCard && summaryCard.title) || detailCard.title,
        job_execution_ids: (summaryCard && summaryCard.job_execution_ids) || detailCard.job_execution_ids,
        summary: (summaryCard && summaryCard.summary) || detailCard.summary,
        shape: (summaryCard && summaryCard.shape) || detailCard.shape,
      };
    }

    function historyItemRenderState(item) {
      const current = item || {};
      const job = current.job || null;
      return {
        kind: current.kind || '',
        key: current.key || '',
        label: current.label || '',
        matrix_label: current.matrix_label || '',
        job: job ? {
          id: job.id || '',
          status: job.status || '',
          metadata: job.metadata || {},
          leased_by_agent_id: job.leased_by_agent_id || '',
          created_utc: job.created_utc || '',
          started_utc: job.started_utc || '',
          finished_utc: job.finished_utc || '',
          error: job.error || '',
          test_summary: job.test_summary || null,
          unmet_requirements: job.unmet_requirements || [],
        } : null,
        items: (Array.isArray(current.items) ? current.items : []).map(historyItemRenderState),
      };
    }

    function historyCardSectionsSignature(card) {
      return JSON.stringify({
        kind: String((card && card.kind) || ''),
        sections: (Array.isArray(card && card.sections) ? card.sections : []).map(section => ({
          kind: section.kind || '',
          key: section.key || '',
          label: section.label || '',
          progress_jobs: section.progress_jobs || [],
          items: (Array.isArray(section.items) ? section.items : []).map(historyItemRenderState),
        })),
      });
    }

    function jobsFromHistoryItems(items) {
      const jobs = [];
      (Array.isArray(items) ? items : []).forEach(item => {
        if (item && item.job) jobs.push(item.job);
        if (item && Array.isArray(item.items)) jobs.push(...jobsFromHistoryItems(item.items));
      });
      return jobs;
    }

    function jobsFromHistoryCard(card) {
      const progressJobs = Array.isArray(card && card.progress_jobs) ? card.progress_jobs : [];
      if (progressJobs.length) return progressJobs;
      const jobs = [];
      (Array.isArray(card && card.sections) ? card.sections : []).forEach(section => {
        jobs.push(...jobsFromHistoryItems(section && section.items));
      });
      return jobs;
    }

    function bindQueueCardHeadProgress(container, card) {
      if (!container) return;
      const jobs = jobsFromHistoryCard(card);
      if (!jobs.length) return;
      const head = container.matches && container.matches('.ciwi-job-group-details')
        ? container.querySelector(':scope > summary')
        : container.querySelector(':scope > .ciwi-job-group-head');
      bindCiwiProgress(head, jobs);
    }

    function buildHistorySectionsContent(card, opts) {
      const sections = Array.isArray(card && card.sections) ? card.sections : [];
      if (sections.length === 0) {
        const empty = document.createElement('div');
        empty.className = 'ciwi-job-history-empty-card';
        empty.textContent = 'No jobs in this execution.';
        return empty;
      }
      const root = document.createElement('div');
      root.className = 'ciwi-job-history-sections';
      const rowOpts = { ...(opts || {}), projectIconURL: null };
      sections.forEach((section, sectionIndex) => {
        const block = document.createElement('div');
        block.className = 'ciwi-job-history-section';
        const showSectionHead = !(String((card && card.kind) || '').trim() === 'pipeline' && sections.length === 1);
        if (showSectionHead) {
          const head = document.createElement('div');
          head.className = 'ciwi-job-history-section-head';
          const label = String((section && section.label) || '').trim() || ('pipeline ' + String(sectionIndex + 1));
          const headLabel = document.createElement('span');
          headLabel.textContent = 'pipeline: ' + label;
          head.appendChild(headLabel);
          if (opts && opts.progressEnabled) {
            const sectionJobs = Array.isArray(section && section.progress_jobs) && section.progress_jobs.length
              ? section.progress_jobs
              : jobsFromHistoryItems(section && section.items);
            bindCiwiProgress(head, sectionJobs);
          }
          block.appendChild(head);
        }
        const table = document.createElement('table');
        table.className = 'ciwi-job-group-table';
        const body = document.createElement('tbody');
        const items = Array.isArray(section && section.items) ? section.items : [];
        items.forEach(item => {
          if (String(item && item.kind || '') === 'matrix') {
            const matrixLabel = String((item && item.label) || '').trim() || 'matrix';
            const headRow = document.createElement('tr');
            headRow.className = 'ciwi-job-history-matrix';
            const headTd = document.createElement('td');
            headTd.colSpan = 7;
            headTd.className = 'ciwi-job-history-matrix-head';
            const matrixHeadLabel = document.createElement('span');
            matrixHeadLabel.textContent = 'matrix: ' + matrixLabel;
            headTd.appendChild(matrixHeadLabel);
            headRow.appendChild(headTd);
            body.appendChild(headRow);
            const matrixItems = Array.isArray(item.items) ? item.items : [];
            matrixItems.forEach(child => {
              const row = buildJobExecutionRow(child.job || {}, rowOpts);
              body.appendChild(row);
            });
            return;
          }
          const row = buildJobExecutionRow(item.job || {}, rowOpts);
          body.appendChild(row);
        });
        table.appendChild(body);
        block.appendChild(table);
        root.appendChild(block);
      });
      return root;
    }

    function patchHistorySectionsContent(container, card, opts) {
      if (!container || !card) return;
      const signature = historyCardSectionsSignature(card);
      const existing = container.querySelector(':scope > .ciwi-job-history-sections, :scope > .ciwi-job-history-empty-card, :scope > .ciwi-job-group-skel-body');
      if (existing && container.__ciwiSectionsSignature === signature) return;
      if (existing) {
        if (typeof destroyOverflowTooltips === 'function') {
          destroyOverflowTooltips(existing);
        }
        existing.remove();
      }
      container.appendChild(buildHistorySectionsContent(card, opts));
      container.__ciwiSectionsSignature = signature;
    }

    function ensureHistoryCardOpenBody(details, card, opts) {
      if (!details || !card) return;
      const existing = details.querySelector('.ciwi-job-history-sections, .ciwi-job-history-empty-card, .ciwi-job-group-skel-body');
      if (existing) return;
      if (Array.isArray(card.sections) && card.sections.length > 0) {
        patchHistorySectionsContent(details, card, opts);
        return;
      }
      details.appendChild(buildHistorySkeletonBody(historyExpandedRowHint(card)));
    }

    function bindHistoryCardToggle(details, fallbackCard) {
      if (!details || details.__ciwiHistoryToggleBound) return;
      details.__ciwiHistoryToggleBound = true;
      details.addEventListener('toggle', () => {
        const cardKey = String(details.__ciwiHistoryCardKey || '').trim();
        const currentCard = details.__ciwiHistoryCard || historyCardDetailsByKey[cardKey] || fallbackCard;
        const currentOpts = details.__ciwiHistoryOpts || null;
        const groupKey = historyCardGroupKey((currentCard && currentCard.key) || cardKey);
        if (details.open) {
          expandedJobGroups.add(groupKey);
          ensureHistoryCardOpenBody(details, currentCard, currentOpts);
        } else {
          expandedJobGroups.delete(groupKey);
        }
        saveStringSet(JOB_GROUPS_STORAGE_KEY, expandedJobGroups);
      });
    }

    function patchHistorySummaryCard(tbody, card, opts, columnCount) {
      const row = findHistoryCardRow(tbody, card && card.key);
      if (!row) return;
      row.classList.remove('ciwi-job-skeleton-row');
      const td = row.firstElementChild;
      if (!td) return;
      td.colSpan = columnCount;
      const collapsible = historyCardIsCollapsible(card);
      const groupKey = historyCardGroupKey(card && card.key);
      if (collapsible) {
        let details = td.querySelector('.ciwi-job-group-details');
        if (!details) {
          if (typeof destroyOverflowTooltips === 'function') {
            destroyOverflowTooltips(td);
          }
          td.innerHTML = '';
          details = document.createElement('details');
          details.className = 'ciwi-job-group-details';
          td.appendChild(details);
        }
        details.__ciwiHistoryCardKey = String((card && card.key) || '').trim();
        const renderedCard = mergeCardSummaryIntoDetail(card, historyCardDetailsByKey[details.__ciwiHistoryCardKey]);
        details.__ciwiHistoryCard = renderedCard;
        details.__ciwiHistoryOpts = opts || null;
        bindHistoryCardToggle(details, card);
        if (expandedJobGroups.has(groupKey)) {
          details.open = true;
        }
        let summary = details.querySelector('summary');
        if (!summary) {
          summary = document.createElement('summary');
          details.appendChild(summary);
        }
        setHistoryCardHeadHTML(summary, buildHistoryCardHeadHTML(renderedCard, opts) + '<span class="ciwi-job-group-toggle" aria-hidden="true">' + ciwiIconHTML('chevron-right') + '</span>', renderedCard, opts);
        if (!details.open) {
          const skel = details.querySelector('.ciwi-job-group-skel-body');
          if (skel) skel.remove();
        } else if (!details.querySelector('.ciwi-job-group-skel-body')) {
          ensureHistoryCardOpenBody(details, historyCardDetailsByKey[String((card && card.key) || '').trim()] || card, opts);
        }
      } else {
        let cardEl = td.querySelector('.ciwi-job-group-card');
        if (!cardEl) {
          cardEl = document.createElement('div');
          cardEl.className = 'ciwi-job-group-card';
          if (typeof destroyOverflowTooltips === 'function') {
            destroyOverflowTooltips(td);
          }
          td.innerHTML = '';
          td.appendChild(cardEl);
        }
        let head = cardEl.querySelector('.ciwi-job-group-head');
        if (!head) {
          head = document.createElement('div');
          head.className = 'ciwi-job-group-head';
          cardEl.insertBefore(head, cardEl.firstChild || null);
        }
        const renderedCard = mergeCardSummaryIntoDetail(card, historyCardDetailsByKey[String((card && card.key) || '').trim()]);
        setHistoryCardHeadHTML(head, buildHistoryCardHeadHTML(renderedCard, opts), renderedCard, opts);
      }
    }

    function patchHistoryFullCard(tbody, card, opts, columnCount) {
      historyCardDetailsByKey[String((card && card.key) || '').trim()] = card;
      const row = findHistoryCardRow(tbody, card && card.key);
      if (!row) return;
      const td = row.firstElementChild;
      if (!td) return;
      td.colSpan = columnCount;
      const collapsible = historyCardIsCollapsible(card);
      const groupKey = historyCardGroupKey(card && card.key);
      if (collapsible) {
        let details = td.querySelector('.ciwi-job-group-details');
        if (!details) {
          patchHistorySummaryCard(tbody, card, opts, columnCount);
          details = td.querySelector('.ciwi-job-group-details');
        }
        if (!details) return;
        details.__ciwiHistoryCardKey = String((card && card.key) || '').trim();
        details.__ciwiHistoryCard = card;
        details.__ciwiHistoryOpts = opts || null;
        if (expandedJobGroups.has(groupKey)) {
          details.open = true;
        }
        if (details.open) {
          patchHistorySectionsContent(details, card, opts);
        }
      } else {
        const cardEl = td.querySelector('.ciwi-job-group-card');
        if (!cardEl) return;
        patchHistorySectionsContent(cardEl, card, opts);
      }
    }

    function queueCardLayoutSignature(cards) {
      const rows = Array.isArray(cards) ? cards : [];
      return rows.map(card => String((card && card.key) || '').trim()).join('\x1f');
    }

    function queueCardGroupKey(cardKey) {
      return 'queued:' + String(cardKey || '').trim();
    }

    function setAllJobExecutionGroupsExpanded(kind, expanded) {
      const queued = kind === 'queued';
      const tbody = document.getElementById(queued ? 'queuedJobsBody' : 'historyJobsBody');
      if (!tbody) return;
      tbody.querySelectorAll('.ciwi-job-group-details').forEach(details => {
        const cardKey = String((queued ? details.__ciwiQueueCardKey : details.__ciwiHistoryCardKey) || '').trim();
        if (!cardKey) return;
        const card = queued
          ? (details.__ciwiQueueCard || queueCardDetailsByKey[cardKey])
          : (details.__ciwiHistoryCard || historyCardDetailsByKey[cardKey]);
        const opts = queued ? details.__ciwiQueueOpts : details.__ciwiHistoryOpts;
        const groupKey = queued ? queueCardGroupKey(cardKey) : historyCardGroupKey(cardKey);
        if (expanded) {
          expandedJobGroups.add(groupKey);
          details.open = true;
          ensureHistoryCardOpenBody(details, card, opts || null);
        } else {
          expandedJobGroups.delete(groupKey);
          details.open = false;
        }
      });
      saveStringSet(JOB_GROUPS_STORAGE_KEY, expandedJobGroups);
    }

    function buildQueueCardSkeletonRow(card, columnCount) {
      ensureJobSkeletonStyles();
      const tr = document.createElement('tr');
      tr.className = 'ciwi-job-group-row ciwi-job-skeleton-row';
      tr.dataset.ciwiQueueCardKey = String((card && card.key) || '').trim();
      const td = document.createElement('td');
      td.colSpan = columnCount;
      const collapsible = historyCardIsCollapsible(card);
      const groupKey = queueCardGroupKey(card && card.key);
      const expanded = collapsible && expandedJobGroups.has(groupKey);
      if (collapsible) {
        const details = document.createElement('details');
        details.className = 'ciwi-job-group-details';
        details.__ciwiQueueCardKey = String((card && card.key) || '').trim();
        const renderedCard = mergeCardSummaryIntoDetail(card, queueCardDetailsByKey[details.__ciwiQueueCardKey]);
        details.__ciwiQueueCard = renderedCard;
        details.__ciwiQueueOpts = null;
        if (expanded) details.open = true;
        const summary = document.createElement('summary');
        summary.className = 'ciwi-job-group-skel-head';
        summary.innerHTML =
          '<span class="ciwi-job-group-main">' +
            '<span class="ciwi-job-group-status-icon" aria-hidden="true">' + ciwiIconHTML('loader-2', { className: 'ciwi-icon-spin' }) + '</span>' +
            '<span class="ciwi-job-group-title"><span class="ciwi-job-skeleton-bar" style="width:180px;display:inline-block;"></span></span>' +
          '</span>' +
          '<span class="ciwi-job-group-status status-queued"><span class="ciwi-job-skeleton-bar ciwi-job-skeleton-bar-short" style="width:110px;display:inline-block;"></span></span>' +
          '<span class="ciwi-job-group-toggle" aria-hidden="true">' + ciwiIconHTML('chevron-right') + '</span>';
        details.appendChild(summary);
        if (expanded) {
          details.appendChild(buildHistorySkeletonBody(historyExpandedRowHint(card)));
        }
        if (!details.__ciwiQueueToggleBound) {
          details.__ciwiQueueToggleBound = true;
          details.addEventListener('toggle', () => {
            const cardKey = String(details.__ciwiQueueCardKey || '').trim();
            const currentCard = details.__ciwiQueueCard || queueCardDetailsByKey[cardKey] || card;
            const currentOpts = details.__ciwiQueueOpts || null;
            const currentGroupKey = queueCardGroupKey((currentCard && currentCard.key) || cardKey);
            if (details.open) {
              expandedJobGroups.add(currentGroupKey);
              ensureHistoryCardOpenBody(details, currentCard, currentOpts);
            } else {
              expandedJobGroups.delete(currentGroupKey);
            }
            saveStringSet(JOB_GROUPS_STORAGE_KEY, expandedJobGroups);
          });
        }
        td.appendChild(details);
      } else {
        const cardEl = document.createElement('div');
        cardEl.className = 'ciwi-job-group-card';
        const head = document.createElement('div');
        head.className = 'ciwi-job-group-head';
        head.innerHTML =
          '<span class="ciwi-job-group-main">' +
            '<span class="ciwi-job-group-status-icon" aria-hidden="true">' + ciwiIconHTML('loader-2', { className: 'ciwi-icon-spin' }) + '</span>' +
            '<span class="ciwi-job-group-title"><span class="ciwi-job-skeleton-bar" style="width:180px;display:inline-block;"></span></span>' +
          '</span>' +
          '<span class="ciwi-job-group-status status-queued"><span class="ciwi-job-skeleton-bar ciwi-job-skeleton-bar-short" style="width:110px;display:inline-block;"></span></span>';
        cardEl.appendChild(head);
        td.appendChild(cardEl);
      }
      tr.appendChild(td);
      return tr;
    }

    function renderQueueLayoutRows(tbody, cards, columnCount) {
      const rows = Array.isArray(cards) ? cards : [];
      if (typeof destroyOverflowTooltips === 'function') {
        destroyOverflowTooltips(tbody);
      }
      tbody.innerHTML = '';
      if (rows.length === 0) {
        const tr = document.createElement('tr');
        tr.className = 'ciwi-empty-row';
        tr.innerHTML = '<td colspan="' + String(columnCount) + '" class="muted">No queued jobs.</td>';
        tbody.appendChild(tr);
        return;
      }
      rows.forEach(card => tbody.appendChild(buildQueueCardSkeletonRow(card, columnCount)));
    }

    function findQueueCardRow(tbody, key) {
      if (!tbody) return null;
      const target = String(key || '').trim();
      return Array.from(tbody.children || []).find(row => row && row.dataset && row.dataset.ciwiQueueCardKey === target) || null;
    }

    function patchQueueSummaryCard(tbody, card, opts, columnCount) {
      const row = findQueueCardRow(tbody, card && card.key);
      if (!row) return;
      row.classList.remove('ciwi-job-skeleton-row');
      const td = row.firstElementChild;
      if (!td) return;
      td.colSpan = columnCount;
      const collapsible = historyCardIsCollapsible(card);
      const groupKey = queueCardGroupKey(card && card.key);
      if (collapsible) {
        let details = td.querySelector('.ciwi-job-group-details');
        if (!details) {
          if (typeof destroyOverflowTooltips === 'function') {
            destroyOverflowTooltips(td);
          }
          td.innerHTML = '';
          details = document.createElement('details');
          details.className = 'ciwi-job-group-details';
          td.appendChild(details);
        }
        details.__ciwiQueueCardKey = String((card && card.key) || '').trim();
        const renderedCard = mergeCardSummaryIntoDetail(card, queueCardDetailsByKey[details.__ciwiQueueCardKey]);
        details.__ciwiQueueCard = renderedCard;
        details.__ciwiQueueOpts = opts || null;
        if (!details.__ciwiQueueToggleBound) {
          details.__ciwiQueueToggleBound = true;
          details.addEventListener('toggle', () => {
            const cardKey = String(details.__ciwiQueueCardKey || '').trim();
            const currentCard = details.__ciwiQueueCard || queueCardDetailsByKey[cardKey] || card;
            const currentOpts = details.__ciwiQueueOpts || null;
            const currentGroupKey = queueCardGroupKey((currentCard && currentCard.key) || cardKey);
            if (details.open) {
              expandedJobGroups.add(currentGroupKey);
              ensureHistoryCardOpenBody(details, currentCard, currentOpts);
            } else {
              expandedJobGroups.delete(currentGroupKey);
            }
            saveStringSet(JOB_GROUPS_STORAGE_KEY, expandedJobGroups);
          });
        }
        if (expandedJobGroups.has(groupKey)) {
          details.open = true;
        }
        let summary = details.querySelector('summary');
        if (!summary) {
          summary = document.createElement('summary');
          details.appendChild(summary);
        }
        setHistoryCardHeadHTML(summary, buildHistoryCardHeadHTML(renderedCard, opts) + '<span class="ciwi-job-group-toggle" aria-hidden="true">' + ciwiIconHTML('chevron-right') + '</span>', renderedCard, opts);
        if (!details.open) {
          const skel = details.querySelector('.ciwi-job-group-skel-body');
          if (skel) skel.remove();
        } else if (!details.querySelector('.ciwi-job-group-skel-body')) {
          ensureHistoryCardOpenBody(details, queueCardDetailsByKey[String((card && card.key) || '').trim()] || card, opts);
        }
      } else {
        let cardEl = td.querySelector('.ciwi-job-group-card');
        if (!cardEl) {
          cardEl = document.createElement('div');
          cardEl.className = 'ciwi-job-group-card';
          if (typeof destroyOverflowTooltips === 'function') {
            destroyOverflowTooltips(td);
          }
          td.innerHTML = '';
          td.appendChild(cardEl);
        }
        let head = cardEl.querySelector('.ciwi-job-group-head');
        if (!head) {
          head = document.createElement('div');
          head.className = 'ciwi-job-group-head';
          cardEl.insertBefore(head, cardEl.firstChild || null);
        }
        const renderedCard = mergeCardSummaryIntoDetail(card, queueCardDetailsByKey[String((card && card.key) || '').trim()]);
        setHistoryCardHeadHTML(head, buildHistoryCardHeadHTML(renderedCard, opts), renderedCard, opts);
      }
    }

    function patchQueueFullCard(tbody, card, opts, columnCount) {
      queueCardDetailsByKey[String((card && card.key) || '').trim()] = card;
      const row = findQueueCardRow(tbody, card && card.key);
      if (!row) return;
      const td = row.firstElementChild;
      if (!td) return;
      td.colSpan = columnCount;
      const collapsible = historyCardIsCollapsible(card);
      const groupKey = queueCardGroupKey(card && card.key);
      if (collapsible) {
        let details = td.querySelector('.ciwi-job-group-details');
        if (!details) {
          patchQueueSummaryCard(tbody, card, opts, columnCount);
          details = td.querySelector('.ciwi-job-group-details');
        }
        if (!details) return;
        details.__ciwiQueueCardKey = String((card && card.key) || '').trim();
        details.__ciwiQueueCard = card;
        details.__ciwiQueueOpts = opts || null;
        if (expandedJobGroups.has(groupKey)) {
          details.open = true;
        }
        let summary = details.querySelector(':scope > summary');
        if (!summary) {
          summary = document.createElement('summary');
          details.insertBefore(summary, details.firstChild || null);
        }
        setHistoryCardHeadHTML(summary, buildHistoryCardHeadHTML(card, opts) + '<span class="ciwi-job-group-toggle" aria-hidden="true">' + ciwiIconHTML('chevron-right') + '</span>', card, opts);
        if (details.open) {
          patchHistorySectionsContent(details, card, opts);
        }
        if (opts && opts.progressEnabled) bindQueueCardHeadProgress(details, card);
      } else {
        const cardEl = td.querySelector('.ciwi-job-group-card');
        if (!cardEl) return;
        const head = cardEl.querySelector(':scope > .ciwi-job-group-head');
        if (head) setHistoryCardHeadHTML(head, buildHistoryCardHeadHTML(card, opts), card, opts);
        patchHistorySectionsContent(cardEl, card, opts);
        if (opts && opts.progressEnabled) bindQueueCardHeadProgress(cardEl, card);
      }
    }

    async function refreshQueueCards(epoch, tbody, opts, columnCount) {
      const layout = await apiJSON('/api/v1/job-queue/layout?offset=0&limit=' + String(HISTORY_CARD_WINDOW));
      if (epoch !== jobsRenderEpoch) return null;
      const cards = Array.isArray(layout.cards) ? layout.cards : [];
      const layoutSig = queueCardLayoutSignature(cards);
      if (!tbodyHasConcreteRows(tbody) || layoutSig !== lastQueuedLayoutSignature) {
        renderQueueLayoutRows(tbody, cards, columnCount);
        lastQueuedLayoutSignature = layoutSig;
      }
      if (cards.length === 0) {
        return '';
      }
      for (let offset = 0; offset < cards.length; offset += HISTORY_CARD_BATCH) {
        const summary = await apiJSON('/api/v1/job-queue/cards?detail=summary&offset=' + String(offset) + '&limit=' + String(HISTORY_CARD_BATCH));
        if (epoch !== jobsRenderEpoch) return null;
        (summary.cards || []).forEach(card => patchQueueSummaryCard(tbody, card, opts, columnCount));
      }
      for (let offset = 0; offset < cards.length; offset += HISTORY_CARD_BATCH) {
        const full = await apiJSON('/api/v1/job-queue/cards?detail=full&offset=' + String(offset) + '&limit=' + String(HISTORY_CARD_BATCH));
        if (epoch !== jobsRenderEpoch) return null;
        (full.cards || []).forEach(card => patchQueueFullCard(tbody, card, opts, columnCount));
      }
      return layoutSig;
    }

    async function refreshHistoryCards(epoch, tbody, opts, columnCount, atomic) {
      const layout = await apiJSON('/api/v1/job-history/layout?offset=0&limit=' + String(HISTORY_CARD_WINDOW));
      if (epoch !== jobsRenderEpoch) return null;
      const cards = Array.isArray(layout.cards) ? layout.cards : [];
      const layoutSig = historyLayoutSignature(cards);
      if (atomic && cards.length > 0) {
        const pageCount = Math.ceil(cards.length / HISTORY_CARD_BATCH);
        const offsets = Array.from({ length: pageCount }, (_, index) => index * HISTORY_CARD_BATCH);
        const [summaryPages, fullPages] = await Promise.all([
          Promise.all(offsets.map(offset => apiJSON('/api/v1/job-history/cards?detail=summary&offset=' + String(offset) + '&limit=' + String(HISTORY_CARD_BATCH)))),
          Promise.all(offsets.map(offset => apiJSON('/api/v1/job-history/cards?detail=full&offset=' + String(offset) + '&limit=' + String(HISTORY_CARD_BATCH)))),
        ]);
        if (epoch !== jobsRenderEpoch) return null;
        if (!tbodyHasConcreteRows(tbody) || layoutSig !== lastHistoryLayoutSignature) {
          renderHistoryLayoutRows(tbody, cards, columnCount);
          lastHistoryLayoutSignature = layoutSig;
        }
        summaryPages.forEach(page => (page.cards || []).forEach(card => patchHistorySummaryCard(tbody, card, opts, columnCount)));
        fullPages.forEach(page => (page.cards || []).forEach(card => patchHistoryFullCard(tbody, card, opts, columnCount)));
        return layoutSig;
      }
      if (!tbodyHasConcreteRows(tbody) || layoutSig !== lastHistoryLayoutSignature) {
        renderHistoryLayoutRows(tbody, cards, columnCount);
        lastHistoryLayoutSignature = layoutSig;
      }
      if (cards.length === 0) {
        return '';
      }
      for (let offset = 0; offset < cards.length; offset += HISTORY_CARD_BATCH) {
        const summary = await apiJSON('/api/v1/job-history/cards?detail=summary&offset=' + String(offset) + '&limit=' + String(HISTORY_CARD_BATCH));
        if (epoch !== jobsRenderEpoch) return null;
        (summary.cards || []).forEach(card => patchHistorySummaryCard(tbody, card, opts, columnCount));
      }
      for (let offset = 0; offset < cards.length; offset += HISTORY_CARD_BATCH) {
        const full = await apiJSON('/api/v1/job-history/cards?detail=full&offset=' + String(offset) + '&limit=' + String(HISTORY_CARD_BATCH));
        if (epoch !== jobsRenderEpoch) return null;
        (full.cards || []).forEach(card => patchHistoryFullCard(tbody, card, opts, columnCount));
      }
      return layoutSig;
    }

    function captureHistoryDeletionAnchor(button) {
      const row = button && button.closest('tr[data-ciwi-history-card-key]');
      if (!row) return null;
      const rows = Array.from(document.querySelectorAll('#historyJobsBody > tr[data-ciwi-history-card-key]'));
      const index = rows.indexOf(row);
      const anchorRow = index > 0 ? rows[index - 1] : (index + 1 < rows.length ? rows[index + 1] : null);
      if (!anchorRow) return null;
      const anchorButton = anchorRow.querySelector('[data-ciwi-history-card-flush]');
      const anchorRect = anchorButton ? anchorButton.getBoundingClientRect() : anchorRow.getBoundingClientRect();
      const clickedRect = button.getBoundingClientRect();
      const anchorVisible = anchorRect.bottom > 0 && anchorRect.top < window.innerHeight;
      return {
        cardKey: String(anchorRow.dataset.ciwiHistoryCardKey || '').trim(),
        viewportTop: anchorVisible ? anchorRect.top : clickedRect.top,
      };
    }

    function restoreHistoryDeletionAnchor(anchor) {
      if (!anchor || !anchor.cardKey) return;
      requestAnimationFrame(() => requestAnimationFrame(() => {
        const row = findHistoryCardRow(document.getElementById('historyJobsBody'), anchor.cardKey);
        if (!row) return;
        const button = row.querySelector('[data-ciwi-history-card-flush]');
        const rect = button ? button.getBoundingClientRect() : row.getBoundingClientRect();
        const delta = rect.top - anchor.viewportTop;
        if (Math.abs(delta) > 0.5) window.scrollBy({ left: 0, top: delta, behavior: 'auto' });
      }));
    }

    async function flushHistoryCard(card, event, button) {
      const jobExecutionIDs = Array.from(new Set((Array.isArray(card && card.job_execution_ids) ? card.job_execution_ids : [])
        .map(value => String(value || '').trim())
        .filter(Boolean)));
      if (!jobExecutionIDs.length) return;
      if (!(event && event.shiftKey)) {
        const confirmed = await showConfirmDialog({
          title: 'Delete Execution',
          message: 'Delete this execution and all of its finished job attempts from server history? This removes server-side logs, events, test results, and stored artifacts. Agent caches and workspaces are not cleared.',
          okLabel: 'Delete execution',
        });
        if (!confirmed) return;
      }
      const deletionAnchor = captureHistoryDeletionAnchor(button);
      if (button) button.disabled = true;
      try {
        await apiJSON('/api/v1/jobs/flush-history', {
          method: 'POST',
          body: JSON.stringify({ job_execution_ids: jobExecutionIDs }),
        });
        const cardKey = String((card && card.key) || '').trim();
        delete historyCardDetailsByKey[cardKey];
        expandedJobGroups.delete(historyCardGroupKey(cardKey));
        saveStringSet(JOB_GROUPS_STORAGE_KEY, expandedJobGroups);
        await refreshJobs({ atomicHistory: true });
        restoreHistoryDeletionAnchor(deletionAnchor);
      } catch (e) {
        if (button) button.disabled = false;
        await showAlertDialog({ title: 'Delete execution failed', message: 'Delete execution failed: ' + e.message });
      }
    }

    async function refreshJobs(options) {
      const refreshOptions = options || {};
      const epoch = ++jobsRenderEpoch;
      const queuedBody = document.getElementById('queuedJobsBody');
      const historyBody = document.getElementById('historyJobsBody');

      const queuedOpts = {
        progressEnabled: true,
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
            await showAlertDialog({ title: 'Remove failed', message: 'Remove failed: ' + e.message });
          }
        },
        onCancel: async (j) => {
          const confirmed = await showConfirmDialog({
            title: 'Cancel Job',
            message: 'Cancel this running job?',
            okLabel: 'Cancel job',
          });
          if (!confirmed) {
            return;
          }
          try {
            await apiJSON('/api/v1/jobs/' + j.id + '/cancel', { method: 'POST', body: '{}' });
            await refreshJobs();
          } catch (e) {
            await showAlertDialog({ title: 'Cancel failed', message: 'Cancel failed: ' + e.message });
          }
        }
      };
      const historyOpts = {
        includeDuration: true,
        fixedLines: 2,
        backPath: window.location.pathname || '/',
        linkClass: 'job-link',
        projectIconURL: projectIconURLForJob,
        onFlushCard: flushHistoryCard,
      };

      const [queuedSig, historySig] = await Promise.all([
        refreshQueueCards(epoch, queuedBody, queuedOpts, 8),
        refreshHistoryCards(epoch, historyBody, historyOpts, 7, !!refreshOptions.atomicHistory),
      ]);
      if (epoch !== jobsRenderEpoch || queuedSig === null || historySig === null) return;
      if (queuedSig !== lastQueuedJobsSignature) {
        lastQueuedJobsSignature = queuedSig;
      }
      if (historySig !== lastHistoryJobsSignature) {
        lastHistoryJobsSignature = historySig;
      }
    }

    const clearQueueBtn = document.getElementById('clearQueueBtn');
    const flushHistoryBtn = document.getElementById('flushHistoryBtn');
    createHoverTooltip(clearQueueBtn, {
      html: '<strong>Clear Queue</strong><br />Removes all queued and leased job records from the server. It does not cancel jobs that are already running, and does not clear agent caches or agent workspaces.',
      showDelayMs: 600,
      hideOnAnchorLeave: true,
      owner: 'clear-queue',
    });
    createHoverTooltip(flushHistoryBtn, {
      html: '<strong>Flush History</strong><br />Removes all finished jobs from server history, including their server-side logs, events, test results, and stored artifacts. Queued and running jobs are left alone. It does not clear caches or workspaces on any agent.',
      showDelayMs: 600,
      hideOnAnchorLeave: true,
      owner: 'flush-history',
    });

    clearQueueBtn.onclick = async () => {
      const confirmed = await showConfirmDialog({
        title: 'Clear Queue',
        message: 'Remove all queued and leased jobs from the server? Running jobs are not cancelled. Agent caches and workspaces are not cleared.',
        okLabel: 'Clear queue',
      });
      if (!confirmed) {
        return;
      }
      try {
        await apiJSON('/api/v1/jobs/clear-queue', { method: 'POST', body: '{}' });
        await refreshJobs();
      } catch (e) {
        await showAlertDialog({ title: 'Clear queue failed', message: 'Clear queue failed: ' + e.message });
      }
    };

    flushHistoryBtn.onclick = async () => {
      const confirmed = await showConfirmDialog({
        title: 'Flush History',
        message: 'Remove all finished jobs from server history, including server-side logs, events, test results, and stored artifacts? Queued and running jobs are left alone. Agent caches and workspaces are not cleared.',
        okLabel: 'Flush history',
      });
      if (!confirmed) {
        return;
      }
      try {
        await apiJSON('/api/v1/jobs/flush-history', { method: 'POST', body: '{}' });
        await refreshJobs();
      } catch (e) {
        await showAlertDialog({ title: 'Flush history failed', message: 'Flush history failed: ' + e.message });
      }
    };

    document.getElementById('queuedCollapseAllBtn').onclick = () => setAllJobExecutionGroupsExpanded('queued', false);
    document.getElementById('queuedExpandAllBtn').onclick = () => setAllJobExecutionGroupsExpanded('queued', true);
    document.getElementById('historyCollapseAllBtn').onclick = () => setAllJobExecutionGroupsExpanded('history', false);
    document.getElementById('historyExpandAllBtn').onclick = () => setAllJobExecutionGroupsExpanded('history', true);

    let queuedFocusUntilMs = 0;

    function requestQueuedJobsFocusWindow(ms) {
      const now = Date.now();
      const duration = Math.max(1000, Number(ms || 8000));
      queuedFocusUntilMs = Math.max(queuedFocusUntilMs, now + duration);
    }

    function loadQueuedJobsFocusWindow() {
      if (window.location.hash === '#queued-jobs') {
        requestQueuedJobsFocusWindow(8000);
      }
    }

    function clearQueuedJobsFocusWindow() {
      queuedFocusUntilMs = 0;
    }

    function focusQueuedJobsIfRequested() {
      if (window.location.hash !== '#queued-jobs') {
        clearQueuedJobsFocusWindow();
        return;
      }
      const now = Date.now();
      if (queuedFocusUntilMs <= now) {
        clearQueuedJobsFocusWindow();
        return;
      }
      const node = document.getElementById('queued-jobs');
      if (!node || typeof node.scrollIntoView !== 'function') return;
      requestAnimationFrame(() => {
        node.scrollIntoView({ block: 'start', behavior: 'smooth' });
      });
    }

    async function tick() {
      if (refreshInFlight || refreshGuard.shouldPause()) {
        return;
      }
      refreshInFlight = true;
      try {
        await Promise.all([refreshProjects(), refreshJobs(), refreshRuntimeStateBanner('runtimeStateBanner')]);
      } catch (e) {
        console.error(e);
      } finally {
        focusQueuedJobsIfRequested();
        refreshInFlight = false;
      }
    }

    refreshGuard.bindSelectionListener();
    loadQueuedJobsFocusWindow();
    refreshServerVersionLabels();
    tick();
    focusQueuedJobsIfRequested();
    setInterval(tick, 3000);
