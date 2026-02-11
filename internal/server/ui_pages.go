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
  const backPath = opts.backPath || (window.location.pathname || '/');
  const onRemove = opts.onRemove || null;
  const linkClass = opts.linkClass || '';

  const tr = document.createElement('tr');
  const pipeline = (job.metadata && job.metadata.pipeline_id) || '';
  const description = jobDescription(job);
  const backTo = encodeURIComponent(backPath);

  tr.innerHTML =
    '<td><a class="' + linkClass + '" href="/jobs/' + encodeURIComponent(job.id) + '?back=' + backTo + '">' + escapeHtml(description) + '</a></td>' +
    '<td class="' + statusClass(job.status) + '">' + escapeHtml(formatJobStatus(job)) + '</td>' +
    '<td>' + escapeHtml(pipeline) + '</td>' +
    '<td>' + escapeHtml(buildVersionLabel(job)) + '</td>' +
    '<td>' + escapeHtml(job.leased_by_agent_id || '') + '</td>' +
    '<td>' + escapeHtml(formatTimestamp(job.created_utc)) + '</td>';

  if (includeReason) {
    const reasons = (job.unmet_requirements || []);
    tr.innerHTML += '<td>' + escapeHtml(reasons.join('; ')) + '</td>';
  }

  if (includeActions) {
    const actionTd = document.createElement('td');
    if (['queued', 'leased'].includes((job.status || '').toLowerCase())) {
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

  return tr;
}

function ensureVersionResolveModal() {
  let modal = document.getElementById('versionResolveModal');
  if (modal) return modal;
  modal = document.createElement('div');
  modal.id = 'versionResolveModal';
  modal.style.cssText = 'display:none;position:fixed;inset:0;background:rgba(12,20,16,.45);z-index:2000;align-items:center;justify-content:center;padding:10px;';
  modal.innerHTML =
    '<div id="versionResolvePanel" style="background:#fff;border:1px solid #c4ddd0;border-radius:12px;width:50vw;height:50vh;display:flex;flex-direction:column;box-shadow:0 12px 36px rgba(21,127,102,.18);overflow:hidden;">' +
      '<div style="padding:12px 14px;border-bottom:1px solid #c4ddd0;display:flex;justify-content:space-between;align-items:center;gap:8px;">' +
        '<div><div id="versionResolveTitle" style="font-size:18px;font-weight:700;">Resolve Upcoming Build Version</div><div id="versionResolveSubtitle" style="font-size:12px;color:#5f6f67;"></div></div>' +
        '<button id="versionResolveCloseBtn" class="secondary" style="padding:6px 10px;">Close</button>' +
      '</div>' +
      '<div style="padding:12px 14px;overflow:hidden;flex:1;display:flex;flex-direction:column;min-height:0;">' +
        '<div id="versionResolveStatus" style="margin-bottom:8px;color:#5f6f67;font-size:13px;"></div>' +
        '<pre id="versionResolveLog" style="margin:0;flex:1 1 auto;min-height:0;background:#0f1b16;color:#d7efe5;border-radius:8px;padding:12px;white-space:pre-wrap;overflow:auto;font-size:12px;line-height:1.45;"></pre>' +
      '</div>' +
    '</div>';
  document.body.appendChild(modal);
  const panel = document.getElementById('versionResolvePanel');
  if (panel) {
    panel.style.minWidth = '760px';
    panel.style.minHeight = '420px';
    panel.style.maxWidth = '96vw';
    panel.style.maxHeight = '96vh';
  }
  document.getElementById('versionResolveCloseBtn').onclick = () => {
    closeVersionResolveStream();
    modal.style.display = 'none';
  };
  if (!window.__ciwiVersionResolveEscBound) {
    document.addEventListener('keydown', (e) => {
      if (e.key === 'Escape') {
        const m = document.getElementById('versionResolveModal');
        if (!m || m.style.display === 'none') return;
        closeVersionResolveStream();
        m.style.display = 'none';
      }
    });
    window.__ciwiVersionResolveEscBound = true;
  }
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
  modal.style.display = 'flex';

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
