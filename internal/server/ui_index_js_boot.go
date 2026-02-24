package server

const uiIndexBootJS = `
    let queuedFocusUntilMs = 0;

    function requestQueuedJobsFocusWindow(ms) {
      const now = Date.now();
      const duration = Math.max(1000, Number(ms || 8000));
      queuedFocusUntilMs = Math.max(queuedFocusUntilMs, now + duration);
    }

    function loadQueuedJobsFocusWindow() {
      if (window.location.hash === '#queued-jobs') {
        requestQueuedJobsFocusWindow(8000);
      }
    }

    function clearQueuedJobsFocusWindow() {
      queuedFocusUntilMs = 0;
    }

    function focusQueuedJobsIfRequested() {
      if (window.location.hash !== '#queued-jobs') {
        clearQueuedJobsFocusWindow();
        return;
      }
      const now = Date.now();
      if (queuedFocusUntilMs <= now) {
        clearQueuedJobsFocusWindow();
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
        await Promise.all([refreshProjects(), refreshJobs(), refreshRuntimeStateBanner('runtimeStateBanner')]);
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
