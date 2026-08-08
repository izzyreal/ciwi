(() => {
  'use strict';

  function createChangeRefreshScheduler(options) {
    const refresh = options.refresh;
    const delay = Number(options.delay == null ? 100 : options.delay);
    const setTimer = options.setTimer || ((callback, timeout) => window.setTimeout(callback, timeout));
    let timer = null;
    let pending = false;
    let activeRefreshes = 0;

    function arm() {
      if (timer !== null || activeRefreshes > 0 || !pending) return;
      timer = setTimer(async () => {
        timer = null;
        if (activeRefreshes > 0 || !pending) {
          arm();
          return;
        }
        pending = false;
        try {
          await refresh();
        } finally {
          arm();
        }
      }, delay);
    }

    return {
      schedule() {
        pending = true;
        arm();
      },
      beginRefresh() {
        activeRefreshes += 1;
      },
      endRefresh() {
        activeRefreshes = Math.max(0, activeRefreshes - 1);
        arm();
      },
    };
  }

  window.ciwiCreateChangeRefreshScheduler = createChangeRefreshScheduler;
})();
