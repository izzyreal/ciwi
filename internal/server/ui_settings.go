package server

const settingsHTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="UTF-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>ciwi settings</title>
  <link rel="icon" type="image/png" href="/ciwi-favicon.png" />
  <style>
` + uiPageChromeCSS + `
    h1 { margin: 0 0 4px; font-size: 28px; }
    h2 { margin: 0 0 12px; font-size: 18px; }
    p { margin: 0 0 10px; color: var(--muted); }
    input {
      border: 1px solid var(--line);
      border-radius: 8px;
      padding: 9px 12px;
      font-size: 14px;
    }
    input { width: 280px; max-width: 100%; }
    .row { display: flex; gap: 8px; flex-wrap: wrap; align-items: center; }
    .project { border-top: 1px solid var(--line); padding-top: 10px; margin-top: 10px; }
    .project-head { display:flex; justify-content: space-between; gap:10px; align-items:center; flex-wrap:wrap; }
    .pill { font-size: 12px; padding: 2px 8px; border-radius: 999px; background: #edf8f2; color: #26644b; }
    a.job-link { color: var(--accent); }
  </style>
</head>
<body>
  <main>
    <div class="card">
      <div class="brand">
        <img src="/ciwi-logo.png" alt="ciwi logo" />
        <div>
          <h1>ciwi settings</h1>
          <p>Global configuration</p>
        </div>
      </div>
      <div class="row">
        <a class="nav-btn" href="/">Back to Main <span class="nav-emoji" aria-hidden="true">↩</span></a>
        <a class="nav-btn" href="/agents">Agents <span class="nav-emoji" aria-hidden="true">🖥️</span></a>
      </div>
    </div>

    <div class="card">
      <h2>Root Projects</h2>
      <div class="row">
        <input id="repoUrl" placeholder="https://github.com/you/project.git" style="width:380px" />
        <input id="repoRef" placeholder="ref (optional: main, tag, sha)" />
        <input id="configFile" value="ciwi-project.yaml" />
        <button id="importProjectBtn">Add Project</button>
        <span id="importResult"></span>
      </div>
      <div id="settingsProjects" style="margin-top:12px;"></div>
    </div>

    <div class="card">
      <h2>Server Updates</h2>
      <div class="row">
        <button id="checkUpdatesBtn" class="secondary">Check for updates</button>
        <button id="applyUpdateBtn" class="secondary">Update now</button>
        <select id="rollbackTagSelect" style="min-width:220px;"></select>
        <button id="refreshRollbackTagsBtn" class="secondary">Refresh tags</button>
        <button id="rollbackUpdateBtn" class="secondary">Rollback</button>
        <span id="updateResult" style="color:#5f6f67;"></span>
      </div>
      <div id="updateStatus" style="margin-top:8px;color:#5f6f67;font-size:12px;"></div>
    </div>

    <div class="card">
      <h2>Vault Connections</h2>
      <p>Manage global Vault AppRole connections and test connectivity.</p>
      <button id="openVaultConnectionsBtn" class="secondary" type="button">Open Vault Connections</button>
    </div>
  </main>
  <script src="/ui/shared.js"></script>
  <script src="/ui/pages.js"></script>
  <script>
    let refreshInFlight = false;
    const refreshGuard = createRefreshGuard(5000);
    const projectReloadState = new Map();
    let updateRestartWatchActive = false;
    let rollbackTagsLoadedAt = 0;

    function setProjectReloadState(projectId, text, color) {
      projectReloadState.set(String(projectId), { text, color });
    }

    async function refreshSettingsProjects() {
      const data = await apiJSON('/api/v1/projects');
      const root = document.getElementById('settingsProjects');
      if (!data.projects || data.projects.length === 0) {
        root.innerHTML = '<p>No projects loaded yet.</p>';
        return;
      }
      root.innerHTML = '';
      data.projects.forEach(project => {
        const wrap = document.createElement('div');
        wrap.className = 'project';
        const top = document.createElement('div');
        top.className = 'project-head';

        const topInfo = document.createElement('div');
        topInfo.innerHTML =
          '<strong>Project: <a class="job-link" href="/projects/' + project.id + '">' + escapeHtml(project.name) + '</a></strong> ' +
          '<span class="pill">' + escapeHtml(project.repo_url || '') + '</span> ' +
          '<span class="pill">' + escapeHtml(project.config_file || project.config_path || '') + '</span>';
        top.appendChild(topInfo);

        const controls = document.createElement('div');
        controls.className = 'row';
        const reloadStatus = document.createElement('span');
        reloadStatus.style.fontSize = '12px';
        const state = projectReloadState.get(String(project.id));
        if (state) {
          reloadStatus.textContent = state.text;
          reloadStatus.style.color = state.color;
        } else {
          reloadStatus.style.color = '#5f6f67';
        }
        const reloadBtn = document.createElement('button');
        reloadBtn.className = 'secondary';
        reloadBtn.textContent = 'Reload project definition from VCS';
        reloadBtn.onclick = async () => {
          setProjectReloadState(project.id, 'Reloading...', '#5f6f67');
          reloadStatus.textContent = 'Reloading...';
          reloadStatus.style.color = '#5f6f67';
          reloadBtn.disabled = true;
          try {
            await apiJSON('/api/v1/projects/' + project.id + '/reload', { method: 'POST', body: '{}' });
            await refreshSettingsProjects();
            setProjectReloadState(project.id, 'Reloaded successfully', '#1f8a4c');
            reloadStatus.textContent = 'Reloaded successfully';
            reloadStatus.style.color = '#1f8a4c';
          } catch (e) {
            const msg = 'Reload failed: ' + e.message;
            setProjectReloadState(project.id, msg, '#b23a48');
            reloadStatus.textContent = msg;
            reloadStatus.style.color = '#b23a48';
          } finally {
            reloadBtn.disabled = false;
          }
        };
        controls.appendChild(reloadBtn);
        controls.appendChild(reloadStatus);
        top.appendChild(controls);
        wrap.appendChild(top);
        root.appendChild(wrap);
      });
    }

    document.getElementById('importProjectBtn').onclick = async () => {
      const repoUrl = (document.getElementById('repoUrl').value || '').trim();
      const repoRef = (document.getElementById('repoRef').value || '').trim();
      const configFile = (document.getElementById('configFile').value || 'ciwi-project.yaml').trim();
      const result = document.getElementById('importResult');
      if (!repoUrl) {
        result.textContent = 'Repo URL required';
        return;
      }
      result.textContent = 'Importing...';
      try {
        await apiJSON('/api/v1/projects/import', {
          method: 'POST',
          body: JSON.stringify({ repo_url: repoUrl, repo_ref: repoRef, config_file: configFile }),
        });
        result.textContent = 'Imported';
        await refreshSettingsProjects();
      } catch (e) {
        result.textContent = 'Error: ' + e.message;
      }
    };

    document.getElementById('openVaultConnectionsBtn').onclick = () => {
      window.location.href = '/vault';
    };

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
      const now = Date.now();
      if (!force && rollbackTagsLoadedAt > 0 && (now - rollbackTagsLoadedAt) < 60000 && select.options.length > 0) {
        return;
      }
      const prev = select.value;
      try {
        const r = await apiJSON('/api/v1/update/tags');
        const tags = Array.isArray(r.tags) ? r.tags : [];
        select.innerHTML = '';
        if (tags.length === 0) {
          const opt = document.createElement('option');
          opt.value = '';
          opt.textContent = 'No tags available';
          select.appendChild(opt);
          rollbackTagsLoadedAt = now;
          return;
        }
        tags.forEach(tag => {
          const v = (tag || '').trim();
          if (!v) return;
          const opt = document.createElement('option');
          opt.value = v;
          opt.textContent = v;
          select.appendChild(opt);
        });
        const hasPrev = prev && Array.from(select.options).some(o => o.value === prev);
        if (hasPrev) {
          select.value = prev;
        } else if (r.current_version && Array.from(select.options).some(o => o.value === r.current_version)) {
          select.value = r.current_version;
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

    document.getElementById('applyUpdateBtn').onclick = async () => {
      const result = document.getElementById('updateResult');
      if (!confirm('Apply update now and restart ciwi?')) return;
      result.textContent = 'Starting update...';
      try {
        const r = await postJSONWithTimeout('/api/v1/update/apply', '{}', 30000);
        result.textContent = (r.message || 'Update started. Refresh in a moment.');
        if (r.updated) {
          waitForServerRestartAndReload();
        }
      } catch (e) {
        if (e && e.name === 'AbortError') {
          result.textContent = 'Update request timed out; checking status...';
          await monitorApplyProgressAfterTimeout();
        } else {
          result.textContent = 'Update failed: ' + e.message;
        }
      }
      await refreshUpdateStatus();
    };

    document.getElementById('refreshRollbackTagsBtn').onclick = async () => {
      await refreshRollbackTags(true);
    };

    document.getElementById('rollbackUpdateBtn').onclick = async () => {
      const result = document.getElementById('updateResult');
      const select = document.getElementById('rollbackTagSelect');
      const target = ((select && select.value) || '').trim();
      if (!target) {
        result.textContent = 'Select a rollback tag first.';
        return;
      }
      if (!confirm('Rollback server and agents to ' + target + '?')) return;
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
      try {
        const r = await apiJSON('/api/v1/update/status');
        const s = r.status || {};
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
      } catch (e) {
        box.textContent = 'Update status unavailable';
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
  </script>
</body>
</html>`
