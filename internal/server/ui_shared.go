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
}`
