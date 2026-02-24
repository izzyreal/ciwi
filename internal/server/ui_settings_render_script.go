package server

const settingsRenderJS = `
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
        const loadedCommit = String(project.loaded_commit || '').trim();
        const projectRepoRef = String(project.repo_ref || '').trim() || 'default';
        const shortCommit = shortCommitHash(loadedCommit);
        const commitURL = deriveCommitURL(project.repo_url, loadedCommit);
        const lastUpdated = String(project.updated_utc || '').trim();
        const commitPart = shortCommit
          ? (commitURL
              ? ('<a class="job-link" href="' + commitURL + '" target="_blank" rel="noopener noreferrer">' + escapeHtml(shortCommit) + '</a>')
              : ('<code>' + escapeHtml(shortCommit) + '</code>'))
          : '<span style="color:#5f6f67;">n/a</span>';
        const updatedPart = lastUpdated
          ? escapeHtml(formatTimestamp(lastUpdated))
          : '<span style="color:#5f6f67;">n/a</span>';
        topInfo.innerHTML =
          '<strong>Project: <a class="job-link" href="/projects/' + project.id + '">' + escapeHtml(project.name) + '</a></strong> ' +
          '<span class="pill">' + escapeHtml(project.repo_url || '') + '</span> ' +
          '<span class="pill">' + escapeHtml('branch:' + projectRepoRef) + '</span> ' +
          '<span class="pill">' + escapeHtml(project.config_file || project.config_path || '') + '</span>' +
          '<div style="margin-top:6px;font-size:12px;color:#3a4f44;">Loaded commit: ' + commitPart + ' | Last update time: ' + updatedPart + '</div>';
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

`
