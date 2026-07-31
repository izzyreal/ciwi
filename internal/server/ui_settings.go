package server

const settingsHTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="UTF-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>ciwi global settings</title>
  <link rel="icon" type="image/png" href="/ciwi-favicon.png" />
  <script src="/ui/theme.js"></script>
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
    .version-select {
      min-width: 220px;
      height: 34px;
      border: 1px solid var(--line);
      border-radius: 8px;
      padding: 0 38px 0 10px;
      font: inherit;
      font-size: 14px;
      line-height: 1.1;
      appearance: none;
      -webkit-appearance: none;
      background-color: var(--input-bg);
      background-image: url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='12' height='8' viewBox='0 0 12 8'%3E%3Cpath d='M1 1l5 5 5-5' fill='none' stroke='%23536b60' stroke-width='1' stroke-linecap='round' stroke-linejoin='round'/%3E%3C/svg%3E");
      background-repeat: no-repeat;
      background-position: right 11px center;
      background-size: 12px 8px;
      color: var(--ink);
    }
    :root[data-ciwi-theme="jungle"] .version-select {
      background-image: url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='12' height='8' viewBox='0 0 12 8'%3E%3Cpath d='M1 1l5 5 5-5' fill='none' stroke='%23abc4a5' stroke-width='1' stroke-linecap='round' stroke-linejoin='round'/%3E%3C/svg%3E");
    }
    :root[data-ciwi-theme="space"] .version-select {
      background-image: url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='12' height='8' viewBox='0 0 12 8'%3E%3Cpath d='M1 1l5 5 5-5' fill='none' stroke='%23aab4d5' stroke-width='1' stroke-linecap='round' stroke-linejoin='round'/%3E%3C/svg%3E");
    }
    .version-select:disabled {
      opacity: 0.65;
      cursor: default;
      color: var(--muted);
      background-color: var(--surface-soft);
    }
    .version-action-row > button,
    .version-action-row > .version-select {
      height: 34px;
    }
    .row { display: flex; gap: 8px; flex-wrap: wrap; align-items: center; }
    .top { display:flex; justify-content:space-between; align-items:center; gap:8px; flex-wrap:wrap; }
    .top-nav { margin-left: auto; justify-content: flex-end; }
    .project { border-top: 1px solid var(--line); padding-top: 10px; margin-top: 10px; }
    .project-head { display:flex; justify-content: space-between; gap:10px; align-items:center; flex-wrap:wrap; }
    .pill { font-size: 12px; padding: 2px 8px; border-radius: 999px; background: var(--pill-bg); color: var(--pill-ink); }
    a.job-link { color: var(--accent); }
    .managed-yaml-modal { grid-template-rows:auto 1fr auto; }
    .managed-yaml-body { display:grid; grid-template-rows:auto 1fr auto; gap:10px; min-height:0; }
    .managed-yaml-editor { width:100%; height:100%; min-height:300px; resize:none; box-sizing:border-box; border:1px solid var(--line); border-radius:8px; padding:12px; font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,monospace; font-size:13px; line-height:1.45; tab-size:2; }
    .managed-yaml-status { white-space:pre-wrap; overflow:auto; min-height:20px; max-height:110px; color:var(--muted); font-size:12px; }
    .managed-yaml-actions { border-top:1px solid var(--line); padding:10px 12px; display:flex; justify-content:flex-end; align-items:center; gap:8px; flex-wrap:wrap; }
    .split-row { display:grid; grid-template-columns: 1fr 1fr; gap: 12px; }
    @media (max-width: 980px) {
      .split-row { grid-template-columns: 1fr; }
    }
  </style>
</head>
<body>
  <main>
    <div class="card top">
      <div class="brand">
        <img src="/ciwi-logo.png" alt="ciwi logo" />
        <div>
          <h1>ciwi <span class="ciwi-header-version" data-ciwi-server-version></span> global settings</h1>
        </div>
      </div>
      <div class="row top-nav">
        <a class="nav-btn" href="/"><span class="nav-emoji" aria-hidden="true"><svg class="ciwi-icon" focusable="false"><use href="/ui/icons.svg#icon-arrow-left"></use></svg></span> Back to Main</a>
        <a class="nav-btn" href="/agents">Agents <span class="nav-emoji" aria-hidden="true"><svg class="ciwi-icon" focusable="false"><use href="/ui/icons.svg#icon-device-desktop"></use></svg></span></a>
        <a id="restartServerBtn" class="nav-btn" href="#" role="button">Restart Server <span class="nav-emoji" aria-hidden="true"><svg class="ciwi-icon" focusable="false"><use href="/ui/icons.svg#icon-refresh"></use></svg></span></a>
      </div>
    </div>

    <div class="card">
      <h2>Appearance</h2>
      <p>Choose the color theme for ciwi in this browser.</p>
      <div class="row">
        <label for="themeSelect"><strong>Theme</strong></label>
        <select id="themeSelect" class="version-select">
          <optgroup label="Core themes">
            <option value="default">Default</option>
            <option value="jungle">Jungle</option>
            <option value="space">Space</option>
          </optgroup>
          <optgroup label="Fruit themes">
            <option value="pina-colada">Piña Colada</option>
            <option value="mango-kent">Mango Kent</option>
            <option value="mango-chaunsa">Mango Chaunsa</option>
            <option value="mango-alphonso">Mango Alphonso</option>
            <option value="yellow-dragon-fruit">Yellow Dragon Fruit</option>
            <option value="dragon-fruit">Dragon Fruit</option>
          </optgroup>
        </select>
        <span id="themeDescription" class="muted"></span>
      </div>
    </div>

    <div class="card">
      <h2>Projects</h2>
      <p>Import a definition from a repository or store and edit a self-contained YAML definition directly in ciwi.</p>
      <div class="row">
        <input id="repoUrl" placeholder="https://github.com/you/project.git" style="width:380px" />
        <input id="repoRef" placeholder="ref (optional: main, tag, sha)" />
        <input id="configFile" value="ciwi-project.yaml" />
        <button id="importProjectBtn">Add Repository Project</button>
        <button id="addManagedYAMLBtn" class="secondary" type="button">Add Managed YAML</button>
        <span id="importResult"></span>
      </div>
      <div id="settingsProjects" style="margin-top:12px;"></div>
    </div>

    <div class="split-row">
      <div class="card">
        <h2>Server Updates</h2>
        <div class="row version-action-row">
          <button id="checkUpdatesBtn" class="secondary">Check for updates</button>
          <select id="updateVersionSelect" class="version-select" disabled>
            <option value="">Check for updates</option>
          </select>
          <button id="applyUpdateBtn" class="secondary">Update now</button>
          <span id="updateResult" class="muted"></span>
        </div>
        <div id="updateCapabilityNotice" class="muted" style="margin-top:8px;"></div>
        <p style="margin-top:8px;">
          Agents automatically update following a server update. Each agent first finishes already queued/running jobs before applying the new agent version.
        </p>
        <div id="updateStatus" class="muted" style="margin-top:8px;"></div>
      </div>
      <div class="card">
        <h2>Rollback</h2>
        <div class="row version-action-row">
          <select id="rollbackTagSelect" class="version-select"></select>
          <button id="refreshRollbackTagsBtn" class="secondary">Refresh tags</button>
          <button id="rollbackUpdateBtn" class="secondary">Rollback</button>
          <span id="rollbackResult" class="muted"></span>
        </div>
        <div id="rollbackCapabilityNotice" class="muted" style="margin-top:8px;"></div>
        <div id="rollbackHint" class="muted" style="margin-top:8px;">
          Shows only versions lower than the current server version.
        </div>
      </div>
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
` + settingsRenderJS + `
` + settingsManagedYAMLJS + `
` + settingsUpdateJS + `
  </script>
</body>
</html>`
