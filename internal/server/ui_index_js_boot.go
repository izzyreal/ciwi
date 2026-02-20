package server

const uiIndexBootJS = `
    function focusQueuedJobsIfRequested() {
      let requested = false;
      try {
        requested = sessionStorage.getItem('__ciwiFocusQueuedJobs') === '1';
        if (requested) sessionStorage.removeItem('__ciwiFocusQueuedJobs');
      } catch (_) {}
      if (window.location.hash === '#queued-jobs') requested = true;
      if (!requested) return;
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
        refreshInFlight = false;
      }
    }

    refreshGuard.bindSelectionListener();
    tick();
    focusQueuedJobsIfRequested();
    setInterval(tick, 3000);
`
