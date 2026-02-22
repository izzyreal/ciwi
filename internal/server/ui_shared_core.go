package server

const uiSharedCoreJS = `
function escapeHtml(s) {
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

function blockedDependencyNameFromError(err) {
  const text = String(err || '').trim();
  if (!text) return '';
  let m = text.match(/^cancelled:\s+required job\s+(.+?)\s+failed$/i);
  if (m) return String(m[1] || '').trim();
  m = text.match(/^cancelled:\s+upstream pipeline\s+(.+?)\s+failed$/i);
  if (m) return String(m[1] || '').trim();
  return '';
}

function isDependencyBlockedJob(job) {
  const j = job || {};
  if (normalizedJobStatus(j.status) !== 'failed') return false;
  if (String(j.started_utc || '').trim()) return false;
  return blockedDependencyNameFromError(j.error).length > 0;
}

function statusClassForJob(job) {
  if (isDependencyBlockedJob(job)) return 'status-blocked';
  return statusClass((job && job.status) || '');
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
  const errText = String((job && job.error) || '').trim();
  if (normalizedJobStatus(status) === 'failed' && errText.toLowerCase() === 'cancelled by user') {
    return 'Cancelled by user';
  }
  const blockedDep = blockedDependencyNameFromError(errText);
  if (isDependencyBlockedJob(job) && blockedDep) {
    return 'blocked (dependency failed: ' + blockedDep + ')';
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

`
