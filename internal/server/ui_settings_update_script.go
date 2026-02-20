package server

const settingsUpdateJS = `
    let refreshInFlight = false;
    const refreshGuard = createRefreshGuard(5000);
    const projectReloadState = new Map();
    let updateRestartWatchActive = false;
    let rollbackTagsLoadedAt = 0;
    const shownAgentUpdateWarningKeys = new Set();
    // DEBUG(apply-update-confirm): temporary client-side diagnostics for flaky confirm/update flow.
    // Remove this block after investigation is complete.
    let applyUpdateClickSeq = 0;
    function logApplyUpdateDebug(phase, payload) {
      const entry = {
        ts: new Date().toISOString(),
        phase: String(phase || ''),
        payload: payload || {},
      };
      if (!window.__ciwiApplyUpdateDebugLog) {
        window.__ciwiApplyUpdateDebugLog = [];
      }
      window.__ciwiApplyUpdateDebugLog.push(entry);
      if (window.__ciwiApplyUpdateDebugLog.length > 200) {
        window.__ciwiApplyUpdateDebugLog.shift();
      }
      try {
        console.info('[ciwi][apply-update]', entry);
      } catch (_) {
      }
    }
    // END DEBUG(apply-update-confirm)

    function parseSemverParts(value) {
      const raw = String(value || '').trim();
      if (!raw) return null;
      const normalized = raw.startsWith('v') ? raw.slice(1) : raw;
      const m = normalized.match(/^(\d+)\.(\d+)\.(\d+)(?:[-+].*)?$/);
      if (!m) return null;
      return [Number(m[1]), Number(m[2]), Number(m[3])];
    }

    function isStrictlyLowerVersion(candidate, current) {
      const c = parseSemverParts(candidate);
      const cur = parseSemverParts(current);
      if (!c || !cur) return false;
      for (let i = 0; i < 3; i += 1) {
        if (c[i] < cur[i]) return true;
        if (c[i] > cur[i]) return false;
      }
      return false;
    }

    document.getElementById('checkUpdatesBtn').onclick = async () => {
      const result = document.getElementById('updateResult');
      result.textContent = 'Checking...';
      try {
        const r = await apiJSON('/api/v1/update/check', { method: 'POST', body: '{}' });
        const latest = r.latest_version || '';
        const current = r.current_version || '';
        if (r.update_available) {
          result.textContent = 'Update available: ' + current + ' -> ' + latest + (r.asset_name ? (' (' + r.asset_name + ')') : '');
        } else {
          result.textContent = r.message || ('Up to date (' + current + ')');
        }
      } catch (e) {
        result.textContent = 'Update check failed: ' + e.message;
      }
      await refreshUpdateStatus();
      await refreshRollbackTags(false);
    };

    async function refreshRollbackTags(force) {
      const select = document.getElementById('rollbackTagSelect');
      if (!select) return;
      const rollbackBtn = document.getElementById('rollbackUpdateBtn');
      if (rollbackBtn && rollbackBtn.disabled) {
        if (!select.options.length) {
          const opt = document.createElement('option');
          opt.value = '';
          opt.textContent = 'Rollback disabled';
          select.appendChild(opt);
        }
        return;
      }
      const now = Date.now();
      if (!force && rollbackTagsLoadedAt > 0 && (now - rollbackTagsLoadedAt) < 60000 && select.options.length > 0) {
        return;
      }
      const prev = select.value;
      try {
        const r = await apiJSON('/api/v1/update/tags');
        const tags = Array.isArray(r.tags) ? r.tags : [];
        const current = String(r.current_version || '').trim();
        select.innerHTML = '';
        const filtered = tags
          .map(tag => String(tag || '').trim())
          .filter(v => v && v !== current && isStrictlyLowerVersion(v, current));
        if (filtered.length === 0) {
          const opt = document.createElement('option');
          opt.value = '';
          opt.textContent = 'No lower versions available';
          select.appendChild(opt);
          rollbackTagsLoadedAt = now;
          return;
        }
        filtered.forEach(v => {
          const opt = document.createElement('option');
          opt.value = v;
          opt.textContent = v;
          select.appendChild(opt);
        });
        const hasPrev = prev && Array.from(select.options).some(o => o.value === prev);
        if (hasPrev) {
          select.value = prev;
        } else if (select.options.length > 0) {
          select.selectedIndex = 0;
        }
        rollbackTagsLoadedAt = now;
      } catch (e) {
        select.innerHTML = '';
        const opt = document.createElement('option');
        opt.value = '';
        opt.textContent = 'Tag load failed';
        select.appendChild(opt);
        rollbackTagsLoadedAt = now;
      }
    }

    async function postJSONWithTimeout(path, body, timeoutMs) {
      const ctrl = new AbortController();
      const timer = setTimeout(() => ctrl.abort(), timeoutMs);
      try {
        const res = await fetch(path, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: body,
          signal: ctrl.signal,
        });
        if (!res.ok) {
          const text = await res.text();
          throw new Error(text || ('HTTP ' + res.status));
        }
        return await res.json();
      } finally {
        clearTimeout(timer);
      }
    }

    async function monitorApplyProgressAfterTimeout() {
      const result = document.getElementById('updateResult');
      const started = Date.now();
      while (Date.now() - started < 120000) {
        try {
          const r = await apiJSON('/api/v1/update/status');
          const s = r.status || {};
          const apply = (s.update_last_apply_status || '').trim();
          const msg = (s.update_message || '').trim();
          if (apply === 'running' || apply === '') {
            result.textContent = msg ? ('Update still running: ' + msg) : 'Update still running...';
          } else if (apply === 'failed') {
            result.textContent = 'Update failed: ' + (msg || 'unknown error');
            return;
          } else if (apply === 'staged' || apply === 'success' || apply === 'noop') {
            result.textContent = msg || ('Update state: ' + apply);
            if (apply === 'staged' || apply === 'success') {
              waitForServerRestartAndReload();
            }
            return;
          } else {
            result.textContent = msg || ('Update state: ' + apply);
            return;
          }
        } catch (_) {
          // During restart or temporary network churn, keep polling.
        }
        await new Promise(r => setTimeout(r, 1000));
      }
      result.textContent = 'Update request timed out; check update status and try again if needed.';
    }

    document.getElementById('applyUpdateBtn').onclick = async (ev) => {
      const clickId = (++applyUpdateClickSeq);
      const result = document.getElementById('updateResult');
      // DEBUG(apply-update-confirm)
      logApplyUpdateDebug('click', {
        click_id: clickId,
        is_trusted: !!(ev && ev.isTrusted),
        detail: Number((ev && ev.detail) || 0),
      });
      const confirmed = await showConfirmDialog({
        title: 'Apply Update',
        message: 'Apply update now and restart ciwi?',
        okLabel: 'Apply update',
      });
      logApplyUpdateDebug('confirm_result', { click_id: clickId, confirmed: !!confirmed });
      if (!confirmed) return;
      result.textContent = 'Starting update...';
      logApplyUpdateDebug('request_begin', { click_id: clickId, path: '/api/v1/update/apply' });
      try {
        const r = await postJSONWithTimeout('/api/v1/update/apply', '{}', 30000);
        logApplyUpdateDebug('request_ok', {
          click_id: clickId,
          updated: !!(r && r.updated),
          message: String((r && r.message) || ''),
        });
        result.textContent = (r.message || 'Update started. Refresh in a moment.');
        if (r.updated) {
          logApplyUpdateDebug('restart_watch_begin', { click_id: clickId });
          waitForServerRestartAndReload();
        }
      } catch (e) {
        logApplyUpdateDebug('request_error', {
          click_id: clickId,
          name: String((e && e.name) || ''),
          message: String((e && e.message) || ''),
        });
        if (e && e.name === 'AbortError') {
          result.textContent = 'Update request timed out; checking status...';
          await monitorApplyProgressAfterTimeout();
        } else {
          result.textContent = 'Update failed: ' + e.message;
        }
      }
      await refreshUpdateStatus();
      logApplyUpdateDebug('refresh_done', { click_id: clickId });
      // END DEBUG(apply-update-confirm)
    };

    document.getElementById('restartServerBtn').onclick = async (ev) => {
      if (ev && typeof ev.preventDefault === 'function') ev.preventDefault();
      const result = document.getElementById('updateResult');
      const confirmed = await showConfirmDialog({
        title: 'Restart Server',
        message: 'Restart ciwi server now?',
        okLabel: 'Restart server',
      });
      if (!confirmed) return;
      result.textContent = 'Restart requested...';
      try {
        const r = await postJSONWithTimeout('/api/v1/server/restart', '{}', 10000);
        result.textContent = r.message || 'Server restarting...';
        waitForServerRestartAndReload();
      } catch (e) {
        if (e && e.name === 'AbortError') {
          result.textContent = 'Restart request timed out; waiting for server restart...';
          waitForServerRestartAndReload();
        } else {
          result.textContent = 'Restart failed: ' + e.message;
        }
      }
    };

    document.getElementById('refreshRollbackTagsBtn').onclick = async () => {
      await refreshRollbackTags(true);
    };

    document.getElementById('rollbackUpdateBtn').onclick = async () => {
      const result = document.getElementById('rollbackResult');
      const select = document.getElementById('rollbackTagSelect');
      const target = ((select && select.value) || '').trim();
      if (!target) {
        result.textContent = 'Select a rollback tag first.';
        return;
      }
      const confirmed = await showConfirmDialog({
        title: 'Rollback',
        message: 'Rollback server and agents to ' + target + '?',
        okLabel: 'Rollback',
      });
      if (!confirmed) return;
      result.textContent = 'Starting rollback to ' + target + '...';
      try {
        const body = JSON.stringify({ target_version: target });
        const r = await postJSONWithTimeout('/api/v1/update/rollback', body, 30000);
        result.textContent = (r.message || ('Rollback to ' + target + ' started.'));
        if (r.updated) {
          waitForServerRestartAndReload();
        }
      } catch (e) {
        if (e && e.name === 'AbortError') {
          result.textContent = 'Rollback request timed out; checking status...';
          await monitorApplyProgressAfterTimeout();
        } else {
          result.textContent = 'Rollback failed: ' + e.message;
        }
      }
      await refreshUpdateStatus();
      await refreshRollbackTags(false);
    };

    async function waitForServerRestartAndReload() {
      if (updateRestartWatchActive) return;
      updateRestartWatchActive = true;
      const result = document.getElementById('updateResult');
      const started = Date.now();
      let seenDown = false;
      while (Date.now() - started < 120000) {
        try {
          const res = await fetch('/healthz', { cache: 'no-store' });
          if (res.ok) {
            let finished = false;
            try {
              const st = await apiJSON('/api/v1/update/status');
              const s = st.status || {};
              const current = (s.update_current_version || '').trim();
              const latest = (s.update_latest_version || '').trim();
              const apply = (s.update_last_apply_status || '').trim();
              const upToDate = current !== '' && latest !== '' && current === latest;
              const success = apply === 'success' || apply === 'noop';
              finished = upToDate || success;
            } catch (_) {}
            if (finished && !seenDown) {
              result.textContent = 'Update successful.';
              updateRestartWatchActive = false;
              return;
            }
            if (seenDown) {
              result.textContent = 'Server is back. Reloading...';
              window.location.reload();
              return;
            }
            result.textContent = 'Waiting for restart...';
          } else {
            seenDown = true;
            result.textContent = 'Server restarting...';
          }
        } catch (_) {
          seenDown = true;
          result.textContent = 'Server restarting...';
        }
        await new Promise(r => setTimeout(r, 500));
      }
      updateRestartWatchActive = false;
      result.textContent = 'Update applied; reload the page if needed.';
    }

    async function refreshUpdateStatus() {
      const box = document.getElementById('updateStatus');
      const checkBtn = document.getElementById('checkUpdatesBtn');
      const applyBtn = document.getElementById('applyUpdateBtn');
      const rollbackBtn = document.getElementById('rollbackUpdateBtn');
      const rollbackRefreshBtn = document.getElementById('refreshRollbackTagsBtn');
      const rollbackSelect = document.getElementById('rollbackTagSelect');
      const updateCapabilityNotice = document.getElementById('updateCapabilityNotice');
      const rollbackCapabilityNotice = document.getElementById('rollbackCapabilityNotice');
      try {
        const r = await apiJSON('/api/v1/update/status');
        const s = r.status || {};
        const serverUpdateSupported = String(s.update_server_self_update_supported || '').trim() === '1';
        const serverMode = String(s.update_server_mode || '').trim();
        const current = (s.update_current_version || '').trim();
        const latest = (s.update_latest_version || '').trim();
        let available = '';
        if (current && latest) {
          available = current === latest ? '0' : '1';
        } else {
          available = (s.update_available || '').trim();
        }
        const parts = [];
        if (current) parts.push('Current: ' + current);
        if (latest) parts.push('Latest: ' + latest);
        if (available === '1') parts.push('Update available');
        if (s.update_last_checked_utc) parts.push('Checked: ' + formatTimestamp(s.update_last_checked_utc));
        if (s.update_last_apply_status) parts.push('Apply: ' + s.update_last_apply_status);
        if (s.update_last_apply_utc) parts.push('Apply time: ' + formatTimestamp(s.update_last_apply_utc));
        if (s.update_message) parts.push('Message: ' + s.update_message);
        box.textContent = parts.join(' | ');
        if (updateCapabilityNotice) {
          if (serverMode === 'dev') {
            updateCapabilityNotice.textContent = 'Running in dev mode. Updates disabled.';
          } else if (!serverUpdateSupported) {
            updateCapabilityNotice.innerHTML = 'Server is not running as a service. Updates disabled. Install updates manually. See <a href="https://github.com/izzyreal/ciwi?tab=readme-ov-file#linux-server-installer-systemd" target="_blank" rel="noopener noreferrer">README</a>.';
          } else {
            updateCapabilityNotice.textContent = '';
          }
        }
        if (rollbackCapabilityNotice) {
          if (serverMode === 'dev') {
            rollbackCapabilityNotice.textContent = 'Running in dev mode. Updates disabled.';
          } else if (!serverUpdateSupported) {
            rollbackCapabilityNotice.innerHTML = 'Server is not running as a service. Updates disabled. Install updates manually. See <a href="https://github.com/izzyreal/ciwi?tab=readme-ov-file#linux-server-installer-systemd" target="_blank" rel="noopener noreferrer">README</a>.';
          } else {
            rollbackCapabilityNotice.textContent = '';
          }
        }
        if (checkBtn) checkBtn.disabled = !serverUpdateSupported;
        if (applyBtn) {
          applyBtn.style.display = (!serverUpdateSupported || available === '1') ? 'inline-block' : 'none';
          applyBtn.disabled = !serverUpdateSupported || (available !== '1');
        }
        if (rollbackSelect) rollbackSelect.disabled = !serverUpdateSupported;
        if (rollbackBtn) rollbackBtn.disabled = !serverUpdateSupported;
        if (rollbackRefreshBtn) rollbackRefreshBtn.disabled = !serverUpdateSupported;

        const blockedAgentsRaw = String(s.update_agent_non_service_agents || '').trim();
        const targetVersion = String(s.update_agent_target_version || '').trim();
        if (blockedAgentsRaw) {
          blockedAgentsRaw.split(',').map(v => String(v || '').trim()).filter(Boolean).forEach(agentID => {
            const key = agentID + '|' + targetVersion;
            if (shownAgentUpdateWarningKeys.has(key)) return;
            shownAgentUpdateWarningKeys.add(key);
            showSnackbar({
              messageHTML: 'Agent <code>' + escapeHtml(agentID) + '</code> is not running as a service. Agent self-updates are disabled on that host. Install or reinstall via the <a href="https://github.com/izzyreal/ciwi?tab=readme-ov-file#automated-installation-scripts" target="_blank" rel="noopener noreferrer">automated installation scripts</a>.',
              timeoutMs: 12000,
            });
          });
        }
      } catch (e) {
        box.textContent = 'Update status unavailable';
        if (checkBtn) checkBtn.disabled = true;
        if (applyBtn) {
          applyBtn.style.display = 'inline-block';
          applyBtn.disabled = true;
        }
        if (rollbackSelect) rollbackSelect.disabled = true;
        if (rollbackBtn) rollbackBtn.disabled = true;
        if (rollbackRefreshBtn) rollbackRefreshBtn.disabled = true;
      }
    }

    async function tick() {
      if (refreshInFlight || refreshGuard.shouldPause()) {
        return;
      }
      refreshInFlight = true;
      try {
        await Promise.all([refreshSettingsProjects(), refreshUpdateStatus(), refreshRollbackTags(false)]);
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
