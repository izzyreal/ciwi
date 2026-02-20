package server

const uiSharedJS = `function escapeHtml(s) {
  return (s || '').replace(/[&<>"']/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
}

function normalizedJobStatus(status) {
  return String(status || '').trim().toLowerCase();
}

function isPendingJobStatus(status) {
  const normalized = normalizedJobStatus(status);
  return normalized === 'queued' || normalized === 'leased';
}

function isActiveJobStatus(status) {
  const normalized = normalizedJobStatus(status);
  return normalized === 'queued' || normalized === 'leased' || normalized === 'running';
}

function isTerminalJobStatus(status) {
  return isSucceededJobStatus(status) || isFailedJobStatus(status);
}

function isRunningJobStatus(status) {
  return normalizedJobStatus(status) === 'running';
}

function isQueuedJobStatus(status) {
  return normalizedJobStatus(status) === 'queued';
}

function isSucceededJobStatus(status) {
  return normalizedJobStatus(status) === 'succeeded';
}

function isFailedJobStatus(status) {
  return normalizedJobStatus(status) === 'failed';
}

function statusClass(status) {
  return 'status-' + normalizedJobStatus(status);
}

function formatTimestamp(ts) {
  if (!ts) return '';
  const d = new Date(ts);
  if (Number.isNaN(d.getTime())) return ts;
  return d.toLocaleString(undefined, {
    weekday: 'short',
    day: '2-digit',
    month: 'short',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false,
  });
}

function formatDurationMs(ms) {
  const value = Number(ms);
  if (!Number.isFinite(value) || value < 0) return '';
  const totalSec = Math.floor(value / 1000);
  const h = Math.floor(totalSec / 3600);
  const m = Math.floor((totalSec % 3600) / 60);
  const s = totalSec % 60;
  return h > 0
    ? String(h) + 'h ' + String(m).padStart(2, '0') + 'm ' + String(s).padStart(2, '0') + 's'
    : String(m).padStart(2, '0') + 'm ' + String(s).padStart(2, '0') + 's';
}

function formatJobExecutionDuration(startedUTC, finishedUTC, status) {
  const startedRaw = String(startedUTC || '').trim();
  if (!startedRaw) return '';
  const started = new Date(startedRaw);
  if (Number.isNaN(started.getTime())) return '';

  const finishedRaw = String(finishedUTC || '').trim();
  const finished = finishedRaw ? new Date(finishedRaw) : null;
  const hasFinished = finished && !Number.isNaN(finished.getTime());
  const running = isRunningJobStatus(status);
  if (!hasFinished && !running) return '';

  const endMs = hasFinished ? finished.getTime() : Date.now();
  const duration = formatDurationMs(Math.max(0, endMs - started.getTime()));
  if (!duration) return '';
  return running && !hasFinished ? (duration + ' (running)') : duration;
}

function jobDescription(job) {
  const m = job.metadata || {};
  if (String(m.adhoc || '').trim() === '1') return 'Adhoc script';
  const matrix = (m.matrix_name || '').trim();
  const pipelineJob = (m.pipeline_job_id || '').trim();
  const pipeline = (m.pipeline_id || '').trim();
  if (matrix && pipelineJob) return pipelineJob + ' / ' + matrix;
  if (matrix) return matrix;
  if (pipelineJob && pipeline) return pipeline + ' / ' + pipelineJob;
  if (pipelineJob) return pipelineJob;
  if (pipeline) return pipeline;
  return 'Job Execution';
}

function buildVersionLabel(job) {
  const m = (job && job.metadata) || {};
  const version = (m.build_version || '').trim();
  if (!version) return '';
  const target = (m.build_target || '').trim();
  return target ? (version + ' (' + target + ')') : version;
}

function formatJobStatus(job) {
  const status = (job && job.status) || '';
  const errText = String((job && job.error) || '').trim().toLowerCase();
  if (normalizedJobStatus(status) === 'failed' && errText === 'cancelled by user') {
    return 'Cancelled by user';
  }
  const summary = job && job.test_summary;
  if (!summary || !summary.total) return status;
  if (summary.failed > 0) return status + ' (' + summary.passed + '/' + summary.total + ' passed)';
  return status + ' (' + summary.passed + '/' + summary.total + ' passed)';
}

function formatBytes(n) {
  const value = Number(n || 0);
  if (!Number.isFinite(value) || value < 0) return '0 B';
  if (value < 1024) return String(Math.round(value)) + ' B';
  const units = ['KB', 'MB', 'GB', 'TB'];
  let size = value / 1024;
  let idx = 0;
  while (size >= 1024 && idx < units.length - 1) {
    size /= 1024;
    idx++;
  }
  const rounded = size >= 10 ? size.toFixed(1) : size.toFixed(2);
  return rounded.replace(/\.00$/, '').replace(/(\.\d)0$/, '$1') + ' ' + units[idx];
}

function createRefreshGuard(holdMs) {
  const pauseMs = Math.max(0, Number(holdMs || 5000));
  let pausedUntil = 0;

  function hasActiveTextSelection() {
    const sel = window.getSelection && window.getSelection();
    if (!sel) return false;
    const text = (sel.toString() || '').trim();
    return text.length > 0;
  }

  return {
    shouldPause: function() {
      return Date.now() < pausedUntil;
    },
    bindSelectionListener: function() {
      document.addEventListener('selectionchange', () => {
        if (hasActiveTextSelection()) {
          pausedUntil = Date.now() + pauseMs;
        }
      });
    },
  };
}

function statusForLastSeen(ts) {
  if (!ts) return { label: 'unknown', cls: 'offline' };
  const d = new Date(ts);
  if (isNaN(d.getTime())) return { label: 'unknown', cls: 'offline' };
  const ageMs = Date.now() - d.getTime();
  if (ageMs <= 20000) return { label: 'online', cls: 'ok' };
  if (ageMs <= 60000) return { label: 'stale', cls: 'stale' };
  return { label: 'offline', cls: 'offline' };
}

function formatCapabilities(caps) {
  if (!caps) return '';
  const entries = Object.entries(caps);
  if (entries.length === 0) return '';
  return entries.map(([k,v]) => k + '=' + v).join(', ');
}

function ensureModalBaseStyles() {
  if (document.getElementById('__ciwiModalBaseStyles')) return;
  const style = document.createElement('style');
  style.id = '__ciwiModalBaseStyles';
  style.textContent = [
    '.ciwi-modal-overlay{position:fixed;inset:0;background:rgba(10,27,20,.45);display:none;align-items:center;justify-content:center;z-index:2000;padding:12px;}',
    '.ciwi-modal{--ciwi-modal-width:70vw;--ciwi-modal-height:70vh;width:var(--ciwi-modal-width);height:var(--ciwi-modal-height);background:#fff;border:1px solid #c4ddd0;border-radius:12px;box-shadow:0 24px 56px rgba(15,31,24,.24);display:grid;grid-template-rows:auto 1fr;overflow:hidden;max-width:96vw;max-height:96vh;}',
    '.ciwi-modal-head{display:flex;align-items:center;justify-content:space-between;gap:8px;border-bottom:1px solid #c4ddd0;padding:12px;background:#f7fcf9;}',
    '.ciwi-modal-title{font-size:18px;font-weight:700;}',
    '.ciwi-modal-subtitle{font-size:12px;color:#5f6f67;}',
    '.ciwi-modal-body{padding:12px;overflow:hidden;min-height:0;}',
  ].join('');
  document.head.appendChild(style);
}

function openModalOverlay(overlay, width, height) {
  if (!overlay) return;
  ensureModalBaseStyles();
  const panel = overlay.querySelector('.ciwi-modal');
  if (panel) {
    if (width) panel.style.setProperty('--ciwi-modal-width', width);
    if (height) panel.style.setProperty('--ciwi-modal-height', height);
  }
  overlay.style.display = 'flex';
  overlay.setAttribute('aria-hidden', 'false');
}

function closeModalOverlay(overlay) {
  if (!overlay) return;
  overlay.style.display = 'none';
  overlay.setAttribute('aria-hidden', 'true');
}

function wireModalCloseBehavior(overlay, onClose) {
  if (!overlay) return;
  if (typeof onClose === 'function') {
    overlay.__ciwiModalOnClose = onClose;
  } else {
    overlay.__ciwiModalOnClose = null;
  }
  if (overlay.__ciwiModalCloseBound) return;
  ensureModalBaseStyles();
  if (overlay.getAttribute('aria-hidden') !== 'false') {
    overlay.style.display = 'none';
    overlay.setAttribute('aria-hidden', 'true');
  }
  overlay.__ciwiModalCloseBound = true;
  let pointerDownOnOverlay = false;
  function hasActiveTextSelection() {
    const sel = window.getSelection && window.getSelection();
    if (!sel) return false;
    const text = String(sel.toString() || '').trim();
    return text.length > 0;
  }
  overlay.addEventListener('mousedown', (ev) => {
    pointerDownOnOverlay = (ev.target === overlay);
  });
  overlay.addEventListener('click', (ev) => {
    if (ev.target !== overlay) return;
    if (!pointerDownOnOverlay) return;
    if (hasActiveTextSelection()) return;
    const closeFn = overlay.__ciwiModalOnClose;
    if (typeof closeFn === 'function') closeFn(); else closeModalOverlay(overlay);
  });
  document.addEventListener('mouseup', () => {
    pointerDownOnOverlay = false;
  });
  document.addEventListener('keydown', (ev) => {
    if (ev.key !== 'Escape') return;
    if (overlay.style.display !== 'flex') return;
    const closeFn = overlay.__ciwiModalOnClose;
    if (typeof closeFn === 'function') closeFn(); else closeModalOverlay(overlay);
  });
}

function ensureConfirmDialogStyles() {
  if (document.getElementById('__ciwiConfirmDialogStyles')) return;
  const style = document.createElement('style');
  style.id = '__ciwiConfirmDialogStyles';
  style.textContent = [
    '.ciwi-confirm-modal{height:auto;grid-template-rows:auto auto auto;max-width:min(520px,92vw);}',
    '.ciwi-confirm-body{padding:14px 16px 6px;color:#1f2a24;font-size:14px;line-height:1.4;overflow-wrap:anywhere;word-break:break-word;}',
    '.ciwi-confirm-actions{padding:8px 16px 14px;display:flex;gap:8px;justify-content:flex-end;flex-wrap:wrap;}',
    '.ciwi-confirm-actions .secondary{background:#fff;color:#27473b;border:1px solid #c4ddd0;}',
  ].join('');
  document.head.appendChild(style);
}

function ensureConfirmDialog() {
  ensureModalBaseStyles();
  ensureConfirmDialogStyles();
  let overlay = document.getElementById('__ciwiConfirmOverlay');
  if (overlay) return overlay;
  overlay = document.createElement('div');
  overlay.id = '__ciwiConfirmOverlay';
  overlay.className = 'ciwi-modal-overlay';
  overlay.setAttribute('aria-hidden', 'true');
  overlay.innerHTML = [
    '<div class="ciwi-modal ciwi-confirm-modal" role="dialog" aria-modal="true" aria-label="Confirm action">',
    '  <div class="ciwi-modal-head">',
    '    <div style="font-weight:700;" id="__ciwiConfirmTitle">Confirm</div>',
    '  </div>',
    '  <div class="ciwi-confirm-body" id="__ciwiConfirmMessage"></div>',
    '  <div class="ciwi-confirm-actions">',
    '    <button type="button" id="__ciwiConfirmCancel" class="secondary">Cancel</button>',
    '    <button type="button" id="__ciwiConfirmOk">OK</button>',
    '  </div>',
    '</div>',
  ].join('');
  document.body.appendChild(overlay);
  return overlay;
}

function showConfirmDialog(opts) {
  const options = opts || {};
  const message = String(options.message || '').trim();
  if (!message) return Promise.resolve(false);
  const title = String(options.title || 'Confirm').trim() || 'Confirm';
  const okLabel = String(options.okLabel || 'OK').trim() || 'OK';
  const cancelLabel = String(options.cancelLabel || 'Cancel').trim() || 'Cancel';
  const overlay = ensureConfirmDialog();
  const titleEl = document.getElementById('__ciwiConfirmTitle');
  const msgEl = document.getElementById('__ciwiConfirmMessage');
  const okBtn = document.getElementById('__ciwiConfirmOk');
  const cancelBtn = document.getElementById('__ciwiConfirmCancel');
  if (!titleEl || !msgEl || !okBtn || !cancelBtn) return Promise.resolve(false);

  titleEl.textContent = title;
  msgEl.textContent = message;
  okBtn.textContent = okLabel;
  cancelBtn.textContent = cancelLabel;
  okBtn.disabled = false;
  cancelBtn.disabled = false;

  return new Promise((resolve) => {
    let settled = false;
    const settle = (value) => {
      if (settled) return;
      settled = true;
      okBtn.onclick = null;
      cancelBtn.onclick = null;
      closeModalOverlay(overlay);
      resolve(!!value);
    };
    wireModalCloseBehavior(overlay, () => settle(false));
    okBtn.onclick = () => settle(true);
    cancelBtn.onclick = () => settle(false);
    openModalOverlay(overlay, '460px', 'auto');
    setTimeout(() => okBtn.focus(), 0);
  });
}

function ensureAlertDialog() {
  ensureModalBaseStyles();
  ensureConfirmDialogStyles();
  let overlay = document.getElementById('__ciwiAlertOverlay');
  if (overlay) return overlay;
  overlay = document.createElement('div');
  overlay.id = '__ciwiAlertOverlay';
  overlay.className = 'ciwi-modal-overlay';
  overlay.setAttribute('aria-hidden', 'true');
  overlay.innerHTML = [
    '<div class="ciwi-modal ciwi-confirm-modal" role="dialog" aria-modal="true" aria-label="Message">',
    '  <div class="ciwi-modal-head">',
    '    <div style="font-weight:700;" id="__ciwiAlertTitle">Message</div>',
    '  </div>',
    '  <div class="ciwi-confirm-body" id="__ciwiAlertMessage"></div>',
    '  <div class="ciwi-confirm-actions">',
    '    <button type="button" id="__ciwiAlertOk">OK</button>',
    '  </div>',
    '</div>',
  ].join('');
  document.body.appendChild(overlay);
  return overlay;
}

function showAlertDialog(opts) {
  const options = opts || {};
  const message = String(options.message || '').trim();
  if (!message) return Promise.resolve();
  const title = String(options.title || 'Message').trim() || 'Message';
  const okLabel = String(options.okLabel || 'OK').trim() || 'OK';
  const overlay = ensureAlertDialog();
  const titleEl = document.getElementById('__ciwiAlertTitle');
  const msgEl = document.getElementById('__ciwiAlertMessage');
  const okBtn = document.getElementById('__ciwiAlertOk');
  if (!titleEl || !msgEl || !okBtn) return Promise.resolve();

  titleEl.textContent = title;
  msgEl.textContent = message;
  okBtn.textContent = okLabel;
  okBtn.disabled = false;

  return new Promise((resolve) => {
    let settled = false;
    const settle = () => {
      if (settled) return;
      settled = true;
      okBtn.onclick = null;
      closeModalOverlay(overlay);
      resolve();
    };
    wireModalCloseBehavior(overlay, settle);
    okBtn.onclick = settle;
    openModalOverlay(overlay, '460px', 'auto');
    setTimeout(() => okBtn.focus(), 0);
  });
}

function ensureSnackbarStyles() {
  if (document.getElementById('__ciwiSnackbarStyles')) return;
  const style = document.createElement('style');
  style.id = '__ciwiSnackbarStyles';
  style.textContent = [
    '#ciwiSnackbarHost{position:fixed;right:14px;bottom:14px;z-index:2500;display:flex;flex-direction:column;gap:10px;max-width:min(480px,92vw);pointer-events:none;}',
    '.ciwi-snackbar{pointer-events:auto;display:flex;align-items:center;justify-content:space-between;gap:10px;background:#173326;color:#eaf7ef;border:1px solid #2f5c46;border-radius:10px;padding:10px 12px;box-shadow:0 16px 32px rgba(8,20,15,.35);}',
    '.ciwi-snackbar-msg{font-size:13px;line-height:1.25;word-break:break-word;}',
    '.ciwi-snackbar-actions{display:flex;gap:8px;align-items:center;flex-wrap:wrap;}',
    '.ciwi-snackbar-btn{font:inherit;font-size:12px;font-weight:600;padding:6px 8px;border-radius:7px;border:1px solid #98c8ad;background:#e7f7ed;color:#163325;cursor:pointer;}',
    '.ciwi-snackbar-btn.dismiss{background:transparent;color:#d2e7da;border-color:#5e8672;}',
  ].join('');
  document.head.appendChild(style);
}

function snackbarHost() {
  ensureSnackbarStyles();
  let host = document.getElementById('ciwiSnackbarHost');
  if (host) return host;
  host = document.createElement('div');
  host.id = 'ciwiSnackbarHost';
  document.body.appendChild(host);
  return host;
}

function showSnackbar(opts) {
  const options = opts || {};
  const message = String(options.message || '').trim();
  const messageHTML = String(options.messageHTML || '').trim();
  if (!message && !messageHTML) return;
  const host = snackbarHost();
  const item = document.createElement('div');
  item.className = 'ciwi-snackbar';
  const msg = document.createElement('div');
  msg.className = 'ciwi-snackbar-msg';
  if (messageHTML) {
    msg.innerHTML = messageHTML;
  } else {
    msg.textContent = message;
  }
  item.appendChild(msg);

  const actions = document.createElement('div');
  actions.className = 'ciwi-snackbar-actions';
  if (options.actionLabel && typeof options.onAction === 'function') {
    const actionBtn = document.createElement('button');
    actionBtn.type = 'button';
    actionBtn.className = 'ciwi-snackbar-btn';
    actionBtn.textContent = String(options.actionLabel);
    actionBtn.onclick = () => {
      try { options.onAction(); } catch (_) {}
      if (item.parentNode) item.parentNode.removeChild(item);
    };
    actions.appendChild(actionBtn);
  }
  const dismissBtn = document.createElement('button');
  dismissBtn.type = 'button';
  dismissBtn.className = 'ciwi-snackbar-btn dismiss';
  dismissBtn.textContent = 'Dismiss';
  dismissBtn.onclick = () => {
    if (item.parentNode) item.parentNode.removeChild(item);
  };
  actions.appendChild(dismissBtn);
  item.appendChild(actions);
  host.appendChild(item);

  const ttl = Math.max(1500, Number(options.timeoutMs || 8000));
  setTimeout(() => {
    if (item.parentNode) item.parentNode.removeChild(item);
  }, ttl);
}

function showJobStartedSnackbar(message, jobExecutionID) {
  const jobID = String(jobExecutionID || '').trim();
  showSnackbar({
    message: message,
    actionLabel: 'Show job execution',
    onAction: () => {
      if (!jobID) return;
      window.location.href = '/jobs/' + encodeURIComponent(jobID);
    },
  });
}

function showQueuedJobsSnackbar(message) {
  showSnackbar({
    message: message,
    actionLabel: 'Show queued jobs',
    onAction: () => {
      try { sessionStorage.setItem('__ciwiFocusQueuedJobs', '1'); } catch (_) {}
      if ((window.location.pathname || '/') === '/') {
        const node = document.getElementById('queued-jobs');
        if (node && typeof node.scrollIntoView === 'function') {
          window.location.hash = 'queued-jobs';
          node.scrollIntoView({ block: 'start', behavior: 'smooth' });
          return;
        }
      }
      window.location.assign('/#queued-jobs');
    },
  });
}

function ensureHoverTooltipStyles() {
  if (document.getElementById('__ciwiHoverTooltipStyles')) return;
  const style = document.createElement('style');
  style.id = '__ciwiHoverTooltipStyles';
  style.textContent = [
    '.ciwi-hover-tooltip{position:fixed;z-index:2600;display:none;max-width:min(560px,88vw);padding:8px 10px;border:1px solid #c4ddd0;border-radius:8px;background:#f8fcfa;color:#1f2a24;font-size:14px;font-weight:400;line-height:1.35;box-shadow:0 6px 18px rgba(26,40,34,.15);}',
    '.ciwi-hover-tooltip code{font-family:ui-monospace,SFMono-Regular,Menlo,Monaco,Consolas,\"Liberation Mono\",\"Courier New\",monospace;background:#eef6f1;border:1px solid #d7e6dd;border-radius:4px;padding:0 4px;font-size:.95em;}',
    '.ciwi-hover-tooltip a{color:#1f5f44;text-decoration:underline;}',
    'body.ciwi-tooltip-selecting *{user-select:none !important;}',
    'body.ciwi-tooltip-selecting .ciwi-hover-tooltip,body.ciwi-tooltip-selecting .ciwi-hover-tooltip *{user-select:text !important;}',
  ].join('');
  document.head.appendChild(style);
}

function createHoverTooltip(anchor, opts) {
  if (!anchor) return null;
  ensureHoverTooltipStyles();
  if (anchor.__ciwiHoverTooltip && typeof anchor.__ciwiHoverTooltip.destroy === 'function') {
    anchor.__ciwiHoverTooltip.destroy();
  }

  const options = opts || {};
  const html = String(options.html || '').trim();
  if (!html) return null;
  const lingerMs = Math.max(0, Number(options.lingerMs || 2000));
  const owner = String(options.owner || '').trim();
  const tip = document.createElement('div');
  tip.className = 'ciwi-hover-tooltip';
  if (owner) tip.setAttribute('data-ciwi-tooltip-owner', owner);
  tip.innerHTML = html;
  document.body.appendChild(tip);

  let hideTimer = null;
  let visible = false;
  let draggingSelection = false;

  function hasSelectionInsideTooltip() {
    const sel = window.getSelection && window.getSelection();
    if (!sel || sel.rangeCount === 0) return false;
    const text = String(sel.toString() || '').trim();
    if (!text) return false;
    const range = sel.getRangeAt(0);
    const node = range.commonAncestorContainer;
    return tip.contains(node.nodeType === 1 ? node : node.parentNode);
  }

  function positionTip() {
    const ar = anchor.getBoundingClientRect();
    const tr = tip.getBoundingClientRect();
    const margin = 8;
    let left = ar.left;
    if ((left + tr.width + margin) > window.innerWidth) {
      left = Math.max(margin, window.innerWidth - tr.width - margin);
    }
    let top = ar.bottom + 8;
    if ((top + tr.height + margin) > window.innerHeight) {
      top = Math.max(margin, ar.top - tr.height - 8);
    }
    tip.style.left = left + 'px';
    tip.style.top = top + 'px';
  }

  function clearHideTimer() {
    if (hideTimer != null) {
      clearTimeout(hideTimer);
      hideTimer = null;
    }
  }

  function showTip() {
    clearHideTimer();
    tip.style.display = 'block';
    tip.classList.add('is-visible');
    visible = true;
    positionTip();
  }

  function hideNow() {
    clearHideTimer();
    tip.style.display = 'none';
    tip.classList.remove('is-visible');
    visible = false;
  }

  function shouldKeepVisible() {
    const anchorHover = !!(anchor.matches && anchor.matches(':hover'));
    const tipHover = !!(tip.matches && tip.matches(':hover'));
    const anchorFocus = document.activeElement === anchor;
    return anchorHover || tipHover || anchorFocus || hasSelectionInsideTooltip();
  }

  function scheduleHide() {
    clearHideTimer();
    hideTimer = setTimeout(function retryHide() {
      if (shouldKeepVisible()) {
        hideTimer = setTimeout(retryHide, 150);
        return;
      }
      hideNow();
    }, lingerMs);
  }

  const onEnter = () => showTip();
  const onLeave = () => scheduleHide();
  const onSelection = () => {
    if (!visible) return;
    if (hasSelectionInsideTooltip()) clearHideTimer();
  };
  const startSelectionDrag = () => {
    draggingSelection = true;
    document.body.classList.add('ciwi-tooltip-selecting');
  };
  const stopSelectionDrag = () => {
    if (!draggingSelection) return;
    draggingSelection = false;
    document.body.classList.remove('ciwi-tooltip-selecting');
  };
  const onReposition = () => {
    if (!visible) return;
    positionTip();
  };

  anchor.addEventListener('mouseenter', onEnter);
  anchor.addEventListener('focus', onEnter);
  anchor.addEventListener('mouseleave', onLeave);
  anchor.addEventListener('blur', onLeave);
  tip.addEventListener('mouseenter', onEnter);
  tip.addEventListener('mouseleave', onLeave);
  tip.addEventListener('mousedown', startSelectionDrag);
  document.addEventListener('mouseup', stopSelectionDrag);
  document.addEventListener('selectionchange', onSelection);
  window.addEventListener('scroll', onReposition, true);
  window.addEventListener('resize', onReposition);

  const controller = {
    isVisible: () => visible,
    destroy: () => {
      hideNow();
      anchor.removeEventListener('mouseenter', onEnter);
      anchor.removeEventListener('focus', onEnter);
      anchor.removeEventListener('mouseleave', onLeave);
      anchor.removeEventListener('blur', onLeave);
      tip.removeEventListener('mouseenter', onEnter);
      tip.removeEventListener('mouseleave', onLeave);
      tip.removeEventListener('mousedown', startSelectionDrag);
      document.removeEventListener('mouseup', stopSelectionDrag);
      document.removeEventListener('selectionchange', onSelection);
      window.removeEventListener('scroll', onReposition, true);
      window.removeEventListener('resize', onReposition);
      stopSelectionDrag();
      if (tip.parentNode) tip.parentNode.removeChild(tip);
      if (anchor.__ciwiHoverTooltip === controller) {
        delete anchor.__ciwiHoverTooltip;
      }
    },
  };
  anchor.__ciwiHoverTooltip = controller;
  return controller;
}

function ensureTextSearchStyles() {
  if (document.getElementById('__ciwiTextSearchStyles')) return;
  const style = document.createElement('style');
  style.id = '__ciwiTextSearchStyles';
  style.textContent = [
    '.ciwi-search-hit{background:#ffe79a;color:#2e2200;padding:0 1px;border-radius:2px;}',
    '.ciwi-search-hit.active{background:#ffc24d;outline:1px solid #946200;}',
  ].join('');
  document.head.appendChild(style);
}

function createTextSearchController(opts) {
  const options = opts || {};
  const scopeEl = options.scopeEl;
  const inputEl = options.inputEl;
  if (!scopeEl || !inputEl) return null;
  ensureTextSearchStyles();

  const prevBtn = options.prevBtn || null;
  const nextBtn = options.nextBtn || null;
  const countEl = options.countEl || null;
  const itemSelector = String(options.itemSelector || '').trim();
  const caseSensitive = !!options.caseSensitive;

  let hits = [];
  let activeIndex = -1;
  let query = '';
  let bound = false;

  function normalized(value) {
    return caseSensitive ? value : value.toLowerCase();
  }

  function updateCount() {
    if (!countEl) return;
    if (!query || hits.length === 0 || activeIndex < 0) {
      countEl.textContent = hits.length > 0 ? ('0/' + hits.length) : '0/0';
      return;
    }
    countEl.textContent = String(activeIndex + 1) + '/' + String(hits.length);
  }

  function updateNavState() {
    const disabled = hits.length === 0;
    if (prevBtn) prevBtn.disabled = disabled;
    if (nextBtn) nextBtn.disabled = disabled;
    updateCount();
  }

  function unwrapMark(mark) {
    if (!mark || !mark.parentNode) return;
    const parent = mark.parentNode;
    parent.replaceChild(document.createTextNode(mark.textContent || ''), mark);
    parent.normalize();
  }

  function clearMarks() {
    hits.forEach(unwrapMark);
    hits = [];
    activeIndex = -1;
    updateNavState();
  }

  function setActive(index, shouldScroll) {
    if (hits.length === 0) {
      activeIndex = -1;
      updateNavState();
      return;
    }
    const next = ((index % hits.length) + hits.length) % hits.length;
    if (activeIndex >= 0 && hits[activeIndex]) {
      hits[activeIndex].classList.remove('active');
    }
    activeIndex = next;
    const current = hits[activeIndex];
    if (current) {
      current.classList.add('active');
      if (shouldScroll) {
        current.scrollIntoView({ block: 'center', inline: 'nearest', behavior: 'smooth' });
      }
    }
    updateNavState();
  }

  function collectSearchRoots() {
    if (!itemSelector) return [scopeEl];
    const roots = Array.from(scopeEl.querySelectorAll(itemSelector));
    return roots.length ? roots : [scopeEl];
  }

  function markInTextNode(node, needle) {
    const text = String(node.nodeValue || '');
    if (!text) return [];
    const haystack = normalized(text);
    if (!haystack.includes(needle)) return [];

    const frag = document.createDocumentFragment();
    const localHits = [];
    let cursor = 0;
    while (cursor < text.length) {
      const pos = haystack.indexOf(needle, cursor);
      if (pos < 0) {
        frag.appendChild(document.createTextNode(text.slice(cursor)));
        break;
      }
      if (pos > cursor) {
        frag.appendChild(document.createTextNode(text.slice(cursor, pos)));
      }
      const mark = document.createElement('mark');
      mark.className = 'ciwi-search-hit';
      mark.textContent = text.slice(pos, pos + needle.length);
      frag.appendChild(mark);
      localHits.push(mark);
      cursor = pos + needle.length;
    }
    if (node.parentNode) {
      node.parentNode.replaceChild(frag, node);
    }
    return localHits;
  }

  function applySearch(rawQuery) {
    query = String(rawQuery || '').trim();
    clearMarks();
    if (!query) {
      updateNavState();
      return;
    }
    const needle = normalized(query);
    collectSearchRoots().forEach(root => {
      const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT, {
        acceptNode: function(node) {
          if (!node || !node.nodeValue || !node.nodeValue.trim()) return NodeFilter.FILTER_REJECT;
          const parent = node.parentElement;
          if (!parent) return NodeFilter.FILTER_REJECT;
          const tag = parent.tagName;
          if (tag === 'SCRIPT' || tag === 'STYLE' || tag === 'NOSCRIPT' || tag === 'MARK') {
            return NodeFilter.FILTER_REJECT;
          }
          return NodeFilter.FILTER_ACCEPT;
        },
      });
      const textNodes = [];
      while (walker.nextNode()) {
        textNodes.push(walker.currentNode);
      }
      textNodes.forEach(node => {
        const newHits = markInTextNode(node, needle);
        if (newHits.length) hits.push.apply(hits, newHits);
      });
    });
    if (hits.length > 0) {
      setActive(0, false);
    } else {
      updateNavState();
    }
  }

  function bind() {
    if (bound) return;
    bound = true;
    inputEl.addEventListener('input', () => applySearch(inputEl.value));
    inputEl.addEventListener('keydown', (ev) => {
      if (ev.key !== 'Enter') return;
      ev.preventDefault();
      if (ev.shiftKey) setActive(activeIndex - 1, true);
      else setActive(activeIndex + 1, true);
    });
    if (prevBtn) {
      prevBtn.addEventListener('click', () => setActive(activeIndex - 1, true));
    }
    if (nextBtn) {
      nextBtn.addEventListener('click', () => setActive(activeIndex + 1, true));
    }
    updateNavState();
  }

  bind();
  applySearch(inputEl.value);
  return {
    refresh: function() {
      applySearch(inputEl.value);
    },
    clear: function() {
      inputEl.value = '';
      applySearch('');
    },
    destroy: function() {
      clearMarks();
    },
  };
}`
