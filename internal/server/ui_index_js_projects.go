package server

const uiIndexProjectsJS = `
    async function refreshProjects() {
      const data = await apiJSON('/api/v1/projects');
      const root = document.getElementById('projects');
      if (!data.projects || data.projects.length === 0) {
        root.innerHTML = '<p>No projects loaded yet.</p>';
        return;
      }
      root.innerHTML = '';
      data.projects.forEach(project => {
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
              await apiJSON('/api/v1/pipelines/' + p.id + '/run', { method: 'POST', body: '{}' });
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
              await apiJSON('/api/v1/pipelines/' + p.id + '/run', { method: 'POST', body: JSON.stringify({ dry_run: true }) });
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
          body.appendChild(row);
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
              await apiJSON('/api/v1/pipeline-chains/' + c.id + '/run', { method: 'POST', body: '{}' });
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
              await apiJSON('/api/v1/pipeline-chains/' + c.id + '/run', { method: 'POST', body: JSON.stringify({ dry_run: true }) });
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
          body.appendChild(row);
        });

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
