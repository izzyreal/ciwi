package server

const agentRuntimeJS = `
    const agentID = decodeURIComponent(location.pathname.replace(/^\/agents\//, '').replace(/\/+$/, ''));
    const adhocModalOverlay = document.getElementById('adhocModalOverlay');
    const adhocShellSelect = document.getElementById('adhocShellSelect');
    const adhocScriptInput = document.getElementById('adhocScriptInput');
    const adhocOutput = document.getElementById('adhocOutput');
    const adhocRunBtn = document.getElementById('adhocRunBtn');
    let refreshInFlight = false;
    const refreshGuard = createRefreshGuard(5000);
    let adhocShells = [];
    let adhocActiveJobID = '';
    let adhocPollTimer = null;

    let lastSuggestedScript = '';
    async function refreshAgent(force) {
      if (refreshInFlight || (!force && refreshGuard.shouldPause())) {
        return;
      }
      refreshInFlight = true;
      try {
        const res = await fetch('/api/v1/agents/' + encodeURIComponent(agentID));
        if (!res.ok) {
          if (res.status === 404) throw new Error('Agent not found');
          throw new Error('HTTP ' + res.status);
        }
        const data = await res.json();
        const a = data.agent || {};
        let historyLoadError = '';
        let agentHistoryJobs = [];
        try {
          const jobRes = await fetch('/api/v1/jobs?view=history&max=150&offset=0&limit=150');
          if (!jobRes.ok) {
            throw new Error('HTTP ' + jobRes.status);
          }
          const jobData = await jobRes.json();
          const allJobs = Array.isArray(jobData.job_executions) ? jobData.job_executions : [];
          agentHistoryJobs = allJobs.filter(isJobForAgent);
        } catch (e) {
          historyLoadError = String((e && e.message) || e || 'unknown error');
        }
        const s = statusForLastSeen(a.last_seen_utc || '');
        document.getElementById('subtitle').textContent = a.agent_id || agentID;
        document.getElementById('statusText').innerHTML = 'Health: <span class="' + s.cls + '">' + s.label + '</span> | Activation: ' + (a.deactivated ? '<span class="offline">deactivated</span>' : '<span class="ok">active</span>');

        const activationButton = document.getElementById('activationBtn');
        const updateButton = document.getElementById('updateBtn');
        const restartButton = document.getElementById('restartBtn');
        const wipeCacheButton = document.getElementById('wipeCacheBtn');
        const flushAgentHistoryButton = document.getElementById('flushAgentHistoryBtn');
        const refreshToolsButton = document.getElementById('refreshToolsBtn');
        const runAdhocButton = document.getElementById('runAdhocBtn');
        adhocShells = parseAgentShells(a.capabilities || {});
        const showUpdate = (!a.update_in_progress) && (!!a.update_requested || (!!a.needs_update && s.label !== 'offline'));
        activationButton.textContent = a.deactivated ? 'Activate' : 'Deactivate';
        updateButton.style.display = showUpdate ? 'inline-block' : 'none';
        updateButton.textContent = a.update_requested ? 'Retry Update Now' : 'Update';
        restartButton.style.display = s.label !== 'offline' ? 'inline-block' : 'none';
        wipeCacheButton.style.display = s.label !== 'offline' ? 'inline-block' : 'none';
        flushAgentHistoryButton.style.display = 'inline-block';
        refreshToolsButton.style.display = s.label !== 'offline' ? 'inline-block' : 'none';
        runAdhocButton.style.display = adhocShells.length > 0 ? 'inline-block' : 'none';

        let updateState = '';
        if (a.update_requested) {
          updateState = formatUpdatePrimaryText(a);
          const retryText = formatUpdateRetryText(a);
          if (retryText) {
            updateState += ' ' + retryText;
          }
        }

        const metaHTML =
          metaRow('Agent ID', escapeHtml(a.agent_id || agentID)) +
          metaRow('Hostname', escapeHtml(a.hostname || '')) +
          metaRow('Platform', escapeHtml((a.os || '') + '/' + (a.arch || ''))) +
          metaRow('Version', escapeHtml(a.version || '')) +
          metaRow('Activation', a.deactivated ? '<span class="offline">Deactivated</span>' : '<span class="ok">Active</span>') +
          metaRow('Last Seen', escapeHtml(formatTimestamp(a.last_seen_utc || ''))) +
          metaRow('Capabilities', formatCapabilitiesInlineCode(a.capabilities || {})) +
          metaRow('Update status', updateState || '<span class="muted">No pending update</span>');
        document.getElementById('meta').innerHTML = metaHTML;
        document.getElementById('logBox').textContent = (a.recent_log || []).join('\n');
        renderAgentJobHistory(agentHistoryJobs, historyLoadError);
      } catch (e) {
        document.getElementById('subtitle').textContent = String(e.message || e);
        document.getElementById('statusText').textContent = 'Failed to load agent';
        renderAgentJobHistory([], String((e && e.message) || e || 'unknown error'));
      } finally {
        refreshInFlight = false;
      }
    }

    document.getElementById('refreshBtn').onclick = () => refreshAgent(true);
    document.getElementById('activationBtn').onclick = async () => {
      try {
        const res = await fetch('/api/v1/agents/' + encodeURIComponent(agentID));
        if (!res.ok) throw new Error('HTTP ' + res.status);
        const data = await res.json();
        const a = data.agent || {};
        const isDeactivated = !!a.deactivated;
        if (!isDeactivated) {
          const confirmed = await showConfirmDialog({
            title: 'Deactivate Agent',
            message: 'Deactivate this agent? Active jobs will be cancelled.',
            okLabel: 'Deactivate',
          });
          if (!confirmed) return;
        }
        await postAction(isDeactivated ? 'activate' : 'deactivate');
        await refreshAgent(true);
      } catch (e) {
        await showAlertDialog({ title: 'Activation change failed', message: 'Activation change failed: ' + e.message });
      }
    };
    document.getElementById('updateBtn').onclick = async () => {
      try {
        await postAction('update');
        await refreshAgent(true);
      } catch (e) {
        await showAlertDialog({ title: 'Update request failed', message: 'Update request failed: ' + e.message });
      }
    };
    document.getElementById('restartBtn').onclick = async () => {
      const confirmed = await showConfirmDialog({
        title: 'Restart Agent',
        message: 'Request restart for this agent?',
        okLabel: 'Restart agent',
      });
      if (!confirmed) return;
      try {
        await postAction('restart');
        await refreshAgent(true);
      } catch (e) {
        await showAlertDialog({ title: 'Restart request failed', message: 'Restart request failed: ' + e.message });
      }
    };
    document.getElementById('wipeCacheBtn').onclick = async () => {
      const confirmed = await showConfirmDialog({
        title: 'Wipe Cache',
        message: 'Wipe this agent cache now? This removes all cached dependency sources on that agent.',
        okLabel: 'Wipe cache',
      });
      if (!confirmed) return;
      try {
        await postAction('wipe-cache');
        await refreshAgent(true);
      } catch (e) {
        await showAlertDialog({ title: 'Cache wipe failed', message: 'Cache wipe request failed: ' + e.message });
      }
    };
    document.getElementById('flushAgentHistoryBtn').onclick = async () => {
      const confirmed = await showConfirmDialog({
        title: 'Flush Agent Job History',
        message: 'Flush job history for this agent? This deletes historical job records and artifact files for this agent.',
        okLabel: 'Flush history',
      });
      if (!confirmed) return;
      try {
        await postAction('flush-job-history');
        await refreshAgent(true);
      } catch (e) {
        await showAlertDialog({ title: 'Flush failed', message: 'Agent job history flush failed: ' + e.message });
      }
    };
    document.getElementById('refreshToolsBtn').onclick = async () => {
      try {
        await postAction('refresh-tools');
        await refreshAgent(true);
      } catch (e) {
        await showAlertDialog({ title: 'Refresh tools failed', message: 'Refresh tools request failed: ' + e.message });
      }
    };
    document.getElementById('runAdhocBtn').onclick = async () => {
      if (adhocShells.length === 0) {
        await showAlertDialog({ title: 'Adhoc unavailable', message: 'Agent does not advertise script shell capabilities.' });
        return;
      }
      openAdhocModal();
    };
    wireModalCloseBehavior(adhocModalOverlay, closeAdhocModal);
    adhocShellSelect.onchange = () => {
      const suggested = exampleScriptForShell(String(adhocShellSelect.value || ''));
      adhocScriptInput.value = suggested;
      lastSuggestedScript = suggested;
    };
    adhocRunBtn.onclick = () => runAdhocScript();
    document.getElementById('adhocCloseBtn').onclick = () => closeAdhocModal();
    refreshGuard.bindSelectionListener();
    refreshAgent(true);
    setInterval(() => refreshAgent(false), 3000);

`
