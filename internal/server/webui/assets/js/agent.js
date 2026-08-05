
    function formatUpdatePrimaryText(a) {
      if (!a || !a.update_requested) return '';
      const target = escapeHtml(a.update_target || '');
      if (a.job_in_progress) {
        return '<span class="badge badge-warn">Pending update → ' + target + ' (agent busy)</span>';
      }
      if (a.update_in_progress) {
        return '<span class="badge">Update → ' + target + ' in progress</span>';
      }
      return '<span class="badge">Update requested → ' + target + '</span>';
    }

    function formatUpdateRetryText(a) {
      if (!a || !a.update_requested || a.job_in_progress || a.update_in_progress || !a.update_next_retry_utc) return '';
      const attempt = Number(a.update_attempts || 0);
      if (attempt <= 0) return '';
      const reason = String(a.update_last_error || '').trim();
      const reasonSuffix = reason ? ': ' + escapeHtml(reason) : '';
      return '<span class="badge badge-error">Backoff until ' + escapeHtml(formatTimestamp(a.update_next_retry_utc)) + ' (attempt ' + String(attempt) + ')' + reasonSuffix + '</span>';
    }
    function metaRow(k, v) {
      return '<div class="label">' + escapeHtml(k) + '</div><div class="value">' + v + '</div>';
    }

    function formatCapabilitiesInlineCode(caps) {
      if (!caps) return '<span class="muted">none</span>';
      const entries = Object.entries(caps);
      if (entries.length === 0) return '<span class="muted">none</span>';
      entries.sort((a, b) => String(a[0] || '').localeCompare(String(b[0] || '')));
      return entries.map(([k, v]) => '<code class="cap-code">' + escapeHtml(String(k) + '=' + String(v)) + '</code>').join('');
    }

    function clearAdhocPoll() {
      if (adhocPollTimer) {
        clearTimeout(adhocPollTimer);
        adhocPollTimer = null;
      }
    }

    function isJobForAgent(job) {
      const leased = String((job && job.leased_by_agent_id) || '').trim();
      if (leased === agentID) return true;
      const meta = (job && job.metadata) || {};
      return String(meta.adhoc_agent_id || '').trim() === agentID;
    }

    function renderAgentJobHistory(jobs, loadError) {
      const metaEl = document.getElementById('jobHistoryMeta');
      const emptyEl = document.getElementById('jobHistoryEmpty');
      const tableEl = document.getElementById('jobHistoryTable');
      const bodyEl = document.getElementById('jobHistoryBody');
      if (!metaEl || !emptyEl || !tableEl || !bodyEl) return;

      if (loadError) {
        metaEl.textContent = 'Failed to load';
        emptyEl.style.display = 'block';
        emptyEl.textContent = 'Job history could not be loaded: ' + String(loadError);
        tableEl.style.display = 'none';
        bodyEl.innerHTML = '';
        return;
      }

      const list = Array.isArray(jobs) ? jobs : [];
      metaEl.textContent = list.length + ' job(s)';
      if (list.length === 0) {
        emptyEl.style.display = 'block';
        emptyEl.textContent = 'No finished jobs executed by this agent yet.';
        tableEl.style.display = 'none';
        bodyEl.innerHTML = '';
        return;
      }

      const backTo = encodeURIComponent(window.location.pathname || '/');
      const rows = list.map(job => {
        const id = String(job.id || '').trim();
        const href = '/jobs/' + encodeURIComponent(id) + '?back=' + backTo;
        return '<tr>' +
          '<td><a href="' + href + '">' + escapeHtml(jobDescription(job)) + '</a></td>' +
          '<td class="' + statusClassForJob(job) + '">' + escapeHtml(formatJobStatus(job)) + '</td>' +
          '<td>' + escapeHtml((job.metadata && job.metadata.pipeline_id) || '') + '</td>' +
          '<td>' + escapeHtml(buildVersionLabel(job)) + '</td>' +
          '<td>' + escapeHtml(formatTimestamp(job.created_utc || '')) + '</td>' +
          '<td>' + escapeHtml(formatTimestamp(job.finished_utc || '')) + '</td>' +
        '</tr>';
      }).join('');
      bodyEl.innerHTML = rows;
      emptyEl.style.display = 'none';
      tableEl.style.display = 'table';
    }

    function openAdhocModal() {
      if (adhocShells.length === 0) return;
      adhocShellSelect.innerHTML = '';
      adhocShells.forEach(shell => {
        const opt = document.createElement('option');
        opt.value = shell;
        opt.textContent = shell;
        adhocShellSelect.appendChild(opt);
      });
      const suggested = adhocShellExamples[adhocShellSelect.value || adhocShells[0]] || '';
      adhocScriptInput.value = suggested;
      lastSuggestedScript = suggested;
      if (!adhocActiveJobID) {
        adhocOutput.textContent = 'Pick a shell, tweak the example script, then click Run.';
      }
      adhocRunBtn.disabled = false;
      adhocRunBtn.textContent = 'Run';
      openModalOverlay(adhocModalOverlay, '90vw', '90vh');
      setTimeout(() => adhocScriptInput.focus(), 0);
    }

    function closeAdhocModal() {
      closeModalOverlay(adhocModalOverlay);
      clearAdhocPoll();
      adhocActiveJobID = '';
      adhocEvents = [];
      adhocLastEventID = 0;
      adhocRunBtn.disabled = false;
      adhocRunBtn.textContent = 'Run';
    }

    function renderJobOutput(job, events) {
      const lines = [];
      lines.push('[job] ' + String(job.id || ''));
      lines.push('[status] ' + String(job.status || ''));
      if (job.created_utc) lines.push('[created] ' + formatTimestamp(job.created_utc));
      if (job.started_utc) lines.push('[started] ' + formatTimestamp(job.started_utc));
      if (job.finished_utc) lines.push('[finished] ' + formatTimestamp(job.finished_utc));
      if (job.exit_code !== undefined && job.exit_code !== null) lines.push('[exit_code] ' + String(job.exit_code));
      let body = lines.join('\n');
      const output = (Array.isArray(events) ? events : []).map(event => {
        if (event && event.type === 'step.output') return String(event.output || '');
        if (event && event.type === 'system.message') return String(event.message || '');
        return '';
      }).join('');
      if (output) body += '\n\n' + output;
      if (job.error) body += '\n\n[error]\n' + String(job.error);
      adhocOutput.textContent = body;
      adhocOutput.scrollTop = adhocOutput.scrollHeight;
    }

    async function pollAdhocJob(jobID) {
      if (!jobID || jobID !== adhocActiveJobID) return;
      try {
        const res = await fetch('/api/v1/jobs/' + encodeURIComponent(jobID));
        if (!res.ok) throw new Error('HTTP ' + res.status);
        const data = await res.json();
        const job = data.job_execution || {};
        const eventsRes = await fetch('/api/v1/jobs/' + encodeURIComponent(jobID) + '/events?after_id=' + encodeURIComponent(String(adhocLastEventID)));
        if (!eventsRes.ok) throw new Error('events HTTP ' + eventsRes.status);
        const eventsData = await eventsRes.json();
        const newEvents = Array.isArray(eventsData.events) ? eventsData.events : [];
        if (newEvents.length) adhocEvents = adhocEvents.concat(newEvents);
        const nextEventID = Number(eventsData.next_event_id || 0);
        if (Number.isFinite(nextEventID) && nextEventID >= adhocLastEventID) adhocLastEventID = nextEventID;
        renderJobOutput(job, adhocEvents);
        const terminal = isTerminalJobStatus(job.status);
        if (terminal) {
          adhocRunBtn.disabled = false;
          adhocRunBtn.textContent = 'Run';
          adhocActiveJobID = '';
          clearAdhocPoll();
          return;
        }
      } catch (e) {
        adhocOutput.textContent += '\n\n[poll error] ' + String(e.message || e);
      }
      adhocPollTimer = setTimeout(() => pollAdhocJob(jobID), 900);
    }

    async function runAdhocScript() {
      const shell = String(adhocShellSelect.value || '').trim();
      const script = String(adhocScriptInput.value || '');
      if (!shell) {
        await showAlertDialog({ title: 'Missing shell', message: 'Pick a shell first.' });
        return;
      }
      if (!script.trim()) {
        await showAlertDialog({ title: 'Missing script', message: 'Script is empty.' });
        return;
      }
      adhocRunBtn.disabled = true;
      adhocRunBtn.textContent = 'Running...';
      adhocOutput.textContent = 'Queueing ad-hoc job...';
      clearAdhocPoll();
      adhocActiveJobID = '';
      adhocEvents = [];
      adhocLastEventID = 0;
      try {
        const data = await postAction('run-script', adhocRunBtn, { shell: shell, script: script, timeout_seconds: 600 });
        const jobID = String(data.job_execution_id || '').trim();
        if (!jobID) throw new Error('server response missing job_execution_id');
        adhocActiveJobID = jobID;
        adhocEvents = [];
        adhocLastEventID = 0;
        showJobStartedSnackbar('Adhoc script started', jobID);
        adhocOutput.textContent = '[queued] job_execution_id=' + jobID + '\n[poll] waiting for agent output...';
        pollAdhocJob(jobID);
      } catch (e) {
        adhocRunBtn.disabled = false;
        adhocRunBtn.textContent = 'Run';
        adhocOutput.textContent = '[run failed] ' + String(e.message || e);
      }
    }

    async function postAction(action, element, extraPayload) {
      const payload = { action: action, ...(extraPayload || {}) };
      return window.ciwiRunAction('agent-action', { agentId: agentID, action: action }, element || null, async runtime => {
        const res = await fetch('/api/v1/agents/' + encodeURIComponent(agentID) + '/actions', {
          method: 'POST',
          headers: ciwiActionHeaders(runtime, { 'Content-Type': 'application/json' }),
          body: JSON.stringify(payload),
          signal: runtime.signal,
        });
        if (!res.ok) throw new Error(await res.text());
        return await res.json();
      });
    }




    const agentID = decodeURIComponent(location.pathname.replace(/^\/agents\//, '').replace(/\/+$/, ''));
    const adhocModalOverlay = document.getElementById('adhocModalOverlay');
    const adhocShellSelect = document.getElementById('adhocShellSelect');
    const adhocScriptInput = document.getElementById('adhocScriptInput');
    const adhocOutput = document.getElementById('adhocOutput');
    const adhocRunBtn = document.getElementById('adhocRunBtn');
    let refreshInFlight = false;
    const refreshGuard = createRefreshGuard(5000);
    let adhocShells = [];
    let adhocShellExamples = {};
    let adhocActiveJobID = '';
    let adhocPollTimer = null;
    let adhocEvents = [];
    let adhocLastEventID = 0;

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
        const scriptShells = Array.isArray(a.script_shells) ? a.script_shells : [];
        adhocShells = scriptShells.map(shell => String(shell.value || '').trim()).filter(Boolean);
        adhocShellExamples = Object.fromEntries(scriptShells.map(shell => [String(shell.value || '').trim(), String(shell.example_script || '')]));
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
    document.getElementById('activationBtn').onclick = async (event) => {
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
        await postAction(isDeactivated ? 'activate' : 'deactivate', event.currentTarget);
        await refreshAgent(true);
      } catch (e) {
        await showAlertDialog({ title: 'Activation change failed', message: 'Activation change failed: ' + e.message });
      }
    };
    document.getElementById('updateBtn').onclick = async (event) => {
      try {
        await postAction('update', event.currentTarget);
        await refreshAgent(true);
      } catch (e) {
        await showAlertDialog({ title: 'Update request failed', message: 'Update request failed: ' + e.message });
      }
    };
    document.getElementById('restartBtn').onclick = async (event) => {
      const confirmed = await showConfirmDialog({
        title: 'Restart Agent',
        message: 'Request restart for this agent?',
        okLabel: 'Restart agent',
      });
      if (!confirmed) return;
      try {
        await postAction('restart', event.currentTarget);
        await refreshAgent(true);
      } catch (e) {
        await showAlertDialog({ title: 'Restart request failed', message: 'Restart request failed: ' + e.message });
      }
    };
    document.getElementById('wipeCacheBtn').onclick = async (event) => {
      const confirmed = await showConfirmDialog({
        title: 'Wipe Cache',
        message: 'Wipe this agent cache now? This removes all cached dependency sources on that agent.',
        okLabel: 'Wipe cache',
      });
      if (!confirmed) return;
      try {
        await postAction('wipe-cache', event.currentTarget);
        await refreshAgent(true);
      } catch (e) {
        await showAlertDialog({ title: 'Cache wipe failed', message: 'Cache wipe request failed: ' + e.message });
      }
    };
    document.getElementById('flushAgentHistoryBtn').onclick = async (event) => {
      const confirmed = await showConfirmDialog({
        title: 'Flush Agent Job History',
        message: 'Flush job history for this agent? This deletes historical job records and artifact files for this agent.',
        okLabel: 'Flush history',
      });
      if (!confirmed) return;
      try {
        await postAction('flush-job-history', event.currentTarget);
        await refreshAgent(true);
      } catch (e) {
        await showAlertDialog({ title: 'Flush failed', message: 'Agent job history flush failed: ' + e.message });
      }
    };
    document.getElementById('refreshToolsBtn').onclick = async (event) => {
      try {
        await postAction('refresh-tools', event.currentTarget);
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
      const suggested = adhocShellExamples[String(adhocShellSelect.value || '')] || '';
      adhocScriptInput.value = suggested;
      lastSuggestedScript = suggested;
    };
    adhocRunBtn.onclick = () => runAdhocScript();
    document.getElementById('adhocCloseBtn').onclick = () => closeAdhocModal();
    refreshGuard.bindSelectionListener();
    refreshAgent(true);
    setInterval(() => refreshAgent(false), 3000);
