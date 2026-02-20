package server

const vaultHTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="UTF-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>ciwi vault</title>
  <link rel="icon" type="image/png" href="/ciwi-favicon.png" />
  <style>
` + uiPageChromeCSS + `
    .row { display:flex; flex-wrap:wrap; gap:8px; align-items:center; }
    .field { display:flex; flex-direction:column; gap:4px; min-width:180px; }
    .field label { font-size:12px; color:var(--muted); }
    input, select { border:1px solid var(--line); border-radius:8px; padding:8px 10px; font-size:14px; }
    table { width:100%; border-collapse:collapse; font-size:13px; }
    th, td { border-bottom:1px solid var(--line); text-align:left; padding:8px 6px; vertical-align:top; }
  </style>
</head>
<body>
  <main>
    <div class="card row" style="justify-content:space-between;">
      <div class="brand">
        <img src="/ciwi-logo.png" alt="ciwi logo" />
        <div>
          <div style="font-size:24px;font-weight:700;">Vault Connections</div>
          <div class="muted">Configure AppRole and test access</div>
        </div>
      </div>
      <a class="nav-btn" href="/">Back to Projects <span class="nav-emoji" aria-hidden="true">↩</span></a>
    </div>
    <div class="card">
      <h3 style="margin:0 0 10px;">Add / Update Connection</h3>
      <div class="row">
        <div class="field">
          <label for="name">Name</label>
          <input id="name" value="home-vault" />
        </div>
        <div class="field">
          <label for="url">Vault URL</label>
          <input id="url" value="http://bhakti.local:8200" style="width:260px;" />
        </div>
        <div class="field">
          <label for="roleId">AppRole Role ID</label>
          <input id="roleId" value="" style="width:260px;" />
        </div>
        <div class="field">
          <label for="mount">AppRole Mount</label>
          <input id="mount" value="approle" />
        </div>
        <div class="field">
          <label for="secretEnv">Secret ID Env</label>
          <input id="secretEnv" value="CIWI_VAULT_SECRET_ID" />
        </div>
        <button id="saveBtn">Save</button>
        <span id="saveMsg" class="muted"></span>
      </div>
    </div>
    <div class="card">
      <h3 style="margin:0 0 10px;">Connections</h3>
      <table><thead><tr><th>Name</th><th>URL</th><th>Mount</th><th>Auth</th><th>Role ID</th><th>Secret Source</th><th>Actions</th></tr></thead><tbody id="rows"></tbody></table>
    </div>
  </main>
  <script>
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
          try {
            const r = await api('/api/v1/vault/connections/' + c.id + '/test', { method: 'POST', body: '{}' });
            await showAlertDialog({
              title: r.ok ? 'Vault test OK' : 'Vault test failed',
              message: r.ok ? ('OK: ' + (r.message || '')) : ('FAILED: ' + (r.message || '')),
            });
          } catch (e) {
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
  </script>
</body>
</html>`
