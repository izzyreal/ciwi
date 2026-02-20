package server

const uiIndexBootJS = `
    let queuedFocusUntilMs = 0;

    function requestQueuedJobsFocusWindow(ms) {
      const now = Date.now();
      const duration = Math.max(1000, Number(ms || 8000));
      queuedFocusUntilMs = Math.max(queuedFocusUntilMs, now + duration);
      try {
        sessionStorage.setItem('__ciwiFocusQueuedJobsUntil', String(queuedFocusUntilMs));
      } catch (_) {}
    }

    function loadQueuedJobsFocusWindow() {
      const now = Date.now();
      let requested = false;
      try {
        const legacy = sessionStorage.getItem('__ciwiFocusQueuedJobs') === '1';
        if (legacy) {
          requested = true;
          sessionStorage.removeItem('__ciwiFocusQueuedJobs');
        }
        const untilRaw = sessionStorage.getItem('__ciwiFocusQueuedJobsUntil') || '';
        const until = Number(untilRaw);
        if (Number.isFinite(until) && until > now) {
          queuedFocusUntilMs = Math.max(queuedFocusUntilMs, until);
          requested = true;
        } else if (untilRaw) {
          sessionStorage.removeItem('__ciwiFocusQueuedJobsUntil');
        }
      } catch (_) {}
      if (window.location.hash === '#queued-jobs') {
        requested = true;
      }
      if (requested) {
        requestQueuedJobsFocusWindow(8000);
      }
    }

    function clearQueuedJobsFocusWindow() {
      queuedFocusUntilMs = 0;
      try {
        sessionStorage.removeItem('__ciwiFocusQueuedJobsUntil');
      } catch (_) {}
    }

    function focusQueuedJobsIfRequested() {
      const now = Date.now();
      if (queuedFocusUntilMs <= now) {
        if (queuedFocusUntilMs > 0) clearQueuedJobsFocusWindow();
        return;
      }
      const node = document.getElementById('queued-jobs');
      if (!node || typeof node.scrollIntoView !== 'function') return;
      requestAnimationFrame(() => {
        node.scrollIntoView({ block: 'start', behavior: 'smooth' });
      });
    }

    async function tick() {
      if (refreshInFlight || refreshGuard.shouldPause()) {
        return;
      }
      refreshInFlight = true;
      try {
        await Promise.all([refreshProjects(), refreshJobs()]);
      } catch (e) {
        console.error(e);
      } finally {
        focusQueuedJobsIfRequested();
        refreshInFlight = false;
      }
    }

    refreshGuard.bindSelectionListener();
    loadQueuedJobsFocusWindow();
    tick();
    focusQueuedJobsIfRequested();
    setInterval(tick, 3000);
`
