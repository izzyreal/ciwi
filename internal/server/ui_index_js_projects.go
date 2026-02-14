package server

const uiIndexProjectsJS = `
    function projectListSignature(projects) {
      const list = Array.isArray(projects) ? projects : [];
      return JSON.stringify(list.map(project => ({
        id: project.id,
        name: project.name,
        repo_url: project.repo_url,
        config_file: project.config_file || project.config_path || '',
        pipelines: (project.pipelines || []).map(p => ({
          id: p.id,
          pipeline_id: p.pipeline_id,
          trigger: p.trigger || '',
          depends_on: p.depends_on || [],
          source_repo: p.source_repo || '',
          supports_dry_run: !!p.supports_dry_run,
        })),
        pipeline_chains: (project.pipeline_chains || []).map(c => ({
          id: c.id,
          chain_id: c.chain_id,
          pipelines: c.pipelines || [],
          supports_dry_run: !!c.supports_dry_run,
          version_pipeline_id: c.version_pipeline_id || 0,
        })),
      })));
    }

    async function refreshProjects() {
      const data = await apiJSON('/api/v1/projects');
      const root = document.getElementById('projects');
      if (!data.projects || data.projects.length === 0) {
        lastProjectsSignature = '';
        root.innerHTML = '<p>No projects loaded yet.</p>';
        return;
      }
      const signature = projectListSignature(data.projects);
      if (signature === lastProjectsSignature) {
        return;
      }
      lastProjectsSignature = signature;
      root.innerHTML = '';
      data.projects.forEach(project => {
        const projectIconURL = '/api/v1/projects/' + encodeURIComponent(project.id) + '/icon';
        projectIconURLByName[String(project.name || '').trim()] = projectIconURL;
        const projectKey = String(project.id || project.name || '');
        const details = document.createElement('details');
        details.className = 'project-group';
        details.open = !projectGroupCollapsed.has(projectKey);
        const summary = document.createElement('summary');
        const top = document.createElement('div');
        top.className = 'project-head';
        const topInfo = document.createElement('div');
        topInfo.innerHTML = '<strong>Project: <a class="job-link" href="/projects/' + project.id + '">' + project.name + '</a></strong> <span class="pill">' + (project.repo_url || '') + '</span> <span class="pill">' + (project.config_file || project.config_path || '') + '</span>';
        const topRight = document.createElement('div');
        topRight.innerHTML = '<span class="pill">' + String((project.pipelines || []).length) + ' pipeline(s)</span>';
        top.appendChild(topInfo);
        top.appendChild(topRight);
        summary.appendChild(top);
        const toggle = document.createElement('span');
        toggle.className = 'project-group-toggle';
        toggle.setAttribute('aria-hidden', 'true');
        summary.appendChild(toggle);
        details.appendChild(summary);

        const body = document.createElement('div');
        body.className = 'project-body';
        const layout = document.createElement('div');
        layout.className = 'project-body-layout';
        const iconCol = document.createElement('div');
        iconCol.className = 'project-icon-col';
        const icon = document.createElement('img');
        icon.className = 'project-icon';
        icon.alt = '';
        icon.src = projectIconURL;
        icon.onerror = () => { icon.style.display = 'none'; };
        iconCol.appendChild(icon);
        const listCol = document.createElement('div');
        listCol.className = 'project-pipelines-col';
        (project.pipelines || []).forEach(p => {
          const row = document.createElement('div');
          row.className = 'pipeline';
          const deps = (p.depends_on || []).join(', ');
          const info = document.createElement('div');
          info.innerHTML = '<div><span class="muted">Pipeline:</span> <code>' + p.pipeline_id + '</code></div><div style="color:#5f6f67;font-size:12px;">' +
            (p.source_repo || '') + (deps ? (' | depends_on: ' + deps) : '') + '</div>';

          const btn = document.createElement('button');
          btn.className = 'secondary';
          btn.textContent = 'Run';
          btn.onclick = async () => {
            btn.disabled = true;
            try {
              const resp = await apiJSON('/api/v1/pipelines/' + p.id + '/run', { method: 'POST', body: '{}' });
              showQueuedJobsSnackbar((project.name || 'Project') + ' ' + (p.pipeline_id || 'pipeline') + ' started');
              await refreshJobs();
            } catch (e) {
              alert('Run failed: ' + e.message);
            } finally {
              btn.disabled = false;
            }
          };

          const supportsDryRun = !!p.supports_dry_run;
          const dryBtn = document.createElement('button');
          dryBtn.className = 'secondary';
          dryBtn.textContent = 'Dry Run';
          dryBtn.onclick = async () => {
            dryBtn.disabled = true;
            try {
              const resp = await apiJSON('/api/v1/pipelines/' + p.id + '/run', { method: 'POST', body: JSON.stringify({ dry_run: true }) });
              showQueuedJobsSnackbar((project.name || 'Project') + ' ' + (p.pipeline_id || 'pipeline') + ' started');
              await refreshJobs();
            } catch (e) {
              alert('Dry run failed: ' + e.message);
            } finally {
              dryBtn.disabled = false;
            }
          };

          const resolveBtn = document.createElement('button');
          resolveBtn.className = 'secondary';
          resolveBtn.textContent = 'Resolve Upcoming Build Version';
          resolveBtn.onclick = () => openVersionResolveModal(p.id, p.pipeline_id);

          row.appendChild(info);
          const actions = document.createElement('div');
          actions.className = 'pipeline-actions';
          const btnRow = document.createElement('div');
          btnRow.className = 'row';
          btnRow.appendChild(btn);
          if (supportsDryRun) btnRow.appendChild(dryBtn);
          btnRow.appendChild(resolveBtn);
          actions.appendChild(btnRow);
          row.appendChild(actions);
          listCol.appendChild(row);
        });

        (project.pipeline_chains || []).forEach(c => {
          const row = document.createElement('div');
          row.className = 'pipeline';
          const info = document.createElement('div');
          const chainPipes = (c.pipelines || []).join(' -> ');
          info.innerHTML = '<div><span class="muted">Chain:</span> <code>' + c.chain_id + '</code></div><div style="color:#5f6f67;font-size:12px;">' + chainPipes + '</div>';

          const runBtn = document.createElement('button');
          runBtn.className = 'secondary';
          runBtn.textContent = 'Run';
          runBtn.onclick = async () => {
            runBtn.disabled = true;
            try {
              const resp = await apiJSON('/api/v1/pipeline-chains/' + c.id + '/run', { method: 'POST', body: '{}' });
              showQueuedJobsSnackbar((project.name || 'Project') + ' ' + (c.chain_id || 'chain') + ' started');
              await refreshJobs();
            } catch (e) {
              alert('Run failed: ' + e.message);
            } finally {
              runBtn.disabled = false;
            }
          };

          const dryBtn = document.createElement('button');
          dryBtn.className = 'secondary';
          dryBtn.textContent = 'Dry Run';
          dryBtn.onclick = async () => {
            dryBtn.disabled = true;
            try {
              const resp = await apiJSON('/api/v1/pipeline-chains/' + c.id + '/run', { method: 'POST', body: JSON.stringify({ dry_run: true }) });
              showQueuedJobsSnackbar((project.name || 'Project') + ' ' + (c.chain_id || 'chain') + ' started');
              await refreshJobs();
            } catch (e) {
              alert('Dry run failed: ' + e.message);
            } finally {
              dryBtn.disabled = false;
            }
          };

          const resolveBtn = document.createElement('button');
          resolveBtn.className = 'secondary';
          resolveBtn.textContent = 'Resolve Upcoming Build Version';
          const versionPipelineID = Number(c.version_pipeline_id || 0);
          if (versionPipelineID > 0) {
            resolveBtn.onclick = () => openVersionResolveModal(versionPipelineID, c.chain_id);
          } else {
            resolveBtn.disabled = true;
          }

          const actions = document.createElement('div');
          actions.className = 'pipeline-actions';
          const btnRow = document.createElement('div');
          btnRow.className = 'row';
          btnRow.appendChild(runBtn);
          if (c.supports_dry_run) btnRow.appendChild(dryBtn);
          btnRow.appendChild(resolveBtn);
          actions.appendChild(btnRow);
          row.appendChild(info);
          row.appendChild(actions);
          listCol.appendChild(row);
        });

        layout.appendChild(iconCol);
        layout.appendChild(listCol);
        body.appendChild(layout);
        details.appendChild(body);
        details.addEventListener('toggle', () => {
          if (details.open) {
            projectGroupCollapsed.delete(projectKey);
          } else {
            projectGroupCollapsed.add(projectKey);
          }
          saveStringSet(PROJECT_GROUPS_STORAGE_KEY, projectGroupCollapsed);
        });
        root.appendChild(details);
      });
    }
`
