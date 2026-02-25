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
    '.source-ref-run-grid{display:grid;grid-template-columns:1fr;gap:10px;}',
    '.source-ref-run-agent-status{min-height:18px;font-size:12px;color:#5f6f67;display:flex;align-items:center;gap:8px;}',
    '.source-ref-run-agent-status.loading::before{content:"";width:12px;height:12px;border-radius:50%;border:2px solid #b8d6c6;border-top-color:#2d7255;animation:ciwi-spin .8s linear infinite;}',
    '@keyframes ciwi-spin{to{transform:rotate(360deg);}}',
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
    '    <div class="source-ref-run-grid">',
    '      <div>',
    '        <label class="source-ref-run-label" for="sourceRefRunSelect">Branch</label>',
    '        <select id="sourceRefRunSelect" class="source-ref-run-select"></select>',
    '      </div>',
    '      <div>',
    '        <label class="source-ref-run-label" for="sourceRefRunAgentSelect">Agent</label>',
    '        <select id="sourceRefRunAgentSelect" class="source-ref-run-select"></select>',
    '        <div id="sourceRefRunAgentStatus" class="source-ref-run-agent-status"></div>',
    '      </div>',
    '    </div>',
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
  const agentSelectEl = document.getElementById('sourceRefRunAgentSelect');
  const agentStatusEl = document.getElementById('sourceRefRunAgentStatus');
  const closeBtn = document.getElementById('sourceRefRunCloseBtn');
  const cancelBtn = document.getElementById('sourceRefRunCancelBtn');
  const confirmBtn = document.getElementById('sourceRefRunConfirmBtn');
  if (!titleEl || !subtitleEl || !noteEl || !selectEl || !agentSelectEl || !agentStatusEl || !closeBtn || !cancelBtn || !confirmBtn) {
    return Promise.reject(new Error('source ref modal elements unavailable'));
  }
  titleEl.textContent = title;
  subtitleEl.textContent = subtitle;
  noteEl.textContent = 'Loading branches...';
  selectEl.innerHTML = '';
  agentSelectEl.innerHTML = '';
  agentStatusEl.textContent = '';
  agentStatusEl.classList.remove('loading');
  selectEl.disabled = true;
  agentSelectEl.disabled = true;
  confirmBtn.textContent = runLabel;
  confirmBtn.disabled = true;
  closeBtn.disabled = true;
  cancelBtn.disabled = true;

  return new Promise((resolve, reject) => {
    let settled = false;
    let eligibleReqSeq = 0;
    let eligibleFetchInFlight = false;
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
      const sourceRef = String(selectEl.value || '').trim();
      if (!sourceRef) return;
      const agentID = String(agentSelectEl.value || '').trim();
      settle({ sourceRef, agentID });
    };
    const setAgentStatus = (text, loading) => {
      agentStatusEl.textContent = String(text || '').trim();
      if (loading) {
        agentStatusEl.classList.add('loading');
      } else {
        agentStatusEl.classList.remove('loading');
      }
    };
    const setRunEnabled = () => {
      confirmBtn.disabled = !String(selectEl.value || '').trim();
    };
    const refreshEligibleAgents = () => {
      const eligibleAgentsPath = String(options.eligibleAgentsPath || '').trim();
      agentSelectEl.innerHTML = '';
      const anyOpt = document.createElement('option');
      anyOpt.value = '';
      anyOpt.textContent = 'Any eligible agent';
      agentSelectEl.appendChild(anyOpt);
      agentSelectEl.disabled = false;
      setRunEnabled();
      if (!eligibleAgentsPath) {
        setAgentStatus('Using default lease matching.', false);
        return Promise.resolve();
      }
      const reqPayload = { ...((options.payload || {})) };
      const sourceRef = String(selectEl.value || '').trim();
      if (sourceRef) reqPayload.source_ref = sourceRef;
      const reqID = ++eligibleReqSeq;
      eligibleFetchInFlight = true;
      setAgentStatus('Finding eligible agents...', true);
      return apiJSON(eligibleAgentsPath, { method: 'POST', body: JSON.stringify(reqPayload) })
        .then((agentsResp) => {
          if (settled || reqID !== eligibleReqSeq) return;
          eligibleFetchInFlight = false;
          const ids = Array.isArray((agentsResp || {}).eligible_agent_ids) ? agentsResp.eligible_agent_ids : [];
          ids.forEach((id) => {
            const agentID = String(id || '').trim();
            if (!agentID) return;
            const opt = document.createElement('option');
            opt.value = agentID;
            opt.textContent = agentID;
            agentSelectEl.appendChild(opt);
          });
          if (ids.length > 0) {
            setAgentStatus(String(ids.length) + ' eligible agent(s) found.', false);
          } else {
            setAgentStatus('No specific agent candidates found; Any eligible agent remains available.', false);
          }
          setRunEnabled();
        })
        .catch((err) => {
          if (settled || reqID !== eligibleReqSeq) return;
          eligibleFetchInFlight = false;
          setAgentStatus('Could not load eligible agents; using Any eligible agent.', false);
          setRunEnabled();
          return err;
        });
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
        closeBtn.disabled = false;
        cancelBtn.disabled = false;
        refreshEligibleAgents()
          .then(() => setTimeout(() => selectEl.focus(), 0));
        selectEl.onchange = () => {
          if (eligibleFetchInFlight) {
            setAgentStatus('Refreshing eligible agents...', true);
          }
          setRunEnabled();
          refreshEligibleAgents();
        };
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
      eligibleAgentsPath: options.eligibleAgentsPath || '',
      payload: payload,
      title: options.title || 'Run With Source Ref',
      subtitle: options.subtitle || '',
      runLabel: options.runLabel || 'Run',
    });
    if (!chosen) return { cancelled: true };
    selectedSourceRef = String(chosen.sourceRef || '').trim();
    const selectedAgentID = String(chosen.agentID || '').trim();
    if (selectedSourceRef) payload.source_ref = selectedSourceRef;
    if (selectedAgentID) payload.agent_id = selectedAgentID;
  }
  const resp = await apiJSON(runPath, { method: 'POST', body: JSON.stringify(payload) });
  return { cancelled: false, response: resp, sourceRef: selectedSourceRef };
}

function renderRuntimeStateBanner(state, bannerID) {
  const id = String(bannerID || 'runtimeStateBanner').trim() || 'runtimeStateBanner';
  const node = document.getElementById(id);
  if (!node) return;
  const payload = state || {};
  const mode = String(payload.mode || '').trim();
  if (mode !== 'degraded_offline') {
    node.style.display = 'none';
    node.classList.remove('runtime-banner-warn');
    node.textContent = '';
    return;
  }
  const reasons = Array.isArray(payload.reasons) ? payload.reasons.map(v => String(v || '').trim()).filter(Boolean) : [];
  const bits = [];
  bits.push('Runtime state: degraded_offline');
  bits.push('online=' + String(Number(payload.online_agents || 0)));
  bits.push('stale=' + String(Number(payload.stale_agents || 0)));
  bits.push('offline=' + String(Number(payload.offline_agents || 0)));
  if (reasons.length > 0) bits.push('reasons: ' + reasons.join(', '));
  if (payload.last_agent_seen_utc) bits.push('last_seen: ' + formatTimestamp(payload.last_agent_seen_utc));
  node.textContent = bits.join(' | ');
  node.style.display = 'block';
  node.classList.add('runtime-banner-warn');
}

async function refreshRuntimeStateBanner(bannerID) {
  try {
    const stateResp = await apiJSON('/api/v1/runtime-state');
    renderRuntimeStateBanner(stateResp || {}, bannerID);
  } catch (_) {
    renderRuntimeStateBanner({ mode: 'degraded_offline', reasons: ['runtime state unavailable'] }, bannerID);
  }
}

function ensureDryRunPreviewStyles() {
  if (document.getElementById('__ciwiDryRunPreviewStyles')) return;
  const style = document.createElement('style');
  style.id = '__ciwiDryRunPreviewStyles';
  style.textContent = [
    '.dryrun-preview-modal{--ciwi-modal-width:min(940px,95vw);--ciwi-modal-height:min(84vh,760px);}',
    '.dryrun-preview-body{display:grid;grid-template-rows:auto 1fr;gap:10px;height:100%;min-height:0;}',
    '.dryrun-preview-controls{display:grid;grid-template-columns:1fr 1fr;gap:10px;}',
    '.dryrun-preview-label{font-size:13px;color:#1f2a24;font-weight:600;}',
    '.dryrun-preview-select{width:100%;font-size:13px;border:1px solid #c4ddd0;border-radius:8px;padding:8px;background:#fff;color:#1f2a24;}',
    '.dryrun-preview-check{display:flex;align-items:center;gap:8px;font-size:13px;color:#1f2a24;user-select:none;}',
    '.dryrun-preview-note{font-size:12px;line-height:1.35;color:#6a5726;background:#fff8e8;border:1px solid #ead7a8;border-radius:8px;padding:8px 10px;}',
    '.dryrun-preview-output{margin:0;background:#0f1412;color:#cde7dc;border-radius:8px;border:1px solid #22352d;padding:12px;width:100%;height:100%;overflow:auto;font-size:12px;line-height:1.35;font-family:ui-monospace,SFMono-Regular,Menlo,Monaco,Consolas,\"Liberation Mono\",\"Courier New\",monospace;}',
    '.dryrun-preview-report{display:flex;flex-direction:column;gap:10px;}',
    '.dryrun-preview-section-title{font-size:12px;font-weight:700;color:#9bc4b1;}',
    '.dryrun-preview-kv-table{display:grid;grid-template-columns:180px 1fr;gap:4px 10px;}',
    '.dryrun-preview-kv-table .key{color:#8ea89d;}',
    '.dryrun-preview-list{margin:0;padding-left:18px;}',
    '.dryrun-preview-list li{margin:2px 0;}',
    '.dryrun-preview-empty{color:#8ea89d;}',
    '.dryrun-preview-output details.log-fold{margin:4px 0;border-left:3px solid #365547;background:rgba(54,85,71,0.2);border-radius:4px;padding:4px 8px;}',
    '.dryrun-preview-output details.log-fold > summary{cursor:pointer;color:#9bc4b1;}',
    '.dryrun-preview-output details.log-fold > pre{margin:8px 0 2px;white-space:pre-wrap;color:#b7d3c7;font:inherit;}',
    '.dryrun-preview-job-head{display:flex;flex-wrap:wrap;align-items:center;gap:6px;}',
    '.dryrun-preview-pill{display:inline-flex;align-items:center;border:1px solid #355949;border-radius:999px;padding:1px 8px;font-size:11px;color:#b7d3c7;background:rgba(53,89,73,0.2);}',
    '.dryrun-preview-pill.warn{border-color:#b48a47;color:#ffd68c;background:rgba(180,138,71,0.15);}',
    '.dryrun-preview-steps{margin-top:8px;}',
    '.dryrun-preview-step{margin-left:10px;}',
    '.dryrun-preview-step-meta{margin:6px 0 0;}',
    '.dryrun-preview-step-script{margin:6px 0 0;background:rgba(8,12,10,0.75);border:1px solid #22352d;border-radius:6px;padding:8px;white-space:pre-wrap;color:#cde7dc;font:inherit;}',
    '.dryrun-preview-exec-log{margin-top:10px;border-top:1px solid #22352d;padding-top:8px;}',
    '.dryrun-preview-exec-line{margin:2px 0;color:#9bc4b1;white-space:pre-wrap;}',
    '.dryrun-preview-actions{display:flex;justify-content:flex-end;gap:8px;padding:8px 12px 12px;}',
  ].join('');
  document.head.appendChild(style);
}

function ensureDryRunPreviewModal() {
  let overlay = document.getElementById('__ciwiDryRunPreviewOverlay');
  if (overlay) return overlay;
  ensureModalBaseStyles();
  ensureDryRunPreviewStyles();
  overlay = document.createElement('div');
  overlay.id = '__ciwiDryRunPreviewOverlay';
  overlay.className = 'ciwi-modal-overlay';
  overlay.setAttribute('aria-hidden', 'true');
  overlay.innerHTML = [
    '<div class="ciwi-modal dryrun-preview-modal" role="dialog" aria-modal="true" aria-label="Execution plan">',
    '  <div class="ciwi-modal-head">',
    '    <div>',
    '      <div id="dryRunPreviewTitle" class="ciwi-modal-title">Execution Plan</div>',
    '      <div id="dryRunPreviewSubtitle" class="ciwi-modal-subtitle"></div>',
    '    </div>',
    '    <button type="button" id="dryRunPreviewCloseBtn" class="secondary">Close</button>',
    '  </div>',
    '  <div class="ciwi-modal-body dryrun-preview-body">',
    '    <div class="dryrun-preview-controls">',
    '      <div>',
    '        <label class="dryrun-preview-label" for="dryRunPreviewSourceRef">Branch</label>',
    '        <select id="dryRunPreviewSourceRef" class="dryrun-preview-select"></select>',
    '      </div>',
    '      <div>',
    '        <label class="dryrun-preview-label" for="dryRunPreviewAgent">Agent</label>',
    '        <select id="dryRunPreviewAgent" class="dryrun-preview-select"></select>',
    '      </div>',
    '      <label class="dryrun-preview-check">',
    '        <input id="dryRunPreviewCachedOnly" type="checkbox" />',
    '        <span>offline_cached_only</span>',
    '      </label>',
    '      <div class="dryrun-preview-note">',
    '        Offline execution guardrails: requires a pinned cached source commit. Non-dry execution is blocked when selected jobs contain <code>skip_dry_run</code> steps.',
    '      </div>',
    '    </div>',
    '    <div id="dryRunPreviewOutput" class="dryrun-preview-output">Loading...</div>',
    '  </div>',
    '  <div class="dryrun-preview-actions">',
    '    <button type="button" id="dryRunPreviewExecuteBtn" class="secondary">Execute Offline</button>',
    '    <button type="button" id="dryRunPreviewRunBtn">Refresh</button>',
    '  </div>',
    '</div>',
  ].join('');
  document.body.appendChild(overlay);
  return overlay;
}

function renderExecutionPlanRows(rows) {
  const normalized = Array.isArray(rows) ? rows.filter(Boolean) : [];
  if (!normalized.length) return '';
  return '<div class="dryrun-preview-kv-table">' + normalized.map((row) => {
    const key = Array.isArray(row) ? row[0] : '';
    const val = Array.isArray(row) ? row[1] : '';
    return '<div class="key">' + escapeHtml(String(key || '')) + '</div><div>' + escapeHtml(String(val || '')) + '</div>';
  }).join('') + '</div>';
}

function renderExecutionPlanMap(raw) {
  const obj = raw && typeof raw === 'object' ? raw : {};
  const keys = Object.keys(obj).sort();
  if (!keys.length) return '<span class="dryrun-preview-empty">(none)</span>';
  return '<ul class="dryrun-preview-list">' + keys.map((k) => {
    const value = obj[k];
    const text = value === null || value === undefined ? '' : String(value);
    return '<li><code>' + escapeHtml(k) + '</code>=<code>' + escapeHtml(text) + '</code></li>';
  }).join('') + '</ul>';
}

function renderExecutionPlanStringList(raw) {
  const values = Array.isArray(raw) ? raw.map(v => String(v || '').trim()).filter(Boolean) : [];
  if (!values.length) return '<span class="dryrun-preview-empty">(none)</span>';
  return '<ul class="dryrun-preview-list">' + values.map((v) => '<li><code>' + escapeHtml(v) + '</code></li>').join('') + '</ul>';
}

function renderExecutionPlanVaultSecrets(raw) {
  const secrets = Array.isArray(raw) ? raw : [];
  if (!secrets.length) return '<span class="dryrun-preview-empty">(none)</span>';
  return '<ul class="dryrun-preview-list">' + secrets.map((entry) => {
    const sec = entry || {};
    const name = String(sec.name || '').trim() || '(unnamed)';
    const parts = [];
    if (String(sec.mount || '').trim()) parts.push('mount=' + String(sec.mount).trim());
    if (String(sec.path || '').trim()) parts.push('path=' + String(sec.path).trim());
    if (String(sec.key || '').trim()) parts.push('key=' + String(sec.key).trim());
    if (Number(sec.kv_version || 0) > 0) parts.push('kv_version=' + String(Number(sec.kv_version)));
    const suffix = parts.length ? (' (' + escapeHtml(parts.join(', ')) + ')') : '';
    return '<li><code>' + escapeHtml(name) + '</code>' + suffix + '</li>';
  }).join('') + '</ul>';
}

function renderExecutionPlanStep(step, stepIndex) {
  const data = step || {};
  const index = Number(data.index || (stepIndex + 1));
  const total = Number(data.total || 0);
  const name = String(data.name || '').trim() || ('step ' + String(index));
  const kind = String(data.kind || '').trim() || 'run';
  const skipDryRun = !!data.skip_dry_run || kind === 'dryrun_skip';
  const summary = 'Step ' + String(index) + (total > 0 ? ('/' + String(total)) : '') + ': ' + escapeHtml(name) +
    ' <span class="dryrun-preview-pill">' + escapeHtml(kind) + '</span>' +
    (skipDryRun ? ' <span class="dryrun-preview-pill warn">skip_dry_run</span>' : '');
  const rows = [
    ['kind', kind],
    ['skip_dry_run', skipDryRun ? 'true' : 'false'],
  ];
  const testName = String(data.test_name || '').trim();
  const testFormat = String(data.test_format || '').trim();
  const testReport = String(data.test_report || '').trim();
  const coverageFormat = String(data.coverage_format || '').trim();
  const coverageReport = String(data.coverage_report || '').trim();
  const vaultConnection = String(data.vault_connection || '').trim();
  if (testName) rows.push(['test_name', testName]);
  if (testFormat) rows.push(['test_format', testFormat]);
  if (testReport) rows.push(['test_report', testReport]);
  if (coverageFormat) rows.push(['coverage_format', coverageFormat]);
  if (coverageReport) rows.push(['coverage_report', coverageReport]);
  if (vaultConnection) rows.push(['vault_connection', vaultConnection]);
  const script = String(data.script || '');
  const scriptHTML = script
    ? ('<div class="dryrun-preview-section-title">Script</div><pre class="dryrun-preview-step-script">' + escapeHtml(script) + '</pre>')
    : '<div class="dryrun-preview-section-title">Script</div><div class="dryrun-preview-empty">(none)</div>';
  return '<details class="log-fold dryrun-preview-step"' + (stepIndex === 0 ? ' open' : '') + '>' +
    '<summary>' + summary + '</summary>' +
    '<div class="dryrun-preview-step-meta">' + renderExecutionPlanRows(rows) + '</div>' +
    scriptHTML +
    '<div class="dryrun-preview-section-title">Env</div>' + renderExecutionPlanMap(data.env) +
    '<div class="dryrun-preview-section-title">Vault Secrets</div>' + renderExecutionPlanVaultSecrets(data.vault_secrets) +
    '</details>';
}

function renderExecutionPlanJob(job, index) {
  const data = job || {};
  const pipelineJobID = String(data.pipeline_job_id || '').trim();
  const matrixName = String(data.matrix_name || '').trim();
  const title = '[' + String(index + 1) + '] ' + (pipelineJobID || '(unknown job)') + (matrixName ? (' / ' + matrixName) : '');
  const stepPlan = Array.isArray(data.step_plan) ? data.step_plan : [];
  const stepCount = Number(data.step_count || stepPlan.length || 0);
  const rows = [
    ['step_count', String(stepCount)],
    ['dependency_blocked', String(!!data.dependency_blocked)],
  ];
  const sourceRepo = String(data.source_repo || '').trim();
  const sourceRef = String(data.source_ref || '').trim();
  if (sourceRepo) rows.push(['source_repo', sourceRepo]);
  if (sourceRef) rows.push(['source_ref', sourceRef]);
  const capsHTML = renderExecutionPlanMap(data.required_capabilities);
  const artifactHTML = renderExecutionPlanStringList(data.artifact_globs);
  const stepsHTML = stepPlan.length
    ? stepPlan.map((step, stepIdx) => renderExecutionPlanStep(step, stepIdx)).join('')
    : '<div class="dryrun-preview-empty">No step plan details available.</div>';
  return '<details class="log-fold dryrun-preview-job"' + (index === 0 ? ' open' : '') + '>' +
    '<summary><span class="dryrun-preview-job-head"><span>' + escapeHtml(title) + '</span>' +
    '<span class="dryrun-preview-pill">steps=' + escapeHtml(String(stepCount)) + '</span>' +
    (data.dependency_blocked ? '<span class="dryrun-preview-pill warn">dependency_blocked</span>' : '') +
    '</span></summary>' +
    renderExecutionPlanRows(rows) +
    '<div class="dryrun-preview-section-title">Required Capabilities</div>' + capsHTML +
    '<div class="dryrun-preview-section-title">Artifact Globs</div>' + artifactHTML +
    '<div class="dryrun-preview-section-title">Steps</div>' +
    '<div class="dryrun-preview-steps">' + stepsHTML + '</div>' +
    '</details>';
}

function formatDryRunPreviewOutput(resp) {
  const data = resp || {};
  const jobs = Array.isArray(data.pending_jobs) ? data.pending_jobs : [];
  const warnings = Array.isArray(data.warnings) ? data.warnings.map(v => String(v || '').trim()).filter(Boolean) : [];
  const eligible = Array.isArray(data.eligible_agent_ids) ? data.eligible_agent_ids.map(v => String(v || '').trim()).filter(Boolean) : [];
  const overviewRows = [
    ['mode', String(data.mode || '')],
    ['offline_cached_only', String(!!data.offline_cached_only)],
    ['cache_used', String(!!data.cache_used)],
    ['cache_source', String(data.cache_source || '')],
    ['pipeline', String(data.pipeline_id || '')],
    ['eligible_agents', eligible.length ? eligible.join(', ') : '(none)'],
    ['pending_jobs', String(jobs.length)],
  ];
  const warningsHTML = warnings.length
    ? ('<div class="dryrun-preview-section-title">Warnings</div><ul class="dryrun-preview-list">' + warnings.map((w) => '<li>' + escapeHtml(w) + '</li>').join('') + '</ul>')
    : '';
  const jobsHTML = jobs.length
    ? jobs.map((job, idx) => renderExecutionPlanJob(job, idx)).join('')
    : '<div class="dryrun-preview-empty">No pending jobs matched this selection.</div>';
  return '<div class="dryrun-preview-report">' +
    '<div class="dryrun-preview-section-title">Overview</div>' +
    renderExecutionPlanRows(overviewRows) +
    warningsHTML +
    '<div class="dryrun-preview-section-title">Pending Jobs</div>' +
    jobsHTML +
    '</div>';
}

function appendExecutionPlanStatus(outputEl, line) {
  if (!outputEl) return;
  let host = outputEl.querySelector('.dryrun-preview-exec-log');
  if (!host) {
    host = document.createElement('div');
    host.className = 'dryrun-preview-exec-log';
    outputEl.appendChild(host);
  }
  const row = document.createElement('div');
  row.className = 'dryrun-preview-exec-line';
  row.textContent = String(line || '');
  host.appendChild(row);
  outputEl.scrollTop = outputEl.scrollHeight;
}

function openDryRunPreviewModal(opts) {
  const options = opts || {};
  const previewPath = String(options.previewPath || '').trim();
  const sourceRefsPath = String(options.sourceRefsPath || '').trim();
  const eligibleAgentsPath = String(options.eligibleAgentsPath || '').trim();
  if (!previewPath || !sourceRefsPath) return Promise.reject(new Error('previewPath and sourceRefsPath are required'));
  const overlay = ensureDryRunPreviewModal();
  const titleEl = document.getElementById('dryRunPreviewTitle');
  const subtitleEl = document.getElementById('dryRunPreviewSubtitle');
  const sourceSel = document.getElementById('dryRunPreviewSourceRef');
  const agentSel = document.getElementById('dryRunPreviewAgent');
  const cachedOnly = document.getElementById('dryRunPreviewCachedOnly');
  const outputEl = document.getElementById('dryRunPreviewOutput');
  const closeBtn = document.getElementById('dryRunPreviewCloseBtn');
  const previewBtn = document.getElementById('dryRunPreviewRunBtn');
  const executeBtn = document.getElementById('dryRunPreviewExecuteBtn');
  if (!titleEl || !subtitleEl || !sourceSel || !agentSel || !cachedOnly || !outputEl || !closeBtn || !previewBtn || !executeBtn) {
    return Promise.reject(new Error('execution plan modal elements unavailable'));
  }
  titleEl.textContent = String(options.title || 'Execution Plan').trim() || 'Execution Plan';
  subtitleEl.textContent = String(options.subtitle || '').trim();
  sourceSel.innerHTML = '';
  agentSel.innerHTML = '';
  outputEl.textContent = 'Loading branches...';
  sourceSel.disabled = true;
  agentSel.disabled = true;
  previewBtn.disabled = true;
  executeBtn.disabled = true;
  executeBtn.style.display = String(options.runPath || '').trim() ? 'inline-block' : 'none';
  cachedOnly.checked = !!options.offlineCachedOnlyDefault;
  const basePayload = { ...((options.payload || {})) };
  const buildPayload = () => {
    const payload = { ...basePayload };
    const ref = String(sourceSel.value || '').trim();
    const agentID = String(agentSel.value || '').trim();
    if (ref) payload.source_ref = ref;
    if (agentID) payload.agent_id = agentID;
    if (cachedOnly.checked) payload.offline_cached_only = true;
    return payload;
  };
  const refreshEligibleAgents = () => {
    agentSel.innerHTML = '';
    const anyOpt = document.createElement('option');
    anyOpt.value = '';
    anyOpt.textContent = 'Any eligible agent';
    agentSel.appendChild(anyOpt);
    if (!eligibleAgentsPath) {
      agentSel.disabled = false;
      previewBtn.disabled = !String(sourceSel.value || '').trim();
      executeBtn.disabled = !String(sourceSel.value || '').trim();
      return Promise.resolve();
    }
    agentSel.disabled = true;
    return apiJSON(eligibleAgentsPath, { method: 'POST', body: JSON.stringify(buildPayload()) })
      .then((data) => {
        const ids = Array.isArray((data || {}).eligible_agent_ids) ? data.eligible_agent_ids : [];
        ids.forEach((id) => {
          const v = String(id || '').trim();
          if (!v) return;
          const opt = document.createElement('option');
          opt.value = v;
          opt.textContent = v;
          agentSel.appendChild(opt);
        });
        agentSel.disabled = false;
        previewBtn.disabled = !String(sourceSel.value || '').trim();
        executeBtn.disabled = !String(sourceSel.value || '').trim();
      });
  };
  const runPreview = () => {
    outputEl.textContent = 'Loading execution plan...';
    previewBtn.disabled = true;
    return apiJSON(previewPath, { method: 'POST', body: JSON.stringify(buildPayload()) })
      .then((resp) => {
        outputEl.innerHTML = formatDryRunPreviewOutput(resp);
      })
      .catch((err) => {
        outputEl.textContent = 'Execution plan failed: ' + String(err && err.message || err);
      })
      .finally(() => {
        previewBtn.disabled = !String(sourceSel.value || '').trim();
        executeBtn.disabled = !String(sourceSel.value || '').trim();
      });
  };
  const runOffline = () => {
    const runPath = String(options.runPath || '').trim();
    if (!runPath) return Promise.resolve();
    const payload = buildPayload();
    payload.execution_mode = 'offline_cached';
    appendExecutionPlanStatus(outputEl, '[execute] enqueueing offline_cached...');
    previewBtn.disabled = true;
    executeBtn.disabled = true;
    return apiJSON(runPath, { method: 'POST', body: JSON.stringify(payload) })
      .then((resp) => {
        const enqueued = Number((resp || {}).enqueued || 0);
        const ids = Array.isArray((resp || {}).job_execution_ids) ? resp.job_execution_ids : [];
        appendExecutionPlanStatus(outputEl, '[execute] enqueued=' + String(enqueued) + (ids.length ? (' ids=' + ids.join(',')) : ''));
        if (typeof showQueuedJobsSnackbar === 'function') {
          showQueuedJobsSnackbar('Offline execution started');
        }
      })
      .catch((err) => {
        appendExecutionPlanStatus(outputEl, '[execute] failed: ' + String(err && err.message || err));
      })
      .finally(() => {
        previewBtn.disabled = !String(sourceSel.value || '').trim();
        executeBtn.disabled = !String(sourceSel.value || '').trim();
      });
  };
  wireModalCloseBehavior(overlay, () => closeModalOverlay(overlay));
  closeBtn.onclick = () => closeModalOverlay(overlay);
  previewBtn.onclick = () => { runPreview(); };
  executeBtn.onclick = () => { runOffline(); };
  sourceSel.onchange = () => {
    refreshEligibleAgents().catch((err) => {
      outputEl.textContent = 'Eligible agent lookup failed: ' + String(err && err.message || err);
    });
  };
  cachedOnly.onchange = () => {
    refreshEligibleAgents().catch((err) => {
      outputEl.textContent = 'Eligible agent lookup failed: ' + String(err && err.message || err);
    });
  };
  openModalOverlay(overlay, 'min(940px,95vw)', 'min(84vh,760px)');
  return apiJSON(sourceRefsPath)
    .then((data) => {
      const refs = Array.isArray((data || {}).refs) ? data.refs : [];
      const defaultRef = String((data || {}).default_ref || '').trim();
      sourceSel.innerHTML = '';
      refs.forEach((entry) => {
        const ref = String((entry || {}).ref || '').trim();
        const name = String((entry || {}).name || '').trim();
        if (!ref) return;
        const opt = document.createElement('option');
        opt.value = ref;
        opt.textContent = name ? (name + ' (' + ref + ')') : ref;
        sourceSel.appendChild(opt);
      });
      if (defaultRef) sourceSel.value = defaultRef;
      if (!String(sourceSel.value || '').trim() && sourceSel.options.length > 0) sourceSel.selectedIndex = 0;
      sourceSel.disabled = false;
      return refreshEligibleAgents();
    })
    .then(() => runPreview())
    .catch((err) => {
      outputEl.textContent = 'Failed to initialize execution plan: ' + String(err && err.message || err);
      previewBtn.disabled = true;
      executeBtn.disabled = true;
      sourceSel.disabled = true;
      agentSel.disabled = true;
    });
}
`
