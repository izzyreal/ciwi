package server

const settingsHTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="UTF-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>ciwi global settings</title>
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
    .top { display:flex; justify-content:space-between; align-items:center; gap:8px; flex-wrap:wrap; }
    .top-nav { margin-left: auto; justify-content: flex-end; }
    .project { border-top: 1px solid var(--line); padding-top: 10px; margin-top: 10px; }
    .project-head { display:flex; justify-content: space-between; gap:10px; align-items:center; flex-wrap:wrap; }
    .pill { font-size: 12px; padding: 2px 8px; border-radius: 999px; background: #edf8f2; color: #26644b; }
    a.job-link { color: var(--accent); }
    .split-row { display:grid; grid-template-columns: 1fr 1fr; gap: 12px; }
    #restartServerBtn .nav-emoji {
      display: inline-block;
      transform: scale(1.5);
      transform-origin: center;
    }
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
          <h1>ciwi global settings</h1>
        </div>
      </div>
      <div class="row top-nav">
        <a class="nav-btn" href="/">Back to Main <span class="nav-emoji" aria-hidden="true">↩</span></a>
        <a class="nav-btn" href="/agents">Agents <span class="nav-emoji" aria-hidden="true">🖥️</span></a>
        <a id="restartServerBtn" class="nav-btn" href="#" role="button">Restart Server <span class="nav-emoji" aria-hidden="true">⟳</span></a>
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

    <div class="split-row">
      <div class="card">
        <h2>Server Updates</h2>
        <div class="row">
          <button id="checkUpdatesBtn" class="secondary">Check for updates</button>
          <button id="applyUpdateBtn" class="secondary">Update now</button>
          <span id="updateResult" style="color:#5f6f67;"></span>
        </div>
        <div id="updateCapabilityNotice" style="margin-top:8px;color:#5f6f67;font-size:12px;"></div>
        <p style="margin-top:8px;">
          Agents automatically update following a server update. Each agent first finishes already queued/running jobs before applying the new agent version.
        </p>
        <div id="updateStatus" style="margin-top:8px;color:#5f6f67;font-size:12px;"></div>
      </div>
      <div class="card">
        <h2>Rollback</h2>
        <div class="row">
          <select id="rollbackTagSelect" style="min-width:220px;"></select>
          <button id="refreshRollbackTagsBtn" class="secondary">Refresh tags</button>
          <button id="rollbackUpdateBtn" class="secondary">Rollback</button>
          <span id="rollbackResult" style="color:#5f6f67;"></span>
        </div>
        <div id="rollbackCapabilityNotice" style="margin-top:8px;color:#5f6f67;font-size:12px;"></div>
        <div id="rollbackHint" style="margin-top:8px;color:#5f6f67;font-size:12px;">
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
` + settingsUpdateJS + `
  </script>
</body>
</html>`
