package server

const indexHTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="UTF-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>ciwi</title>
  <link rel="icon" type="image/png" href="/ciwi-favicon.png" />
  <style>
` + uiPageChromeCSS + uiIndexCSS + `
  </style>
</head>
<body>
  <main>
    <div class="card">
      <div class="header">
        <div class="brand">
          <img src="/ciwi-logo.png" alt="ciwi logo" />
          <div>
            <h1>ciwi</h1>
            <p>Projects, pipelines and job executions</p>
          </div>
        </div>
        <div class="header-actions">
          <a class="nav-btn" href="/agents">Agents <span class="nav-emoji" aria-hidden="true">🖥️</span></a>
          <a class="nav-btn" href="/settings" aria-label="Global Settings" title="Global Settings">Global Settings <span class="nav-emoji" aria-hidden="true">⚙️</span></a>
        </div>
      </div>
    </div>
    <div class="card">
      <h2>Projects</h2>
      <div id="projects"></div>
    </div>
    <div class="card">
      <h2 id="queued-jobs">Queued Job Executions</h2>
      <div class="row" style="margin-bottom:10px;">
        <button id="clearQueueBtn" class="secondary">Clear Queue</button>
      </div>
      <table>
        <thead>
          <tr><th>Job</th><th>Status</th><th>Pipeline</th><th>Build</th><th>Agent</th><th>Created</th><th>Reason</th><th>Actions</th></tr>
        </thead>
        <tbody id="queuedJobsBody"></tbody>
      </table>
    </div>
    <div class="card">
      <h2>Job Execution History</h2>
      <div class="row" style="margin-bottom:10px;">
        <button id="flushHistoryBtn" class="secondary">Flush History</button>
      </div>
      <table>
        <thead>
          <tr><th>Job</th><th>Status</th><th>Pipeline</th><th>Build</th><th>Agent</th><th>Created</th><th>Duration</th></tr>
        </thead>
        <tbody id="historyJobsBody"></tbody>
      </table>
    </div>
  </main>
  <script src="/ui/shared.js"></script>
  <script src="/ui/pages.js"></script>
  <script>
` + uiIndexStateJS + uiIndexProjectsJS + uiIndexJobExecutionsJS + uiIndexBootJS + `
  </script>
</body>
</html>`
