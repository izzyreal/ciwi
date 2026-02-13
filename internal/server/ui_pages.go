package server

const uiPagesJS = `function apiJSON(path, opts = {}) {
  return fetch(path, { headers: { 'Content-Type': 'application/json' }, ...opts })
    .then(async (res) => {
      if (!res.ok) {
        const text = await res.text();
        throw new Error(text || ('HTTP ' + res.status));
      }
      return res.json();
    });
}

function buildJobExecutionRow(job, opts = {}) {
  const includeActions = !!opts.includeActions;
  const includeReason = !!opts.includeReason;
  const fixedLines = Math.max(0, Number(opts.fixedLines || 0));
  const backPath = opts.backPath || (window.location.pathname || '/');
  const onRemove = opts.onRemove || null;
  const linkClass = opts.linkClass || '';

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

  tr.innerHTML =
    '<td><a class="' + linkClasses + '" href="/jobs/' + encodeURIComponent(job.id) + '?back=' + backTo + '">' + cellText(description) + '</a></td>' +
    '<td class="' + statusClass(job.status) + '">' + cellText(formatJobStatus(job)) + '</td>' +
    '<td>' + cellText(pipeline) + '</td>' +
    '<td>' + cellText(buildVersionLabel(job)) + '</td>' +
    '<td>' + cellText(job.leased_by_agent_id || '') + '</td>' +
    '<td>' + cellText(formatTimestamp(job.created_utc)) + '</td>';

  if (includeReason) {
    const reasons = (job.unmet_requirements || []);
    tr.innerHTML += '<td>' + cellText(reasons.join('; ')) + '</td>';
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
    '.ciwi-job-two-line-row td{padding-top:6px;padding-bottom:6px;vertical-align:top;}',
    '.ciwi-job-cell-link{display:block;color:inherit;}',
    '.ciwi-job-two-line-row .ciwi-job-cell{display:-webkit-box;-webkit-box-orient:vertical;overflow:hidden;line-height:1.25;min-height:2.5em;max-height:2.5em;}',
    '.ciwi-job-two-line-row .ciwi-job-cell-lines-2{-webkit-line-clamp:2;}',
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
`
