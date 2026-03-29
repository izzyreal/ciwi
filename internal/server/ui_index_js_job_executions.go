package server

const uiIndexJobExecutionsJS = `
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
      if (failed > 0) {
        return { emoji: '❌', cls: 'status-failed', text: succeeded + '/' + total + ' successful, ' + failed + ' failed' };
      }
      if (inProgress > 0) {
        return { emoji: '⏳', cls: 'status-running', text: succeeded + '/' + total + ' successful, ' + inProgress + ' in progress' };
      }
      if (total > 0 && succeeded === total) {
        return { emoji: '✅', cls: 'status-succeeded', text: succeeded + '/' + total + ' successful' };
      }
      return { emoji: '🟡', cls: 'status-queued', text: succeeded + '/' + total + ' successful' };
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
      return total > 1;
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
            '<span class="ciwi-job-group-emoji" aria-hidden="true">⏳</span>' +
            '<span class="ciwi-job-group-title"><span class="ciwi-job-skeleton-bar" style="width:180px;display:inline-block;"></span></span>' +
          '</span>' +
          '<span class="ciwi-job-group-status status-queued"><span class="ciwi-job-skeleton-bar ciwi-job-skeleton-bar-short" style="width:110px;display:inline-block;"></span></span>' +
          '<span class="ciwi-job-group-toggle" aria-hidden="true"></span>';
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
            '<span class="ciwi-job-group-emoji" aria-hidden="true">⏳</span>' +
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
      const title = escapeHtml(kind + ': ' + rawTitle);
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
      return '<span class="ciwi-job-group-main">' + iconHTML + '<span class="ciwi-job-group-emoji" aria-hidden="true">' + status.emoji +
        '</span><span class="ciwi-job-group-title">' + title +
        '</span></span><span class="ciwi-job-group-status ' + status.cls + '">' + escapeHtml(status.text) + '</span>';
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
          head.textContent = 'pipeline: ' + label;
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
            headTd.textContent = 'matrix: ' + matrixLabel;
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

    function ensureHistoryCardOpenBody(details, card, opts) {
      if (!details || !card) return;
      const existing = details.querySelector('.ciwi-job-history-sections, .ciwi-job-history-empty-card, .ciwi-job-group-skel-body');
      if (existing) return;
      if (Array.isArray(card.sections) && card.sections.length > 0) {
        details.appendChild(buildHistorySectionsContent(card, opts));
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
          td.innerHTML = '';
          details = document.createElement('details');
          details.className = 'ciwi-job-group-details';
          td.appendChild(details);
        }
        details.__ciwiHistoryCardKey = String((card && card.key) || '').trim();
        details.__ciwiHistoryCard = historyCardDetailsByKey[details.__ciwiHistoryCardKey] || card;
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
        summary.innerHTML = buildHistoryCardHeadHTML(card, opts) + '<span class="ciwi-job-group-toggle" aria-hidden="true"></span>';
        const body = details.querySelector('.ciwi-job-history-sections, .ciwi-job-history-empty-card');
        if (body) body.remove();
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
          td.innerHTML = '';
          td.appendChild(cardEl);
        }
        let head = cardEl.querySelector('.ciwi-job-group-head');
        if (!head) {
          head = document.createElement('div');
          head.className = 'ciwi-job-group-head';
          cardEl.insertBefore(head, cardEl.firstChild || null);
        }
        head.innerHTML = buildHistoryCardHeadHTML(card, opts);
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
        const oldBody = details.querySelector('.ciwi-job-history-sections, .ciwi-job-history-empty-card, .ciwi-job-group-skel-body');
        if (oldBody) oldBody.remove();
        if (details.open) {
          details.appendChild(buildHistorySectionsContent(card, opts));
        }
      } else {
        const cardEl = td.querySelector('.ciwi-job-group-card');
        if (!cardEl) return;
        const oldBody = cardEl.querySelector('.ciwi-job-history-sections, .ciwi-job-history-empty-card, .ciwi-job-group-skel-body');
        if (oldBody) oldBody.remove();
        cardEl.appendChild(buildHistorySectionsContent(card, opts));
      }
    }

    function queueCardLayoutSignature(cards) {
      const rows = Array.isArray(cards) ? cards : [];
      return rows.map(card => String((card && card.key) || '').trim()).join('\x1f');
    }

    function queueCardGroupKey(cardKey) {
      return 'queued:' + String(cardKey || '').trim();
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
        details.__ciwiQueueCard = queueCardDetailsByKey[details.__ciwiQueueCardKey] || card;
        details.__ciwiQueueOpts = null;
        if (expanded) details.open = true;
        const summary = document.createElement('summary');
        summary.className = 'ciwi-job-group-skel-head';
        summary.innerHTML =
          '<span class="ciwi-job-group-main">' +
            '<span class="ciwi-job-group-emoji" aria-hidden="true">⏳</span>' +
            '<span class="ciwi-job-group-title"><span class="ciwi-job-skeleton-bar" style="width:180px;display:inline-block;"></span></span>' +
          '</span>' +
          '<span class="ciwi-job-group-status status-queued"><span class="ciwi-job-skeleton-bar ciwi-job-skeleton-bar-short" style="width:110px;display:inline-block;"></span></span>' +
          '<span class="ciwi-job-group-toggle" aria-hidden="true"></span>';
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
            '<span class="ciwi-job-group-emoji" aria-hidden="true">⏳</span>' +
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
          td.innerHTML = '';
          details = document.createElement('details');
          details.className = 'ciwi-job-group-details';
          td.appendChild(details);
        }
        details.__ciwiQueueCardKey = String((card && card.key) || '').trim();
        details.__ciwiQueueCard = queueCardDetailsByKey[details.__ciwiQueueCardKey] || card;
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
        summary.innerHTML = buildHistoryCardHeadHTML(card, opts) + '<span class="ciwi-job-group-toggle" aria-hidden="true"></span>';
        const body = details.querySelector('.ciwi-job-history-sections, .ciwi-job-history-empty-card');
        if (body) body.remove();
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
          td.innerHTML = '';
          td.appendChild(cardEl);
        }
        let head = cardEl.querySelector('.ciwi-job-group-head');
        if (!head) {
          head = document.createElement('div');
          head.className = 'ciwi-job-group-head';
          cardEl.insertBefore(head, cardEl.firstChild || null);
        }
        head.innerHTML = buildHistoryCardHeadHTML(card, opts);
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
        const oldBody = details.querySelector('.ciwi-job-history-sections, .ciwi-job-history-empty-card, .ciwi-job-group-skel-body');
        if (oldBody) oldBody.remove();
        if (details.open) {
          details.appendChild(buildHistorySectionsContent(card, opts));
        }
      } else {
        const cardEl = td.querySelector('.ciwi-job-group-card');
        if (!cardEl) return;
        const oldBody = cardEl.querySelector('.ciwi-job-history-sections, .ciwi-job-history-empty-card, .ciwi-job-group-skel-body');
        if (oldBody) oldBody.remove();
        cardEl.appendChild(buildHistorySectionsContent(card, opts));
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

    async function refreshHistoryCards(epoch, tbody, opts, columnCount) {
      const layout = await apiJSON('/api/v1/job-history/layout?offset=0&limit=' + String(HISTORY_CARD_WINDOW));
      if (epoch !== jobsRenderEpoch) return null;
      const cards = Array.isArray(layout.cards) ? layout.cards : [];
      const layoutSig = historyLayoutSignature(cards);
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

    async function refreshJobs() {
      const epoch = ++jobsRenderEpoch;
      const queuedBody = document.getElementById('queuedJobsBody');
      const historyBody = document.getElementById('historyJobsBody');

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
            await showAlertDialog({ title: 'Remove failed', message: 'Remove failed: ' + e.message });
          }
        }
      };
      const historyOpts = {
        includeDuration: true,
        fixedLines: 2,
        backPath: window.location.pathname || '/',
        linkClass: 'job-link',
        projectIconURL: projectIconURLForJob
      };

      const [queuedSig, historySig] = await Promise.all([
        refreshQueueCards(epoch, queuedBody, queuedOpts, 8),
        refreshHistoryCards(epoch, historyBody, historyOpts, 7),
      ]);
      if (epoch !== jobsRenderEpoch || queuedSig === null || historySig === null) return;
      if (queuedSig !== lastQueuedJobsSignature) {
        lastQueuedJobsSignature = queuedSig;
      }
      if (historySig !== lastHistoryJobsSignature) {
        lastHistoryJobsSignature = historySig;
      }
    }

    document.getElementById('clearQueueBtn').onclick = async () => {
      const confirmed = await showConfirmDialog({
        title: 'Clear Queue',
        message: 'Clear all queued/leased jobs?',
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

    document.getElementById('flushHistoryBtn').onclick = async () => {
      const confirmed = await showConfirmDialog({
        title: 'Flush History',
        message: 'Flush all finished jobs from history?',
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
`
