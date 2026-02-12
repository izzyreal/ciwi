package server

const uiIndexStateJS = `
    let refreshInFlight = false;
    const refreshGuard = createRefreshGuard(5000);
    let jobsRenderEpoch = 0;
    let lastQueuedJobsSignature = '';
    let lastHistoryJobsSignature = '';
    const PROJECT_GROUPS_STORAGE_KEY = 'ciwi.index.projectGroupsCollapsed.v1';
    const JOB_GROUPS_STORAGE_KEY = 'ciwi.index.jobGroupsExpanded.v1';

    function loadStringSet(key) {
      try {
        const raw = localStorage.getItem(key);
        if (!raw) return new Set();
        const parsed = JSON.parse(raw);
        if (!Array.isArray(parsed)) return new Set();
        return new Set(parsed.map(v => String(v || '')));
      } catch (_) {
        return new Set();
      }
    }

    function saveStringSet(key, values) {
      try {
        localStorage.setItem(key, JSON.stringify(Array.from(values || [])));
      } catch (_) {}
    }

    const projectGroupCollapsed = loadStringSet(PROJECT_GROUPS_STORAGE_KEY);
    const expandedJobGroups = loadStringSet(JOB_GROUPS_STORAGE_KEY);
    const JOBS_WINDOW = 150;
    const JOBS_BATCH_SIZE = 5;
`
