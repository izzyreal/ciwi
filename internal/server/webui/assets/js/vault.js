    async function api(path, opts={}) {
      const res = await fetch(path, { headers: { 'Content-Type': 'application/json' }, ...opts });
      if (!res.ok) throw new Error(await res.text() || ('HTTP ' + res.status));
      return await res.json();
    }
    async function refresh() {
      const data = await api('/api/v1/vault/connections');
      const body = document.getElementById('rows');
      body.innerHTML = '';
      for (const c of (data.connections || [])) {
        const tr = document.createElement('tr');
        tr.innerHTML = '<td><code>' + (c.name || '') + '</code></td>' +
          '<td>' + (c.url || '') + '</td>' +
          '<td>' + (c.approle_mount || '') + '</td>' +
          '<td>' + (c.auth_method || '') + '</td>' +
          '<td><code>' + (c.role_id || '') + '</code></td>' +
          '<td>' + ((c.secret_id_env || '') ? ('env:' + c.secret_id_env) : '') + '</td>' +
          '<td></td>';
        const td = tr.lastChild;
        const testBtn = document.createElement('button');
        testBtn.className = 'secondary';
        testBtn.textContent = 'Test';
        testBtn.onclick = async () => {
          testBtn.disabled = true;
          const msg = document.getElementById('vaultActionMsg');
          msg.textContent = 'Testing ' + (c.name || '') + '...';
          try {
            const r = await api('/api/v1/vault/connections/' + c.id + '/test', { method: 'POST', body: '{}' });
            msg.textContent = (r.ok ? 'OK: ' : 'FAILED: ') + String(r.message || '');
            await showAlertDialog({
              title: r.ok ? 'Vault test OK' : 'Vault test failed',
              message: r.ok ? ('OK: ' + (r.message || '')) : ('FAILED: ' + (r.message || '')),
            });
          } catch (e) {
            msg.textContent = 'Test failed: ' + e.message;
            await showAlertDialog({ title: 'Vault test failed', message: 'Test failed: ' + e.message });
          } finally { testBtn.disabled = false; }
        };
        const delBtn = document.createElement('button');
        delBtn.className = 'secondary';
        delBtn.textContent = 'Delete';
        delBtn.onclick = async () => {
          const confirmed = await showConfirmDialog({
            title: 'Delete Vault Connection',
            message: 'Delete connection?',
            okLabel: 'Delete',
          });
          if (!confirmed) return;
          delBtn.disabled = true;
          try {
            await api('/api/v1/vault/connections/' + c.id, { method: 'DELETE' });
            await refresh();
          } catch (e) {
            await showAlertDialog({ title: 'Delete failed', message: String(e.message || e) });
          } finally { delBtn.disabled = false; }
        };
        td.appendChild(testBtn);
        td.appendChild(delBtn);
        body.appendChild(tr);
      }
    }
    document.getElementById('saveBtn').onclick = async () => {
      const msg = document.getElementById('saveMsg');
      msg.textContent = 'Saving...';
      try {
        await api('/api/v1/vault/connections', { method: 'POST', body: JSON.stringify({
          name: document.getElementById('name').value.trim(),
          url: document.getElementById('url').value.trim(),
          auth_method: 'approle',
          approle_mount: document.getElementById('mount').value.trim() || 'approle',
          role_id: document.getElementById('roleId').value.trim(),
          secret_id_env: document.getElementById('secretEnv').value.trim()
        })});
        msg.textContent = 'Saved';
        await refresh();
      } catch (e) {
        msg.textContent = 'Error: ' + e.message;
      }
    };
    refresh();
