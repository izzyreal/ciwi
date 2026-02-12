package server

const uiSharedJS = `function escapeHtml(s) {
  return (s || '').replace(/[&<>"']/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
}

function statusClass(status) {
  return 'status-' + (status || '').toLowerCase();
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

function formatDuration(startTs, finishTs, status) {
  if (!startTs) return '';
  const start = new Date(startTs);
  if (Number.isNaN(start.getTime())) return '';
  const end = finishTs ? new Date(finishTs) : new Date();
  if (Number.isNaN(end.getTime())) return '';
  let ms = end.getTime() - start.getTime();
  if (ms < 0) ms = 0;
  const totalSec = Math.floor(ms / 1000);
  const h = Math.floor(totalSec / 3600);
  const m = Math.floor((totalSec % 3600) / 60);
  const s = totalSec % 60;
  const core = h > 0
    ? String(h) + 'h ' + String(m).padStart(2, '0') + 'm ' + String(s).padStart(2, '0') + 's'
    : String(m).padStart(2, '0') + 'm ' + String(s).padStart(2, '0') + 's';
  if (!finishTs && status === 'running') {
    return core + ' (running)';
  }
  return core;
}

function jobDescription(job) {
  const m = job.metadata || {};
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
  if (!overlay || overlay.__ciwiModalCloseBound) return;
  overlay.__ciwiModalCloseBound = true;
  overlay.addEventListener('click', (ev) => {
    if (ev.target !== overlay) return;
    if (typeof onClose === 'function') onClose(); else closeModalOverlay(overlay);
  });
  document.addEventListener('keydown', (ev) => {
    if (ev.key !== 'Escape') return;
    if (overlay.style.display !== 'flex') return;
    if (typeof onClose === 'function') onClose(); else closeModalOverlay(overlay);
  });
}`
