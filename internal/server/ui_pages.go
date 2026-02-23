package server

const uiPagesJS = `function apiJSON(path, opts = {}) {
  const baseHeaders = { 'Content-Type': 'application/json' };
  const extraHeaders = (opts && opts.headers) || {};
  const request = {
    ...opts,
    cache: 'no-store',
    headers: { ...baseHeaders, ...extraHeaders },
  };
  return fetch(path, request)
    .then(async (res) => {
      if (!res.ok) {
        const text = await res.text();
        throw new Error(text || ('HTTP ' + res.status));
      }
      return res.json();
    });
}

function formatUnmetRequirementHTML(reason) {
  const text = String(reason || '').trim();
  if (!text) return '';
  let m = text.match(/^missing tool\s+(.+)$/i);
  if (m) return 'Missing tool <code>' + escapeHtml(String(m[1] || '').trim()) + '</code>';
  m = text.match(/^tool\s+(\S+)\s+unavailable$/i);
  if (m) return 'Tool <code>' + escapeHtml(String(m[1] || '').trim()) + '</code> unavailable';
  m = text.match(/^tool\s+(\S+)\s+does not satisfy\s+(.+)$/i);
  if (m) {
    return 'Tool <code>' + escapeHtml(String(m[1] || '').trim()) + '</code> does not satisfy <code>' + escapeHtml(String(m[2] || '').trim()) + '</code>';
  }
  return escapeHtml(text);
}

function formatUnmetRequirementsInlineHTML(reasons) {
  const rows = Array.isArray(reasons) ? reasons : [];
  const htmlRows = rows.map(formatUnmetRequirementHTML).filter(Boolean);
  if (!htmlRows.length) return '';
  return htmlRows.join('; ');
}

function formatUnmetRequirementsTooltipHTML(reasons) {
  const rows = Array.isArray(reasons) ? reasons : [];
  const htmlRows = rows.map(formatUnmetRequirementHTML).filter(Boolean);
  if (!htmlRows.length) return '';
  return '<strong>Missing requirements</strong><div style="margin-top:6px;">' + htmlRows.join('<br />') + '</div>';
}

function buildJobExecutionRow(job, opts = {}) {
  const includeActions = !!opts.includeActions;
  const includeReason = !!opts.includeReason;
  const includeDuration = !!opts.includeDuration;
  const fixedLines = Math.max(0, Number(opts.fixedLines || 0));
  const backPath = opts.backPath || (window.location.pathname || '/');
  const onRemove = opts.onRemove || null;
  const linkClass = opts.linkClass || '';
  const projectIconURLFn = (typeof opts.projectIconURL === 'function') ? opts.projectIconURL : null;

  const tr = document.createElement('tr');
  if (fixedLines > 0) {
    tr.classList.add('ciwi-job-two-line-row');
  }
  const pipeline = (job.metadata && job.metadata.pipeline_id) || '';
  const description = jobDescription(job);
  const backTo = encodeURIComponent(backPath);
  const cellText = (value) => {
    const text = escapeHtml(value || '');
    if (fixedLines <= 0) return text;
    return '<span class="ciwi-job-cell ciwi-job-cell-lines-' + String(fixedLines) + '">' + text + '</span>';
  };
  const linkClasses = fixedLines > 0 ? ((linkClass ? linkClass + ' ' : '') + 'ciwi-job-cell-link') : linkClass;
  const iconURL = projectIconURLFn ? String(projectIconURLFn(job) || '').trim() : '';
  const iconHTML = iconURL
    ? '<img class="ciwi-project-mini-icon" src="' + escapeHtml(iconURL) + '" alt="" onerror="this.style.display=\'none\'" />'
    : '';

  tr.innerHTML =
    '<td><span class="ciwi-job-desc">' + iconHTML + '<a class="' + linkClasses + '" href="/jobs/' + encodeURIComponent(job.id) + '?back=' + backTo + '">' + cellText(description) + '</a></span></td>' +
    '<td class="' + statusClassForJob(job) + '">' + cellText(formatJobStatus(job)) + '</td>' +
    '<td>' + cellText(pipeline) + '</td>' +
    '<td>' + cellText(buildVersionLabel(job)) + '</td>' +
    '<td>' + cellText(job.leased_by_agent_id || '') + '</td>' +
    '<td>' + cellText(formatTimestamp(job.created_utc)) + '</td>';

  if (includeReason) {
    const reasons = (job.unmet_requirements || []);
    const reasonTd = document.createElement('td');
    if (reasons.length === 0) {
      reasonTd.innerHTML = cellText('');
    } else {
      const summaryHTML = formatUnmetRequirementsInlineHTML(reasons);
      reasonTd.innerHTML = '' +
        '<span class="ciwi-job-reason">' +
          '<span class="ciwi-job-reason-summary">' + (fixedLines > 0 ? ('<span class="ciwi-job-cell ciwi-job-cell-lines-' + String(fixedLines) + '">' + summaryHTML + '</span>') : summaryHTML) + '</span>' +
          '<span class="ciwi-job-reason-info" tabindex="0" aria-label="Missing requirements info">ⓘ</span>' +
        '</span>';
      const info = reasonTd.querySelector('.ciwi-job-reason-info');
      if (info && typeof createHoverTooltip === 'function') {
        createHoverTooltip(info, {
          html: formatUnmetRequirementsTooltipHTML(reasons),
          lingerMs: 2000,
          owner: 'queue-reason-' + String(job.id || ''),
        });
      }
    }
    tr.appendChild(reasonTd);
  } else if (includeDuration) {
    tr.innerHTML += '<td>' + cellText(formatJobExecutionDuration(job.started_utc, job.finished_utc, job.status)) + '</td>';
  }

  if (includeActions) {
    const actionTd = document.createElement('td');
    if (fixedLines > 0) {
      actionTd.className = 'ciwi-job-actions-cell';
    }
    if (isPendingJobStatus(job.status)) {
      const btn = document.createElement('button');
      btn.className = 'secondary';
      btn.textContent = 'Remove';
      btn.onclick = async () => {
        btn.disabled = true;
        try {
          if (onRemove) {
            await onRemove(job);
          }
        } finally {
          btn.disabled = false;
        }
      };
      actionTd.appendChild(btn);
    }
    tr.appendChild(actionTd);
  }

  tr.dataset.ciwiRenderKey = jobRowRenderKey(job);
  return tr;
}

function jobRowRenderKey(job) {
  const m = (job && job.metadata) || {};
  const reasons = ((job && job.unmet_requirements) || []).join('|');
  return [
    job && job.id || '',
    job && job.status || '',
    job && job.leased_by_agent_id || '',
    job && job.created_utc || '',
    job && job.started_utc || '',
    job && job.finished_utc || '',
    m.pipeline_id || '',
    m.pipeline_job_id || '',
    m.matrix_name || '',
    m.build_version || '',
    m.build_target || '',
    reasons,
  ].join('\x1f');
}

function ensureJobSkeletonStyles() {
  if (document.getElementById('__ciwiJobSkeletonStyles')) return;
  const style = document.createElement('style');
  style.id = '__ciwiJobSkeletonStyles';
  style.textContent = [
    '@keyframes ciwiSkeletonFade { 0% { opacity: .35; } 50% { opacity: .9; } 100% { opacity: .35; } }',
    '.ciwi-job-two-line-row{--ciwi-row-text-block:2.5em;}',
    '.ciwi-job-two-line-row td{padding-top:6px;padding-bottom:6px;vertical-align:top;}',
    '.ciwi-job-cell-link{display:block;color:inherit;}',
    '.ciwi-job-two-line-row .ciwi-job-cell{display:-webkit-box;-webkit-box-orient:vertical;overflow:hidden;line-height:1.25;min-height:var(--ciwi-row-text-block);max-height:var(--ciwi-row-text-block);}',
    '.ciwi-job-two-line-row .ciwi-project-mini-icon{width:var(--ciwi-row-text-block);height:var(--ciwi-row-text-block);}',
    '.ciwi-job-two-line-row .ciwi-job-cell-lines-2{-webkit-line-clamp:2;}',
    '.ciwi-job-reason{display:flex;align-items:flex-start;gap:8px;}',
    '.ciwi-job-reason-summary code{font-family:ui-monospace,SFMono-Regular,Menlo,Monaco,Consolas,\"Liberation Mono\",\"Courier New\",monospace;background:#eef6f1;border:1px solid #d7e6dd;border-radius:4px;padding:0 4px;font-size:.95em;}',
    '.ciwi-job-reason-info{display:inline-block;line-height:1;cursor:help;user-select:none;}',
    '.ciwi-job-two-line-row .ciwi-job-actions-cell{vertical-align:middle;}',
    '.ciwi-job-skeleton-row td{padding-top:6px;padding-bottom:6px;}',
    '.ciwi-job-skeleton-lines{display:flex;flex-direction:column;gap:8px;}',
    '.ciwi-job-skeleton-bar{height:12px;border-radius:999px;background:#dcebe2;animation:ciwiSkeletonFade 2.2s ease-in-out infinite;}',
    '.ciwi-job-skeleton-bar-short{width:72%;}',
    '.ciwi-job-row-enter{opacity:0;transform:translateY(3px);}',
    '.ciwi-job-row-enter-active{transition:opacity .45s ease,transform .45s ease;opacity:1;transform:translateY(0);}',
  ].join('');
  document.head.appendChild(style);
}

function buildJobSkeletonRow(columnCount) {
  ensureJobSkeletonStyles();
  const tr = document.createElement('tr');
  tr.className = 'ciwi-job-skeleton-row';
  const colspan = Math.max(1, Number(columnCount || 1));
  tr.innerHTML = '<td colspan="' + String(colspan) + '"><div class="ciwi-job-skeleton-lines"><div class="ciwi-job-skeleton-bar"></div><div class="ciwi-job-skeleton-bar ciwi-job-skeleton-bar-short"></div></div></td>';
  return tr;
}

function fadeInJobRow(tr) {
  if (!tr) return;
  tr.classList.add('ciwi-job-row-enter');
  requestAnimationFrame(() => tr.classList.add('ciwi-job-row-enter-active'));
  setTimeout(() => {
    tr.classList.remove('ciwi-job-row-enter');
    tr.classList.remove('ciwi-job-row-enter-active');
  }, 520);
}

function ensureVersionResolveStyles() {
  if (document.getElementById('__ciwiVersionResolveStyles')) return;
  const style = document.createElement('style');
  style.id = '__ciwiVersionResolveStyles';
  style.textContent = [
    '.version-resolve-modal{min-width:760px;min-height:420px;}',
    '.version-resolve-body{display:flex;flex-direction:column;min-height:0;height:100%;}',
    '.version-resolve-status{margin-bottom:8px;color:#5f6f67;font-size:13px;}',
    '.version-resolve-log{margin:0;flex:1 1 auto;min-height:0;background:#0f1b16;color:#d7efe5;border-radius:8px;padding:12px;white-space:pre-wrap;overflow:auto;font-size:12px;line-height:1.45;}',
  ].join('');
  document.head.appendChild(style);
}

function ensureVersionResolveModal() {
  let modal = document.getElementById('versionResolveModal');
  if (modal) return modal;
  ensureModalBaseStyles();
  ensureVersionResolveStyles();
  modal = document.createElement('div');
  modal.id = 'versionResolveModal';
  modal.className = 'ciwi-modal-overlay';
  modal.setAttribute('aria-hidden', 'true');
  modal.innerHTML =
    '<div id="versionResolvePanel" class="ciwi-modal version-resolve-modal">' +
      '<div class="ciwi-modal-head">' +
        '<div><div id="versionResolveTitle" class="ciwi-modal-title">Resolve Upcoming Build Version</div><div id="versionResolveSubtitle" class="ciwi-modal-subtitle"></div></div>' +
        '<button id="versionResolveCloseBtn" class="secondary">Close</button>' +
      '</div>' +
      '<div class="ciwi-modal-body version-resolve-body">' +
        '<div id="versionResolveStatus" class="version-resolve-status"></div>' +
        '<pre id="versionResolveLog" class="version-resolve-log"></pre>' +
      '</div>' +
    '</div>';
  document.body.appendChild(modal);
  const closeVersionModal = () => {
    closeVersionResolveStream();
    closeModalOverlay(modal);
  };
  document.getElementById('versionResolveCloseBtn').onclick = () => {
    closeVersionModal();
  };
  wireModalCloseBehavior(modal, closeVersionModal);
  return modal;
}

let activeVersionResolveSource = null;

function closeVersionResolveStream() {
  if (activeVersionResolveSource) {
    activeVersionResolveSource.close();
    activeVersionResolveSource = null;
  }
}

function openVersionResolveModal(pipelineId, pipelineLabel) {
  const modal = ensureVersionResolveModal();
  closeVersionResolveStream();
  const title = document.getElementById('versionResolveTitle');
  const subtitle = document.getElementById('versionResolveSubtitle');
  const status = document.getElementById('versionResolveStatus');
  const log = document.getElementById('versionResolveLog');
  title.textContent = 'Resolve Upcoming Build Version';
  subtitle.textContent = 'Pipeline: ' + (pipelineLabel || String(pipelineId));
  status.textContent = 'Running...';
  log.textContent = '';
  openModalOverlay(modal, '50vw', '50vh');

  const fmt = (evt) => {
    const step = evt.step || 'step';
    const st = evt.status || '';
    const msg = evt.message || '';
    return '[' + step + '] ' + (st ? st + ': ' : '') + msg;
  };

  const es = new EventSource('/api/v1/pipelines/' + encodeURIComponent(pipelineId) + '/version-resolve');
  activeVersionResolveSource = es;
  es.onmessage = (e) => {
    let evt = {};
    try { evt = JSON.parse(e.data || '{}'); } catch (_) {}
    const line = fmt(evt);
    log.textContent += line + '\n';
    log.scrollTop = log.scrollHeight;
    if (evt.step === 'done') {
      if (evt.status === 'ok') {
        const v = (evt.pipeline_version || '').trim();
        status.textContent = v ? ('Resolved: ' + v) : 'Resolved: n/a';
      } else {
        status.textContent = 'Failed';
      }
      closeVersionResolveStream();
    }
  };
  es.onerror = () => {
    status.textContent = 'Connection closed';
    closeVersionResolveStream();
  };
}

function ensureSourceRefRunStyles() {
  if (document.getElementById('__ciwiSourceRefRunStyles')) return;
  const style = document.createElement('style');
  style.id = '__ciwiSourceRefRunStyles';
  style.textContent = [
    '.source-ref-run-modal{height:auto;max-width:min(560px,92vw);grid-template-rows:auto auto auto;}',
    '.source-ref-run-body{padding:14px 16px 8px;display:flex;flex-direction:column;gap:10px;}',
    '.source-ref-run-note{font-size:13px;color:#5f6f67;}',
    '.source-ref-run-label{font-size:13px;color:#1f2a24;font-weight:600;}',
    '.source-ref-run-select{width:100%;font-size:13px;border:1px solid #c4ddd0;border-radius:8px;padding:8px;background:#fff;color:#1f2a24;}',
    '.source-ref-run-actions{padding:8px 16px 14px;display:flex;justify-content:flex-end;gap:8px;}',
  ].join('');
  document.head.appendChild(style);
}

function ensureSourceRefRunModal() {
  let overlay = document.getElementById('__ciwiSourceRefRunOverlay');
  if (overlay) return overlay;
  ensureModalBaseStyles();
  ensureSourceRefRunStyles();
  overlay = document.createElement('div');
  overlay.id = '__ciwiSourceRefRunOverlay';
  overlay.className = 'ciwi-modal-overlay';
  overlay.setAttribute('aria-hidden', 'true');
  overlay.innerHTML = [
    '<div class="ciwi-modal source-ref-run-modal" role="dialog" aria-modal="true" aria-label="Run with source ref">',
    '  <div class="ciwi-modal-head">',
    '    <div>',
    '      <div id="sourceRefRunTitle" class="ciwi-modal-title">Run With Source Ref</div>',
    '      <div id="sourceRefRunSubtitle" class="ciwi-modal-subtitle"></div>',
    '    </div>',
    '    <button type="button" id="sourceRefRunCloseBtn" class="secondary">Cancel</button>',
    '  </div>',
    '  <div class="source-ref-run-body">',
    '    <div id="sourceRefRunNote" class="source-ref-run-note">Loading branches...</div>',
    '    <label class="source-ref-run-label" for="sourceRefRunSelect">Branch</label>',
    '    <select id="sourceRefRunSelect" class="source-ref-run-select"></select>',
    '  </div>',
    '  <div class="source-ref-run-actions">',
    '    <button type="button" id="sourceRefRunCancelBtn" class="secondary">Cancel</button>',
    '    <button type="button" id="sourceRefRunConfirmBtn">Run</button>',
    '  </div>',
    '</div>',
  ].join('');
  document.body.appendChild(overlay);
  return overlay;
}

function openSourceRefRunDialog(opts) {
  const options = opts || {};
  const sourceRefsPath = String(options.sourceRefsPath || '').trim();
  if (!sourceRefsPath) return Promise.reject(new Error('sourceRefsPath is required'));
  const title = String(options.title || 'Run With Source Ref').trim() || 'Run With Source Ref';
  const subtitle = String(options.subtitle || '').trim();
  const runLabel = String(options.runLabel || 'Run').trim() || 'Run';
  const overlay = ensureSourceRefRunModal();
  const titleEl = document.getElementById('sourceRefRunTitle');
  const subtitleEl = document.getElementById('sourceRefRunSubtitle');
  const noteEl = document.getElementById('sourceRefRunNote');
  const selectEl = document.getElementById('sourceRefRunSelect');
  const closeBtn = document.getElementById('sourceRefRunCloseBtn');
  const cancelBtn = document.getElementById('sourceRefRunCancelBtn');
  const confirmBtn = document.getElementById('sourceRefRunConfirmBtn');
  if (!titleEl || !subtitleEl || !noteEl || !selectEl || !closeBtn || !cancelBtn || !confirmBtn) {
    return Promise.reject(new Error('source ref modal elements unavailable'));
  }
  titleEl.textContent = title;
  subtitleEl.textContent = subtitle;
  noteEl.textContent = 'Loading branches...';
  selectEl.innerHTML = '';
  selectEl.disabled = true;
  confirmBtn.textContent = runLabel;
  confirmBtn.disabled = true;
  closeBtn.disabled = true;
  cancelBtn.disabled = true;

  return new Promise((resolve, reject) => {
    let settled = false;
    const settle = (value) => {
      if (settled) return;
      settled = true;
      closeBtn.onclick = null;
      cancelBtn.onclick = null;
      confirmBtn.onclick = null;
      closeModalOverlay(overlay);
      resolve(value);
    };
    wireModalCloseBehavior(overlay, () => settle(null));
    closeBtn.onclick = () => settle(null);
    cancelBtn.onclick = () => settle(null);
    confirmBtn.onclick = () => {
      const value = String(selectEl.value || '').trim();
      if (!value) return;
      settle(value);
    };
    openModalOverlay(overlay, '520px', 'auto');
    apiJSON(sourceRefsPath)
      .then((data) => {
        const refs = Array.isArray((data || {}).refs) ? data.refs : [];
        const defaultRef = String((data || {}).default_ref || '').trim();
        if (!refs.length) {
          noteEl.textContent = 'No branches available.';
          return;
        }
        selectEl.innerHTML = '';
        refs.forEach((entry) => {
          const ref = String((entry || {}).ref || '').trim();
          const name = String((entry || {}).name || '').trim();
          if (!ref) return;
          const opt = document.createElement('option');
          opt.value = ref;
          opt.textContent = name ? (name + ' (' + ref + ')') : ref;
          selectEl.appendChild(opt);
        });
        if (defaultRef) selectEl.value = defaultRef;
        if (!String(selectEl.value || '').trim() && selectEl.options.length > 0) {
          selectEl.selectedIndex = 0;
        }
        noteEl.textContent = 'Select a source branch for this one-off run.';
        selectEl.disabled = false;
        confirmBtn.disabled = !String(selectEl.value || '').trim();
        closeBtn.disabled = false;
        cancelBtn.disabled = false;
        selectEl.onchange = () => {
          confirmBtn.disabled = !String(selectEl.value || '').trim();
        };
        setTimeout(() => selectEl.focus(), 0);
      })
      .catch((err) => {
        closeBtn.disabled = false;
        cancelBtn.disabled = false;
        noteEl.textContent = 'Failed to load branches.';
        reject(err);
      });
  });
}

async function runWithOptionalSourceRef(event, opts) {
  const options = opts || {};
  const runPath = String(options.runPath || '').trim();
  if (!runPath) throw new Error('runPath is required');
  const payload = { ...(options.payload || {}) };
  let selectedSourceRef = '';
  if (event && event.shiftKey) {
    const sourceRefsPath = String(options.sourceRefsPath || '').trim();
    if (!sourceRefsPath) throw new Error('sourceRefsPath is required for shift-run');
    const chosen = await openSourceRefRunDialog({
      sourceRefsPath,
      title: options.title || 'Run With Source Ref',
      subtitle: options.subtitle || '',
      runLabel: options.runLabel || 'Run',
    });
    if (!chosen) return { cancelled: true };
    selectedSourceRef = String(chosen).trim();
    if (selectedSourceRef) payload.source_ref = selectedSourceRef;
  }
  const resp = await apiJSON(runPath, { method: 'POST', body: JSON.stringify(payload) });
  return { cancelled: false, response: resp, sourceRef: selectedSourceRef };
}
`
