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

function isWaitingJob(job) {
  const current = job || {};
  if (normalizedJobStatus(current.status) !== 'queued') return false;
  if (current.waiting === true) return true;
  const metadata = current.metadata || {};
  return String(metadata.chain_blocked || '').trim() === '1' || String(metadata.needs_blocked || '').trim() === '1';
}

function jobWaitingReason(job) {
  if (!isWaitingJob(job)) return '';
  const metadata = (job && job.metadata) || {};
  const splitIDs = value => String(value || '').split(',').map(item => item.trim()).filter(Boolean);
  const pipelineIDs = splitIDs(metadata.chain_depends_on_pipelines);
  if (String(metadata.chain_blocked || '').trim() === '1' && pipelineIDs.length) {
    return 'Waiting for ' + (pipelineIDs.length === 1 ? 'pipeline ' : 'pipelines ') + pipelineIDs.join(', ');
  }
  const jobIDs = splitIDs(metadata.needs_job_ids);
  if (jobIDs.length) {
    return 'Waiting for ' + (jobIDs.length === 1 ? 'job ' : 'jobs ') + jobIDs.join(', ');
  }
  return 'Waiting for prerequisites';
}

function statusClassForJob(job) {
  if (isWaitingJob(job)) return 'status-waiting';
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

function ciwiJobActualDurationMs(job) {
  const started = new Date(String((job && job.started_utc) || ''));
  const finished = new Date(String((job && job.finished_utc) || ''));
  if (Number.isNaN(started.getTime()) || Number.isNaN(finished.getTime())) return 0;
  return Math.max(0, finished.getTime() - started.getTime());
}

function ciwiJobProgressModel(job, nowMs) {
  const current = job || {};
  const status = normalizedJobStatus(current.status);
  if (isTerminalJobStatus(status)) {
    return { state: 'complete', fraction: 1, overrun: false, weight: Math.max(0, Number(current.expected_duration_ms || 0)) || ciwiJobActualDurationMs(current) };
  }
  if (isWaitingJob(current)) {
    return { state: 'waiting', fraction: 0, overrun: false, weight: Math.max(0, Number(current.expected_duration_ms || 0)) };
  }
  if (!isActiveJobStatus(status)) return { state: 'none', fraction: 0, overrun: false, weight: 0 };

  const expected = Math.max(0, Number(current.expected_duration_ms || 0));
  const started = new Date(String(current.started_utc || ''));
  const hasStarted = !Number.isNaN(started.getTime());
  if (!expected) {
    return { state: 'indeterminate', fraction: 0, overrun: false, weight: 0 };
  }
  if (!hasStarted) return { state: 'determinate', fraction: 0, overrun: false, weight: expected };
  const elapsed = Math.max(0, Number(nowMs || Date.now()) - started.getTime());
  const ratio = elapsed / expected;
  return { state: ratio >= 1 ? 'overrun' : 'determinate', fraction: Math.min(1, ratio), overrun: ratio >= 1, weight: expected };
}

function ciwiAggregateProgressModel(jobs, nowMs) {
  const rows = Array.isArray(jobs) ? jobs.filter(Boolean) : [];
  if (!rows.length) return { state: 'none', fraction: 0, overrun: false };
  let totalWeight = 0;
  let completedWeight = 0;
  let active = false;
  let waiting = false;
  let waitingWithoutEstimate = false;
  let overrun = false;
  for (const job of rows) {
    const model = ciwiJobProgressModel(job, nowMs);
    if (model.state === 'waiting') {
      waiting = true;
      if (model.weight > 0) {
        totalWeight += model.weight;
      } else {
        waitingWithoutEstimate = true;
      }
      continue;
    }
    if (isActiveJobStatus(job.status)) active = true;
    if (model.state === 'indeterminate' || model.state === 'none' || model.weight <= 0) {
      if (isActiveJobStatus(job.status)) return { state: 'indeterminate', fraction: 0, overrun: false };
      continue;
    }
    totalWeight += model.weight;
    completedWeight += model.weight * model.fraction;
    overrun = overrun || model.overrun;
  }
  if (!active) return waiting ? { state: 'none', fraction: 0, overrun: false } : { state: 'complete', fraction: 1, overrun: false };
  if (waitingWithoutEstimate) return { state: 'indeterminate', fraction: 0, overrun: false };
  if (totalWeight <= 0) return { state: 'indeterminate', fraction: 0, overrun: false };
  const fraction = Math.max(0, Math.min(1, completedWeight / totalWeight));
  return { state: overrun && fraction >= .999 ? 'overrun' : 'determinate', fraction, overrun: overrun && fraction >= .999 };
}

function bindCiwiProgress(element, jobs) {
  if (!element) return;
  element.classList.add('ciwi-progress-surface');
  element.__ciwiProgressJobs = Array.isArray(jobs) ? jobs : [jobs];
  updateCiwiProgressElement(element, Date.now());
}

function updateCiwiProgressElement(element, nowMs) {
  if (!element) return;
  const jobs = Array.isArray(element.__ciwiProgressJobs) ? element.__ciwiProgressJobs : [];
  const model = jobs.length === 1 ? ciwiJobProgressModel(jobs[0], nowMs) : ciwiAggregateProgressModel(jobs, nowMs);
  const previousState = String(element.__ciwiProgressState || '');
  if (previousState !== model.state) {
    element.classList.remove('ciwi-progress-indeterminate', 'ciwi-progress-overrun', 'ciwi-progress-complete');
    if (model.state === 'indeterminate' || model.state === 'overrun') {
      const cycleMs = model.state === 'indeterminate' ? 4000 : 2000;
      const phaseMs = Math.max(0, Number(nowMs || Date.now())) % cycleMs;
      element.style.setProperty('--ciwi-progress-animation-delay', '-' + String(phaseMs) + 'ms');
    }
    if (model.state === 'indeterminate') element.classList.add('ciwi-progress-indeterminate');
    if (model.state === 'overrun') element.classList.add('ciwi-progress-overrun');
    if (model.state === 'complete') element.classList.add('ciwi-progress-complete');
    element.__ciwiProgressState = model.state;
  }
  if (model.state === 'none' || model.state === 'waiting') {
    element.style.setProperty('--ciwi-progress-width', '0%');
    return;
  }
  if (model.state === 'indeterminate') {
    return;
  }
  element.style.setProperty('--ciwi-progress-width', String(Math.max(0, Math.min(100, model.fraction * 100))) + '%');
}

function updateCiwiProgressIndicators() {
  document.querySelectorAll('.ciwi-progress-surface').forEach(element => updateCiwiProgressElement(element, Date.now()));
}

if (typeof window !== 'undefined') {
  window.setInterval(updateCiwiProgressIndicators, 250);
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
  if (isWaitingJob(job)) return 'waiting';
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
