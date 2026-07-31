package server

const uiIndexCSS = `
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
    .header { display: flex; justify-content: space-between; align-items: center; gap: 12px; }
    .header-actions { display: flex; align-items: center; gap: 12px; }
    .project-group {
      border: 1px solid var(--line);
      border-radius: 12px;
      margin-top: 10px;
      background: var(--surface);
      overflow: hidden;
    }
    .project-group > summary {
      list-style: none;
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: 12px;
      cursor: pointer;
      padding: 10px 12px;
      background: var(--surface-subtle);
    }
    .project-group > summary::-webkit-details-marker { display: none; }
    .project-group > summary:focus:not(:focus-visible),
    .ciwi-job-group-details > summary:focus:not(:focus-visible) { outline: none; }
    .project-group > summary:focus-visible,
    .ciwi-job-group-details > summary:focus-visible { outline: 3px solid var(--focus-ring); outline-offset: -3px; }
    .project-group-toggle {
      color: var(--muted);
      font-size: 14px;
      line-height: 1;
      min-width: 14px;
      text-align: center;
    }
    .project-group-toggle .ciwi-icon { transition: transform .15s ease; }
    .project-group[open] .project-group-toggle .ciwi-icon { transform: rotate(90deg); }
    .project-head { display:flex; justify-content: space-between; gap:10px; align-items:center; flex-wrap:wrap; min-width:0; flex:1 1 auto; }
    .project-body {
      margin-top: 0;
      padding: 8px 12px 10px;
      border-top: 1px solid var(--line);
    }
    .project-body-layout {
      display: grid;
      grid-template-columns: 88px 1fr;
      gap: 12px;
      align-items: start;
    }
    .project-icon-col {
      display: flex;
      justify-content: center;
      align-items: center;
      align-self: center;
      min-height: 72px;
    }
    .project-icon {
      width: 72px;
      height: 72px;
      object-fit: contain;
      border: none;
      background: transparent;
      image-rendering: pixelated;
      image-rendering: crisp-edges;
    }
    .project-pipelines-col { min-width: 0; }
    .pipeline { display: flex; justify-content: space-between; gap: 8px; padding: 8px 0; }
    .pipeline-actions { display:flex; flex-direction:column; gap:6px; align-items:flex-end; }
    .pill { font-size: 12px; padding: 2px 8px; border-radius: 999px; background: var(--pill-bg); color: var(--pill-ink); }
    table { width: 100%; border-collapse: collapse; font-size: 13px; table-layout: fixed; }
    th, td {
      text-align: left;
      padding: 8px 6px;
      vertical-align: top;
      overflow-wrap: anywhere;
      word-break: break-word;
    }
    th { border-bottom: none; }
    td { border-bottom: 1px solid var(--line); }
    table th:first-child { padding-left: 10px; }
    table tbody tr:not(.ciwi-job-group-row):not(.ciwi-empty-row) td:first-child { padding-left: 10px; }
    td code { white-space: pre-wrap; max-height: 80px; overflow: auto; display: block; max-width: 100%; overflow-wrap: anywhere; word-break: break-word; }
    .status-succeeded { color: var(--ok); font-weight: 600; }
    .status-failed { color: var(--bad); font-weight: 600; }
    .status-blocked { color: var(--warn); font-weight: 600; }
    .status-running { color: var(--warn); font-weight: 600; }
    .status-queued, .status-leased, .status-waiting { color: var(--muted); }
    .ciwi-empty-row td { border-bottom: none; }
    .ciwi-job-group-row td { padding: 4px 0; border-bottom: none; }
    .ciwi-job-group-details {
      margin: 0;
      border: 1px solid var(--line);
      border-radius: 10px;
      overflow: hidden;
      background: var(--surface);
      width: 100%;
      box-sizing: border-box;
    }
    .ciwi-job-group-details > summary {
      list-style: none;
      display: flex;
      align-items: center;
      justify-content: flex-start;
      gap: 10px;
      cursor: pointer;
      background: var(--surface-subtle);
      padding: 10px 10px;
    }
    .ciwi-job-group-details > summary::-webkit-details-marker { display: none; }
    .ciwi-job-group-toggle {
      color: var(--muted);
      font-size: 14px;
      line-height: 1;
      min-width: 14px;
      text-align: center;
      flex: 0 0 auto;
    }
    .ciwi-job-group-toggle .ciwi-icon { transition: transform .15s ease; }
    .ciwi-job-group-details[open] .ciwi-job-group-toggle .ciwi-icon { transform: rotate(90deg); }
    .ciwi-job-group-main {
      display: inline-flex;
      align-items: center;
      gap: 8px;
      min-width: 0;
      font-weight: 600;
      flex: 1 1 auto;
    }
    .ciwi-job-group-title {
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
    }
    .ciwi-job-group-status-icon { display:inline-flex; align-items:center; justify-content:center; width:17px; height:17px; font-size:15px; line-height:1; flex:0 0 auto; }
    .ciwi-job-group-status-icon .ciwi-icon { width:17px; height:17px; }
    .ciwi-job-group-status { font-size: 12px; white-space: nowrap; flex: 0 0 auto; }
    .ciwi-job-group-side-icon {
      width: 28px;
      height: 28px;
      object-fit: contain;
      border: none;
      flex: 0 0 auto;
      background: transparent;
      image-rendering: pixelated;
      image-rendering: crisp-edges;
    }
    .ciwi-job-desc {
      display: inline-flex;
      align-items: center;
      gap: 6px;
      min-width: 0;
    }
    .ciwi-project-mini-icon {
      width: 16px;
      height: 16px;
      object-fit: contain;
      border: none;
      flex: 0 0 auto;
      background: transparent;
      image-rendering: pixelated;
      image-rendering: crisp-edges;
    }
    .ciwi-job-group-table { width: 100%; border-collapse: collapse; }
    .ciwi-job-group-card {
      margin: 0;
      border: 1px solid var(--line);
      border-radius: 10px;
      overflow: hidden;
      background: var(--surface);
      width: 100%;
      box-sizing: border-box;
    }
    .ciwi-job-group-head {
      display: flex;
      align-items: center;
      justify-content: flex-start;
      gap: 10px;
      background: var(--surface-subtle);
      padding: 10px 10px;
    }
    .ciwi-job-group-skel-head {
      display: flex;
      align-items: center;
      justify-content: flex-start;
      gap: 10px;
      background: var(--surface-subtle);
      padding: 10px 10px;
    }
    .ciwi-job-group-skel-body {
      padding: 8px 10px;
      border-top: 1px solid var(--line);
      display: grid;
      gap: 8px;
    }
    .ciwi-job-history-sections {
      display: grid;
      gap: 0;
      border-top: 1px solid var(--line);
    }
    .ciwi-job-history-section + .ciwi-job-history-section {
      border-top: 1px solid var(--line);
    }
    .ciwi-job-history-section-head {
      padding: 8px 10px;
      background: var(--surface-subtle);
      color: var(--muted);
      font-size: 12px;
      font-weight: 600;
      text-transform: none;
      letter-spacing: .01em;
    }
    .ciwi-job-history-matrix-head {
      padding: 8px 10px;
      background: var(--surface);
      color: var(--muted);
      font-size: 12px;
      font-weight: 600;
    }
    .ciwi-job-history-empty-card {
      padding: 10px;
      color: var(--muted);
      border-top: 1px solid var(--line);
      font-size: 13px;
    }
    a.job-link { color: var(--accent); }
    @media (max-width: 760px) {
      table { font-size: 12px; }
      .project-body-layout { grid-template-columns: 1fr; }
      .project-icon-col { justify-content: flex-start; }
      .project-icon { width: 56px; height: 56px; }
    }
`
