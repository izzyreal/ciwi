package webui

const settingsManagedYAMLJS = `
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

`
