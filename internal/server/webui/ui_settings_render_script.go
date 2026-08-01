package webui

const settingsRenderJS = `
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

`
