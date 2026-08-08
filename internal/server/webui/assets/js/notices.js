(function () {
  'use strict';

  const capacity = 4;
  const state = {active: null, queue: []};

  function identity(notice) {
    return [notice.message, notice.action_label, notice.route, notice.section].join('\u0000');
  }

  function finish(notice) {
    if (!notice || state.active !== notice) return;
    if (notice.timer) window.clearTimeout(notice.timer);
    if (notice.node) notice.node.remove();
    state.active = null;
    presentNext();
  }

  function pause(notice) {
    if (!notice || state.active !== notice || !notice.timer) return;
    window.clearTimeout(notice.timer);
    notice.timer = null;
    notice.remaining = Math.max(0, notice.remaining - (Date.now() - notice.startedAt));
  }

  function resume(notice) {
    if (!notice || state.active !== notice || notice.timer) return;
    notice.startedAt = Date.now();
    notice.timer = window.setTimeout(() => finish(notice), Math.max(1, notice.remaining));
  }

  function navigateNotice(notice) {
    if (!notice.route) return;
    if (typeof window.ciwiNavigate === 'function') {
      void window.ciwiNavigate(notice.route, {section: notice.section || ''});
      return;
    }
    window.location.assign(notice.route + (notice.section ? '#' + encodeURIComponent(notice.section) : ''));
  }

  function presentNext() {
    if (state.active || state.queue.length === 0) return;
    const notice = state.queue.shift();
    state.active = notice;
    let host = document.getElementById('ciwiSnackbarHost');
    if (!host) {
      host = document.createElement('div');
      host.id = 'ciwiSnackbarHost';
      host.setAttribute('aria-live', 'polite');
      host.setAttribute('aria-atomic', 'false');
      document.body.appendChild(host);
    }
    const item = document.createElement('div');
    item.className = 'ciwi-snackbar';
    item.setAttribute('role', 'status');
    const message = document.createElement('div');
    message.className = 'ciwi-snackbar-message';
    message.textContent = notice.message;
    const actions = document.createElement('div');
    actions.className = 'ciwi-snackbar-actions';
    if (notice.action_label && notice.route) {
      const action = document.createElement('button');
      action.type = 'button';
      action.className = 'ciwi-snackbar-button';
      action.textContent = notice.action_label;
      action.addEventListener('click', () => {
        finish(notice);
        navigateNotice(notice);
      });
      actions.appendChild(action);
    }
    const dismiss = document.createElement('button');
    dismiss.type = 'button';
    dismiss.className = 'ciwi-snackbar-button dismiss';
    dismiss.textContent = 'Dismiss';
    dismiss.addEventListener('click', () => finish(notice));
    actions.appendChild(dismiss);
    item.append(message, actions);
    host.appendChild(item);
    notice.node = item;
    item.addEventListener('mouseenter', () => pause(notice));
    item.addEventListener('mouseleave', () => resume(notice));
    item.addEventListener('focusin', () => pause(notice));
    item.addEventListener('focusout', () => window.setTimeout(() => {
      if (!item.contains(document.activeElement)) resume(notice);
    }, 0));
    resume(notice);
  }

  function show(rawNotice) {
    const raw = rawNotice || {};
    const message = String(raw.message || '').trim();
    if (!message) return;
    const notice = {
      message,
      action_label: String(raw.action_label || raw.actionLabel || '').trim(),
      route: String(raw.route || '').trim(),
      section: String(raw.section || '').trim(),
      remaining: Math.max(1500, Number(raw.timeout_ms || 8000)),
      timer: null, node: null, startedAt: 0,
    };
    const key = identity(notice);
    if ((state.active && identity(state.active) === key) || state.queue.some(item => identity(item) === key)) return;
    const waitingCapacity = state.active ? capacity - 1 : capacity;
    if (state.queue.length >= waitingCapacity) state.queue.shift();
    state.queue.push(notice);
    presentNext();
  }

  window.ciwiShowNotice = show;
  window.ciwiNoticeState = state;
})();
