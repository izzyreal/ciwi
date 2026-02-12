package server

const uiIndexBootJS = `
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
    setInterval(tick, 3000);
`
