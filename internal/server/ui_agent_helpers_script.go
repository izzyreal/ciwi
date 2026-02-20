package server

const agentHelpersJS = `
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

    function parseAgentShells(caps) {
      const executor = String((caps && caps.executor) || '').trim().toLowerCase();
      if (executor !== 'script') return [];
      const raw = String((caps && caps.shells) || '').trim();
      if (!raw) return [];
      const unique = {};
      const out = [];
      raw.split(',').forEach(part => {
        const v = String(part || '').trim().toLowerCase();
        if (!v || unique[v]) return;
        unique[v] = true;
        out.push(v);
      });
      return out;
    }

    function exampleScriptForShell(shell) {
      if (shell === 'cmd') {
        return [
          'ver',
          'echo Hello from ciwi ad-hoc cmd',
          'echo Date: %DATE%',
          'echo Time: %TIME%',
        ].join('\n');
      }
      if (shell === 'powershell') {
        return [
          '$ErrorActionPreference = "Stop"',
          'Write-Host "Hello from ciwi ad-hoc (PowerShell)"',
          'Write-Host ("PSVersion: " + $PSVersionTable.PSVersion.ToString())',
          'Write-Host ("Date: " + (Get-Date -Format "yyyy-MM-dd HH:mm:ss"))',
        ].join('\n');
      }
      return [
        'set -eu',
        'echo "Hello from ciwi ad-hoc (POSIX)"',
        'uname -a',
        'date',
      ].join('\n');
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
          '<td class="' + statusClass(job.status) + '">' + escapeHtml(formatJobStatus(job)) + '</td>' +
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
      const suggested = exampleScriptForShell(adhocShellSelect.value || adhocShells[0]);
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
      adhocRunBtn.disabled = false;
      adhocRunBtn.textContent = 'Run';
    }

    function renderJobOutput(job) {
      const lines = [];
      lines.push('[job] ' + String(job.id || ''));
      lines.push('[status] ' + String(job.status || ''));
      if (job.created_utc) lines.push('[created] ' + formatTimestamp(job.created_utc));
      if (job.started_utc) lines.push('[started] ' + formatTimestamp(job.started_utc));
      if (job.finished_utc) lines.push('[finished] ' + formatTimestamp(job.finished_utc));
      if (job.exit_code !== undefined && job.exit_code !== null) lines.push('[exit_code] ' + String(job.exit_code));
      let body = lines.join('\n');
      if (job.output) body += '\n\n' + String(job.output);
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
        renderJobOutput(job);
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
      try {
        const res = await fetch('/api/v1/agents/' + encodeURIComponent(agentID) + '/actions', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ action: 'run-script', shell: shell, script: script, timeout_seconds: 600 }),
        });
        if (!res.ok) throw new Error(await res.text());
        const data = await res.json();
        const jobID = String(data.job_execution_id || '').trim();
        if (!jobID) throw new Error('server response missing job_execution_id');
        adhocActiveJobID = jobID;
        showJobStartedSnackbar('Adhoc script started', jobID);
        adhocOutput.textContent = '[queued] job_execution_id=' + jobID + '\n[poll] waiting for agent output...';
        pollAdhocJob(jobID);
      } catch (e) {
        adhocRunBtn.disabled = false;
        adhocRunBtn.textContent = 'Run';
        adhocOutput.textContent = '[run failed] ' + String(e.message || e);
      }
    }

    async function postAction(action) {
      const res = await fetch('/api/v1/agents/' + encodeURIComponent(agentID) + '/actions', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ action: action }),
      });
      if (!res.ok) throw new Error(await res.text());
    }


`
