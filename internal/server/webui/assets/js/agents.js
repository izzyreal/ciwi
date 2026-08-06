    let refreshInFlight = false;
    const refreshGuard = createRefreshGuard(5000);

    function updateHeartbeatVisuals() {
      const now = Date.now();
      document.querySelectorAll('[data-heartbeat-unix-ms]').forEach(element => {
        const timestamp = element.getAttribute('data-heartbeat-unix-ms') || '';
        if (element.classList.contains('heartbeat-icon')) {
          element.style.opacity = String(window.ciwiHeartbeat.opacity(timestamp, now));
        } else if (element.classList.contains('heartbeat-age')) {
          element.textContent = window.ciwiHeartbeat.ageLabel(timestamp, now);
        }
      });
    }

    function formatUpdatePrimaryText(a) {
      if (!a || !a.update_requested) return '';
      const target = escapeHtml(a.update_target || '');
      if (a.job_in_progress) {
        return '<div class="badge badge-warn">Pending update → ' + target + ' (agent busy)</div>';
      }
      if (a.update_in_progress) {
        return '<div class="badge">Update → ' + target + ' in progress</div>';
      }
      return '<div class="badge">Update requested → ' + target + '</div>';
    }

    function formatUpdateRetryText(a) {
      if (!a || !a.update_requested || a.job_in_progress || a.update_in_progress || !a.update_next_retry_utc) return '';
      const attempt = Number(a.update_attempts || 0);
      if (attempt <= 0) return '';
      const reason = String(a.update_last_error || '').trim();
      const reasonSuffix = reason ? ': ' + escapeHtml(reason) : '';
      return '<div class="badge badge-error">Backoff until ' + escapeHtml(formatTimestamp(a.update_next_retry_utc)) + ' (attempt ' + String(attempt) + ')' + reasonSuffix + '</div>';
    }

    async function refreshAgents() {
      if (refreshInFlight || refreshGuard.shouldPause()) {
        return;
      }
      refreshInFlight = true;
      const rows = document.getElementById('rows');
      const summary = document.getElementById('summary');
      try {
        const res = await fetch('/api/v1/agents');
        if (!res.ok) throw new Error('HTTP ' + res.status);
        const data = await res.json();
        const agents = data.agents || [];
        rows.innerHTML = '';
        if (agents.length === 0) {
          rows.innerHTML = '<tr><td colspan="8" class="muted">No agents have sent heartbeats yet.</td></tr>';
          summary.textContent = '0 agents';
          return;
        }
        agents.sort((a, b) => String(a.agent_id || '').localeCompare(String(b.agent_id || '')));
        for (const a of agents) {
          const s = statusForLastSeen(a.last_seen_utc || '');
          const authorized = !!a.authorized;
          const tr = document.createElement('tr');
          const updateBtn = (a.update_requested && !a.update_in_progress)
            ? '<button data-action="update" data-agent-id="' + escapeHtml(a.agent_id || '') + '">Retry Update Now</button>'
            : ((!a.update_in_progress && a.needs_update && s.label !== 'offline')
              ? '<button data-action="update" data-agent-id="' + escapeHtml(a.agent_id || '') + '">Update</button>'
              : '');
          const refreshBtn = (s.label !== 'offline')
            ? '<button data-action="refresh-tools" data-agent-id="' + escapeHtml(a.agent_id || '') + '">Refresh Tools</button>'
            : '';
          const activationBtn = a.deactivated
            ? '<button data-action="activate" data-agent-id="' + escapeHtml(a.agent_id || '') + '">Activate</button>'
            : '<button data-action="deactivate" data-agent-id="' + escapeHtml(a.agent_id || '') + '">Deactivate</button>';
          const authBtn = authorized
            ? '<button data-action="unauthorize" data-agent-id="' + escapeHtml(a.agent_id || '') + '">Unauthorize</button>'
            : '<button data-action="authorize" data-agent-id="' + escapeHtml(a.agent_id || '') + '">Authorize</button>';
          const deleteBtn = '<button data-action="delete" data-agent-id="' + escapeHtml(a.agent_id || '') + '">Delete</button>';
          const primaryUpdateText = formatUpdatePrimaryText(a);
          const retryText = formatUpdateRetryText(a);
          const activationBadge = a.deactivated ? '<div class="badge badge-warn">Deactivated</div>' : '';
          const versionCell = escapeHtml(a.version || '') +
            activationBadge +
            primaryUpdateText +
            retryText;
          const lastSeen = String(a.last_seen_utc || '');
          const lastSeenUnixMS = window.ciwiHeartbeat.timestampMilliseconds(lastSeen);
          const runModeRaw = String((a.capabilities && a.capabilities.run_mode) || '').trim().toLowerCase();
          const runModeLabel = runModeRaw === 'service' ? 'Service' : 'Manual';
          const actionHTML = authorized
            ? (authBtn + ' ' + activationBtn + ' ' + deleteBtn + ' ' + updateBtn + ' ' + refreshBtn)
            : authBtn;
          tr.innerHTML =
            '<td><a href="/agents/' + encodeURIComponent(a.agent_id || '') + '">' + escapeHtml(a.agent_id || '') + '</a></td>' +
            '<td>' + escapeHtml(a.hostname || '') + '</td>' +
            '<td>' + escapeHtml((a.os || '') + '/' + (a.arch || '')) + '</td>' +
            '<td>' + versionCell + '</td>' +
            '<td class="heartbeat-cell"><div class="heartbeat-wrap"><span class="heartbeat-icon" data-heartbeat-unix-ms="' + String(lastSeenUnixMS) + '" role="img" aria-label="heartbeat">❤️</span><span class="heartbeat-age" data-heartbeat-unix-ms="' + String(lastSeenUnixMS) + '">' + escapeHtml(window.ciwiHeartbeat.ageLabel(lastSeenUnixMS, Date.now())) + '</span></div></td>' +
            '<td class="' + s.cls + '">' + s.label + '</td>' +
            '<td class="run-mode">' + runModeLabel + '</td>' +
            '<td>' + actionHTML + '</td>';
          rows.appendChild(tr);
        }
        updateHeartbeatVisuals();
        rows.querySelectorAll('button[data-action]').forEach(btn => {
          btn.addEventListener('click', async () => {
            const id = btn.getAttribute('data-agent-id') || '';
            if (!id) return;
            const action = btn.getAttribute('data-action') || '';
            if (action === 'deactivate') {
              const confirmed = await showConfirmDialog({
                title: 'Deactivate Agent',
                message: 'Deactivate this agent? Active jobs will be cancelled.',
                okLabel: 'Deactivate',
              });
              if (!confirmed) return;
            }
            if (action === 'unauthorize') {
              const confirmed = await showConfirmDialog({
                title: 'Unauthorize Agent',
                message: 'Revoke authorization for this agent? It will stop leasing new jobs.',
                okLabel: 'Unauthorize',
              });
              if (!confirmed) return;
            }
            if (action === 'delete') {
              const confirmed = await showConfirmDialog({
                title: 'Delete Agent Snapshot',
                message: 'Delete this agent snapshot from server state? It will reappear if the agent heartbeats again.',
                okLabel: 'Delete',
              });
              if (!confirmed) return;
            }
            btn.disabled = true;
            try {
              await window.ciwiRunAction('agent-action', { agentId: id, action: action }, btn, async runtime => {
                const res = await fetch('/api/v1/agents/' + encodeURIComponent(id) + '/actions', {
                  method: 'POST',
                  headers: ciwiActionHeaders(runtime, { 'Content-Type': 'application/json' }),
                  body: JSON.stringify({ action: action }),
                  signal: runtime.signal,
                });
                if (!res.ok) throw new Error(await res.text());
                return await res.json();
              });
              await refreshAgents();
            } catch (e) {
              await showAlertDialog({ title: 'Request failed', message: 'Request failed: ' + e.message });
            } finally {
              btn.disabled = false;
            }
          });
        });
        const online = agents.filter(a => statusForLastSeen(a.last_seen_utc || '').label === 'online').length;
        summary.textContent = online + '/' + agents.length + ' online';
      } catch (e) {
        rows.innerHTML = '<tr><td colspan="8" class="offline">Could not load agents</td></tr>';
        summary.textContent = 'Failed to load agents';
      } finally {
        refreshInFlight = false;
      }
    }

    document.getElementById('refreshBtn').onclick = refreshAgents;
    refreshGuard.bindSelectionListener();
    refreshAgents();
    setInterval(refreshAgents, 3000);
    setInterval(updateHeartbeatVisuals, 250);
