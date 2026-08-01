
    function initializeThemeSettings() {
      const select = document.getElementById('themeSelect');
      const description = document.getElementById('themeDescription');
      if (!select) return;
      const descriptions = {
        default: 'Bright mint with stronger color and contrast.',
        jungle: 'Deep forest greens with vivid tropical accents.',
        space: 'Midnight blue with cyan and violet highlights.',
        'pina-colada': 'Pineapple gold, coconut cream, leafy green, and a splash of lagoon teal.',
        'mango-kent': 'Golden-orange flesh framed by Kent mango red, yellow, and green skin.',
        'mango-chaunsa': 'Sunlit Chaunsa yellow with honey, saffron, and warm wooden notes.',
        'mango-alphonso': 'Deep Alphonso orange with burnt amber and softly creamy surfaces.',
        'yellow-dragon-fruit': 'Luminous yellow rind, cactus green, and seed-speckled ivory.',
        'dragon-fruit': 'Electric magenta skin, fresh green scales, and clean white flesh.',
      };
      const update = theme => {
        const selected = ciwiApplyTheme(theme);
        select.value = selected;
        if (description) description.textContent = descriptions[selected] || '';
      };
      select.value = ciwiStoredTheme();
      if (description) description.textContent = descriptions[select.value] || '';
      select.onchange = () => update(select.value);
      window.addEventListener('ciwi-theme-change', event => {
        const selected = ciwiNormalizeTheme(event && event.detail && event.detail.theme);
        select.value = selected;
        if (description) description.textContent = descriptions[selected] || '';
      });
    }

    function setProjectReloadState(projectId, text, color) {
      projectReloadState.set(String(projectId), { text, color });
    }

    function shortCommitHash(commit) {
      const value = String(commit || '').trim();
      if (!value) return '';
      if (value.length <= 8) return value;
      return value.slice(0, 8);
    }

    function deriveCommitURL(repoURL, commit) {
      const rawRepo = String(repoURL || '').trim();
      const sha = String(commit || '').trim();
      if (!rawRepo || !sha) return '';
      // Support github https and ssh remotes.
      if (rawRepo.indexOf('github.com') >= 0) {
        let path = rawRepo;
        path = path.replace(/^https?:\/\/github\.com\//i, '');
        path = path.replace(/^git@github\.com:/i, '');
        path = path.replace(/\.git$/i, '');
        path = path.replace(/^\/+/, '');
        if (!path) return '';
        return 'https://github.com/' + path + '/commit/' + encodeURIComponent(sha);
      }
      return '';
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
        const isManagedYAML = isManagedYAMLProject(project);
        const loadedCommit = String(project.loaded_commit || '').trim();
        const shortCommit = shortCommitHash(loadedCommit);
        const commitURL = deriveCommitURL(project.repo_url, loadedCommit);
        const lastUpdated = String(project.updated_utc || '').trim();
        const commitPart = shortCommit
          ? (commitURL
              ? ('<a class="job-link" href="' + commitURL + '" target="_blank" rel="noopener noreferrer">' + escapeHtml(shortCommit) + '</a>')
              : ('<code>' + escapeHtml(shortCommit) + '</code>'))
          : '<span class="muted">n/a</span>';
        const updatedPart = lastUpdated
          ? escapeHtml(formatTimestamp(lastUpdated))
          : '<span class="muted">n/a</span>';
        const sourceMetadata = projectSourceMetadataHTML(project);
        const updateMetadata = isManagedYAML
          ? '<div class="muted" style="margin-top:6px;">Stored in ciwi database | Last update time: ' + updatedPart + '</div>'
          : '<div class="muted" style="margin-top:6px;">Loaded commit: ' + commitPart + ' | Last update time: ' + updatedPart + '</div>';
        topInfo.innerHTML =
          '<strong>Project: <a class="job-link" href="/projects/' + project.id + '?back=' + encodeURIComponent('/settings') + '">' + escapeHtml(project.name) + '</a></strong> ' +
          sourceMetadata + updateMetadata;
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
          reloadStatus.style.color = 'var(--muted)';
        }
        const definitionBtn = document.createElement('button');
        definitionBtn.className = 'secondary';
        if (isManagedYAML) {
          definitionBtn.textContent = 'Edit YAML';
          definitionBtn.onclick = () => { void openEditManagedYAML(project); };
        } else {
          definitionBtn.textContent = 'Reload project definition from VCS';
          definitionBtn.onclick = async () => {
            setProjectReloadState(project.id, 'Reloading...', 'var(--muted)');
            reloadStatus.textContent = 'Reloading...';
            reloadStatus.style.color = 'var(--muted)';
            definitionBtn.disabled = true;
            try {
              await apiJSON('/api/v1/projects/' + project.id + '/reload', { method: 'POST', body: '{}' });
              await refreshSettingsProjects();
              setProjectReloadState(project.id, 'Reloaded successfully', 'var(--ok)');
              reloadStatus.textContent = 'Reloaded successfully';
              reloadStatus.style.color = 'var(--ok)';
            } catch (e) {
              const msg = 'Reload failed: ' + e.message;
              setProjectReloadState(project.id, msg, 'var(--bad)');
              reloadStatus.textContent = msg;
              reloadStatus.style.color = 'var(--bad)';
            } finally {
              definitionBtn.disabled = false;
            }
          };
        }
        const deleteBtn = document.createElement('button');
        deleteBtn.className = 'secondary';
        deleteBtn.textContent = 'Delete Project';
        deleteBtn.onclick = async () => {
          const projectName = String(project.name || '').trim() || 'project';
          const confirmed = await showConfirmDialog({
            title: 'Delete project',
            message:
              'Delete project "' + projectName + '"? This removes its pipelines/chains from ciwi. ' +
              'Existing job execution history remains.',
            confirmText: 'Delete',
            cancelText: 'Cancel',
            danger: true,
          });
          if (!confirmed) return;
          reloadStatus.textContent = 'Deleting...';
          reloadStatus.style.color = 'var(--muted)';
          definitionBtn.disabled = true;
          deleteBtn.disabled = true;
          try {
            await apiJSON('/api/v1/projects/' + project.id, { method: 'DELETE' });
            setProjectReloadState(project.id, 'Deleted', 'var(--ok)');
            await refreshSettingsProjects();
          } catch (e) {
            const msg = 'Delete failed: ' + String(e && e.message || e);
            setProjectReloadState(project.id, msg, 'var(--bad)');
            reloadStatus.textContent = msg;
            reloadStatus.style.color = 'var(--bad)';
            definitionBtn.disabled = false;
            deleteBtn.disabled = false;
          }
        };
        controls.appendChild(definitionBtn);
        controls.appendChild(deleteBtn);
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

    initializeThemeSettings();



    const managedYAMLState = {
      mode: 'create',
      projectId: 0,
      revision: '',
      initialYAML: '',
      busy: false,
    };

    function ensureManagedYAMLModal() {
      let overlay = document.getElementById('managedYAMLOverlay');
      if (overlay) return overlay;
      overlay = document.createElement('div');
      overlay.id = 'managedYAMLOverlay';
      overlay.className = 'ciwi-modal-overlay';
      overlay.setAttribute('aria-hidden', 'true');
      overlay.innerHTML =
        '<div class="ciwi-modal managed-yaml-modal" role="dialog" aria-modal="true" aria-label="Managed YAML editor">' +
          '<div class="ciwi-modal-head">' +
            '<div><div id="managedYAMLTitle" class="ciwi-modal-title">Add Managed YAML</div><div id="managedYAMLSubtitle" class="ciwi-modal-subtitle">Stored in the ciwi database</div></div>' +
            '<button id="managedYAMLCloseBtn" class="secondary" type="button">Close</button>' +
          '</div>' +
          '<div class="ciwi-modal-body managed-yaml-body">' +
            '<div class="row">' +
              '<button id="managedYAMLLoadFileBtn" class="secondary" type="button">Load YAML file</button>' +
              '<input id="managedYAMLFileInput" type="file" accept=".yaml,.yml,text/yaml,text/plain" style="display:none" />' +
              '<span class="muted">Repository checkout is configured explicitly with <code>vcs_source</code>.</span>' +
            '</div>' +
            '<textarea id="managedYAMLEditor" class="managed-yaml-editor" spellcheck="false" placeholder="version: 1&#10;project:&#10;  name: my-project&#10;pipelines: []"></textarea>' +
            '<div id="managedYAMLStatus" class="managed-yaml-status" role="status"></div>' +
          '</div>' +
          '<div class="managed-yaml-actions">' +
            '<button id="managedYAMLReloadBtn" class="secondary" type="button" style="display:none;margin-right:auto;">Reload latest</button>' +
            '<button id="managedYAMLCancelBtn" class="secondary" type="button">Cancel</button>' +
            '<button id="managedYAMLValidateBtn" class="secondary" type="button">Validate</button>' +
            '<button id="managedYAMLSaveBtn" type="button">Save</button>' +
          '</div>' +
        '</div>';
      document.body.appendChild(overlay);

      document.getElementById('managedYAMLCloseBtn').onclick = () => { void requestCloseManagedYAMLModal(); };
      document.getElementById('managedYAMLCancelBtn').onclick = () => { void requestCloseManagedYAMLModal(); };
      document.getElementById('managedYAMLValidateBtn').onclick = () => { void validateManagedYAML(); };
      document.getElementById('managedYAMLSaveBtn').onclick = () => { void saveManagedYAML(); };
      document.getElementById('managedYAMLReloadBtn').onclick = () => { void reloadManagedYAMLLatest(); };
      document.getElementById('managedYAMLLoadFileBtn').onclick = () => {
        const input = document.getElementById('managedYAMLFileInput');
        input.value = '';
        input.click();
      };
      document.getElementById('managedYAMLFileInput').onchange = ev => { void loadManagedYAMLFile(ev); };
      document.getElementById('managedYAMLEditor').addEventListener('input', () => {
        document.getElementById('managedYAMLReloadBtn').style.display = 'none';
        setManagedYAMLStatus('', false);
      });
      wireModalCloseBehavior(overlay, () => { void requestCloseManagedYAMLModal(); });
      return overlay;
    }

    function setManagedYAMLStatus(message, isError) {
      const status = document.getElementById('managedYAMLStatus');
      if (!status) return;
      status.textContent = String(message || '');
      status.style.color = isError ? 'var(--bad)' : 'var(--ok)';
    }

    function setManagedYAMLBusy(busy) {
      managedYAMLState.busy = !!busy;
      ['managedYAMLLoadFileBtn', 'managedYAMLCancelBtn', 'managedYAMLValidateBtn', 'managedYAMLSaveBtn', 'managedYAMLReloadBtn', 'managedYAMLCloseBtn'].forEach(id => {
        const el = document.getElementById(id);
        if (el) el.disabled = !!busy;
      });
    }

    function managedYAMLIsDirty() {
      const editor = document.getElementById('managedYAMLEditor');
      return !!editor && editor.value !== managedYAMLState.initialYAML;
    }

    async function requestCloseManagedYAMLModal() {
      if (managedYAMLState.busy) return;
      if (managedYAMLIsDirty()) {
        const discard = await showConfirmDialog({
          title: 'Discard YAML changes?',
          message: 'The YAML editor contains unsaved changes.',
          okLabel: 'Discard',
          cancelLabel: 'Keep editing',
        });
        if (!discard) return;
      }
      closeModalOverlay(document.getElementById('managedYAMLOverlay'));
    }

    function openCreateManagedYAML() {
      const overlay = ensureManagedYAMLModal();
      managedYAMLState.mode = 'create';
      managedYAMLState.projectId = 0;
      managedYAMLState.revision = '';
      managedYAMLState.initialYAML = '';
      document.getElementById('managedYAMLTitle').textContent = 'Add Managed YAML';
      document.getElementById('managedYAMLSubtitle').textContent = 'Stored in the ciwi database';
      document.getElementById('managedYAMLEditor').value = '';
      document.getElementById('managedYAMLReloadBtn').style.display = 'none';
      setManagedYAMLStatus('', false);
      openModalOverlay(overlay, 'min(900px, 94vw)', 'min(82vh, 760px)');
      setTimeout(() => document.getElementById('managedYAMLEditor').focus(), 0);
    }

    async function openEditManagedYAML(project) {
      const overlay = ensureManagedYAMLModal();
      managedYAMLState.mode = 'edit';
      managedYAMLState.projectId = Number(project.id || 0);
      managedYAMLState.revision = '';
      managedYAMLState.initialYAML = '';
      document.getElementById('managedYAMLTitle').textContent = 'Edit Managed YAML';
      document.getElementById('managedYAMLSubtitle').textContent = String(project.name || 'Project');
      document.getElementById('managedYAMLEditor').value = '';
      document.getElementById('managedYAMLReloadBtn').style.display = 'none';
      setManagedYAMLStatus('Loading...', false);
      openModalOverlay(overlay, 'min(900px, 94vw)', 'min(82vh, 760px)');
      setManagedYAMLBusy(true);
      try {
        await loadManagedYAMLDefinition(false);
      } catch (e) {
        setManagedYAMLStatus('Load failed: ' + String(e && e.message || e), true);
      } finally {
        setManagedYAMLBusy(false);
      }
    }

    async function loadManagedYAMLDefinition(confirmDiscard) {
      if (confirmDiscard && managedYAMLIsDirty()) {
        const discard = await showConfirmDialog({
          title: 'Reload latest YAML?',
          message: 'Reloading discards the current editor contents.',
          okLabel: 'Reload',
          cancelLabel: 'Keep editing',
        });
        if (!discard) return false;
      }
      const definition = await apiJSON('/api/v1/projects/' + encodeURIComponent(String(managedYAMLState.projectId)) + '/managed-yaml');
      const raw = String(definition.yaml || '');
      managedYAMLState.revision = String(definition.revision || '');
      managedYAMLState.initialYAML = raw;
      document.getElementById('managedYAMLEditor').value = raw;
      document.getElementById('managedYAMLSubtitle').textContent = String(definition.project_name || 'Project');
      document.getElementById('managedYAMLReloadBtn').style.display = 'none';
      setManagedYAMLStatus('Loaded revision ' + managedYAMLState.revision.slice(0, 12), false);
      return true;
    }

    async function reloadManagedYAMLLatest() {
      setManagedYAMLBusy(true);
      try {
        await loadManagedYAMLDefinition(true);
      } catch (e) {
        setManagedYAMLStatus('Reload failed: ' + String(e && e.message || e), true);
      } finally {
        setManagedYAMLBusy(false);
      }
    }

    async function loadManagedYAMLFile(ev) {
      const file = ev && ev.target && ev.target.files && ev.target.files[0];
      if (!file) return;
      if (file.size > 2 * 1024 * 1024) {
        setManagedYAMLStatus('File exceeds the 2 MiB limit.', true);
        return;
      }
      if (managedYAMLIsDirty()) {
        const replace = await showConfirmDialog({
          title: 'Replace editor contents?',
          message: 'Loading this file discards the current YAML editor contents.',
          okLabel: 'Replace',
          cancelLabel: 'Cancel',
        });
        if (!replace) return;
      }
      try {
        const raw = await file.text();
        document.getElementById('managedYAMLEditor').value = raw;
        setManagedYAMLStatus('Loaded ' + file.name + '. Validate or save to persist it.', false);
      } catch (e) {
        setManagedYAMLStatus('File load failed: ' + String(e && e.message || e), true);
      }
    }

    async function validateManagedYAML() {
      const raw = document.getElementById('managedYAMLEditor').value;
      if (!raw.trim()) {
        setManagedYAMLStatus('YAML is required.', true);
        return;
      }
      setManagedYAMLBusy(true);
      setManagedYAMLStatus('Validating...', false);
      try {
        const payload = { yaml: raw };
        if (managedYAMLState.mode === 'edit') payload.project_id = managedYAMLState.projectId;
        const result = await apiJSON('/api/v1/projects/managed-yaml/validate', {
          method: 'POST',
          body: JSON.stringify(payload),
        });
        setManagedYAMLStatus('Valid: ' + result.project_name + ' — ' + result.pipelines + ' pipeline(s), ' + result.pipeline_chains + ' chain(s).', false);
      } catch (e) {
        setManagedYAMLStatus('Validation failed:\n' + String(e && e.message || e).trim(), true);
      } finally {
        setManagedYAMLBusy(false);
      }
    }

    async function saveManagedYAML() {
      const editor = document.getElementById('managedYAMLEditor');
      const raw = editor.value;
      if (!raw.trim()) {
        setManagedYAMLStatus('YAML is required.', true);
        return;
      }
      setManagedYAMLBusy(true);
      setManagedYAMLStatus('Validating and saving...', false);
      try {
        let result;
        if (managedYAMLState.mode === 'edit') {
          result = await apiJSON('/api/v1/projects/' + encodeURIComponent(String(managedYAMLState.projectId)) + '/managed-yaml', {
            method: 'PUT',
            body: JSON.stringify({ yaml: raw, revision: managedYAMLState.revision }),
          });
        } else {
          result = await apiJSON('/api/v1/projects/managed-yaml', {
            method: 'POST',
            body: JSON.stringify({ yaml: raw }),
          });
          managedYAMLState.projectId = Number(result.project_id || 0);
          managedYAMLState.mode = 'edit';
        }
        managedYAMLState.revision = String(result.revision || '');
        managedYAMLState.initialYAML = raw;
        setManagedYAMLStatus('Saved ' + String(result.project_name || 'project') + '.', false);
        await refreshSettingsProjects();
        closeModalOverlay(document.getElementById('managedYAMLOverlay'));
      } catch (e) {
        const message = String(e && e.message || e).trim();
        setManagedYAMLStatus('Save failed:\n' + message, true);
        if (message.toLowerCase().indexOf('changed since it was loaded') >= 0) {
          document.getElementById('managedYAMLReloadBtn').style.display = 'inline-block';
        }
      } finally {
        setManagedYAMLBusy(false);
      }
    }

    document.getElementById('addManagedYAMLBtn').onclick = openCreateManagedYAML;



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

    function setAvailableUpdateVersions(versions, preferredVersion) {
      const select = document.getElementById('updateVersionSelect');
      if (!select) return;
      const previous = String(select.value || '').trim();
      const unique = [];
      const seen = new Set();
      (Array.isArray(versions) ? versions : []).forEach(version => {
        const value = String(version || '').trim();
        if (!value || seen.has(value)) return;
        seen.add(value);
        unique.push(value);
      });
      select.innerHTML = '';
      if (unique.length === 0) {
        const opt = document.createElement('option');
        opt.value = '';
        opt.textContent = 'No newer versions available';
        select.appendChild(opt);
        select.disabled = true;
        return;
      }
      unique.forEach(version => {
        const opt = document.createElement('option');
        opt.value = version;
        opt.textContent = version;
        select.appendChild(opt);
      });
      const preferred = String(preferredVersion || '').trim();
      if (previous && seen.has(previous)) {
        select.value = previous;
      } else if (preferred && seen.has(preferred)) {
        select.value = preferred;
      } else {
        select.selectedIndex = 0;
      }
      select.disabled = false;
    }

    document.getElementById('checkUpdatesBtn').onclick = async () => {
      const result = document.getElementById('updateResult');
      result.textContent = 'Checking...';
      try {
        const r = await apiJSON('/api/v1/update/check', { method: 'POST', body: '{}' });
        const latest = r.latest_version || '';
        const current = r.current_version || '';
        setAvailableUpdateVersions(r.available_versions, latest);
        if (r.update_available) {
          result.textContent = 'Update available: ' + current + ' -> ' + latest + (r.asset_name ? (' (' + r.asset_name + ')') : '');
        } else {
          result.textContent = r.message || ('Up to date (' + current + ')');
        }
      } catch (e) {
        setAvailableUpdateVersions([], '');
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
      const select = document.getElementById('updateVersionSelect');
      const target = String((select && select.value) || '').trim();
      if (!target) {
        result.textContent = 'Check for updates and select a version first.';
        return;
      }
      // DEBUG(apply-update-confirm)
      logApplyUpdateDebug('click', {
        click_id: clickId,
        is_trusted: !!(ev && ev.isTrusted),
        detail: Number((ev && ev.detail) || 0),
      });
      const confirmed = await showConfirmDialog({
        title: 'Apply Update',
        message: 'Update server and agents to ' + target + ' and restart ciwi?',
        okLabel: 'Apply update',
      });
      logApplyUpdateDebug('confirm_result', { click_id: clickId, confirmed: !!confirmed });
      if (!confirmed) return;
      result.textContent = 'Starting update...';
      logApplyUpdateDebug('request_begin', { click_id: clickId, path: '/api/v1/update/apply', target_version: target });
      try {
        const body = JSON.stringify({ target_version: target });
        const r = await postJSONWithTimeout('/api/v1/update/apply', body, 30000);
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
      const updateSelect = document.getElementById('updateVersionSelect');
      const updateCapabilityNotice = document.getElementById('updateCapabilityNotice');
      const rollbackCapabilityNotice = document.getElementById('rollbackCapabilityNotice');
      try {
        const r = await apiJSON('/api/v1/update/status');
        const s = r.status || {};
        const serverUpdateSupported = String(s.update_server_self_update_supported || '').trim() === '1';
        const serverMode = String(s.update_server_mode || '').trim();
        const current = (s.update_current_version || '').trim();
        const latest = (s.update_latest_version || '').trim();
        let available = (s.update_available || '').trim();
        if (current && latest && current === latest) available = '0';
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
          applyBtn.disabled = !serverUpdateSupported || (available !== '1') || !updateSelect || !updateSelect.value;
        }
        if (updateSelect) updateSelect.disabled = !serverUpdateSupported || !updateSelect.value;
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
        if (updateSelect) updateSelect.disabled = true;
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
    refreshServerVersionLabels();
    tick();
    setInterval(tick, 3000);
