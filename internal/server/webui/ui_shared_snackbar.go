package webui

const uiSharedSnackbarJS = `
function ensureSnackbarStyles() {
  if (document.getElementById('__ciwiSnackbarStyles')) return;
  const style = document.createElement('style');
  style.id = '__ciwiSnackbarStyles';
  style.textContent = [
    '#ciwiSnackbarHost{position:fixed;right:14px;bottom:14px;z-index:2500;display:flex;flex-direction:column;gap:10px;max-width:min(480px,92vw);pointer-events:none;}',
    '.ciwi-snackbar{pointer-events:auto;display:flex;align-items:center;justify-content:space-between;gap:10px;background:var(--snackbar-bg);color:var(--snackbar-ink);border:1px solid var(--snackbar-line);border-radius:10px;padding:10px 12px;box-shadow:0 16px 32px var(--shadow);}',
    '.ciwi-snackbar-msg{font-size:13px;line-height:1.25;word-break:break-word;}',
    '.ciwi-snackbar-actions{display:flex;gap:8px;align-items:center;flex-wrap:wrap;}',
    '.ciwi-snackbar-btn{font:inherit;font-size:12px;font-weight:600;padding:6px 8px;border-radius:7px;border:1px solid var(--snackbar-line);background:var(--surface-hover);color:var(--ink);cursor:pointer;}',
    '.ciwi-snackbar-btn.dismiss{background:transparent;color:var(--snackbar-ink);border-color:var(--snackbar-line);}',
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

`
