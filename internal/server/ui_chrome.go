package server

const uiPageChromeCSS = uiFruitThemeCSS + `
    :root {
      --bg: #edf7f1;
      --bg2: #bce8d0;
      --bg3: #dff2e8;
      --bg-glow-a: #79d5aa;
      --bg-glow-b: #b8dcf0;
      --card: #ffffff;
      --card-glow: #e0f7eb;
      --ink: #17271f;
      --muted: #536b60;
      --ok: #13834a;
      --bad: #c03b4d;
      --accent: #087d61;
      --accent-strong: #075f4b;
      --line: #abd4bf;
      --surface: #ffffff;
      --surface-soft: #f1faf5;
      --surface-subtle: #f7fcf9;
      --surface-hover: #e6f6ee;
      --input-bg: #ffffff;
      --code-bg: #eaf6ef;
      --code-line: #cae3d5;
      --pill-bg: #dff4e9;
      --pill-ink: #176247;
      --focus-ring: rgba(0, 132, 96, .34);
      --shadow: rgba(6, 95, 72, .13);
      --overlay: rgba(7, 31, 22, .52);
      --warn: #a76900;
      --warn-bg: #fff3d6;
      --warn-line: #e4c06c;
      --ok-bg: #e8f8ef;
      --ok-line: #87c7a4;
      --bad-bg: #fff0f2;
      --bad-line: #e0a0aa;
      --info-bg: #edf6ff;
      --info-line: #96bedc;
      --graph-bg-start: #fbfffd;
      --graph-bg-end: #edf8f2;
      --graph-glow: #d9f4e6;
      --graph-node-bg: #ffffff;
      --graph-node-border: #9fcab5;
      --graph-selected-border: #087d61;
      --graph-selected-inset: rgba(8, 125, 97, .20);
      --graph-running-bg: #fff5dc;
      --graph-running-border: #d7ad4f;
      --graph-succeeded-bg: #eaf9f0;
      --graph-succeeded-border: #6eb991;
      --graph-failed-bg: #fff0f2;
      --graph-failed-border: #d77b89;
      --graph-waiting-bg: #f9f1df;
      --graph-waiting-border: #c6a363;
      --graph-skipped-bg: #edf5fa;
      --graph-skipped-border: #7ca7bd;
      --graph-edge: #609b80;
      --console-bg: #0e1713;
      --console-surface: #17261f;
      --console-glow: #234b38;
      --console-line: #294238;
      --console-ink: #d5eee2;
      --console-muted: #98b4a7;
      --console-accent: #a8d7c1;
      --console-blue: #82d6ff;
      --console-green: #9ce397;
      --console-yellow: #ffd477;
      --console-warn: #ffc273;
      --snackbar-bg: #113a2b;
      --snackbar-ink: #effbf4;
      --snackbar-line: #3d775e;
      --page-background: radial-gradient(circle at 12% -10%, color-mix(in srgb, var(--bg-glow-a) 86%, transparent) 0%, transparent 38%), radial-gradient(circle at 90% 8%, color-mix(in srgb, var(--bg-glow-b) 82%, transparent) 0%, transparent 34%), linear-gradient(145deg, var(--bg2) 0%, var(--bg) 48%, var(--bg3) 100%);
      --card-background: radial-gradient(circle at 100% 0%, var(--card-glow) 0%, transparent 38%), linear-gradient(145deg, var(--card) 0%, var(--surface-subtle) 100%);
      --graph-background: radial-gradient(circle at 18% 0%, var(--graph-glow) 0%, transparent 42%), linear-gradient(155deg, var(--graph-bg-start) 0%, var(--graph-bg-end) 100%);
      --console-background: radial-gradient(circle at 92% 0%, var(--console-glow) 0%, transparent 44%), linear-gradient(150deg, var(--console-bg) 0%, var(--console-surface) 130%);
    }
    :root[data-ciwi-theme="jungle"] {
      --bg: #061a12;
      --bg2: #22552f;
      --bg3: #0d3020;
      --bg-glow-a: #4c954f;
      --bg-glow-b: #b38a25;
      --card: #0d2a1d;
      --card-glow: #1e4b2b;
      --ink: #f0f8dc;
      --muted: #abc4a5;
      --ok: #75e08e;
      --bad: #ff7b7b;
      --accent: #a3e635;
      --accent-strong: #c7f36b;
      --line: #356044;
      --surface: #113524;
      --surface-soft: #153d29;
      --surface-subtle: #0f3021;
      --surface-hover: #1d4b31;
      --input-bg: #0a2418;
      --code-bg: #173d29;
      --code-line: #386449;
      --pill-bg: #204b2e;
      --pill-ink: #c9f27a;
      --focus-ring: rgba(163, 230, 53, .42);
      --shadow: rgba(0, 0, 0, .34);
      --overlay: rgba(0, 10, 5, .72);
      --warn: #ffd166;
      --warn-bg: #493814;
      --warn-line: #8e7029;
      --ok-bg: #143d25;
      --ok-line: #3f8756;
      --bad-bg: #452222;
      --bad-line: #914747;
      --info-bg: #163b38;
      --info-line: #3f7770;
      --graph-bg-start: #0b271a;
      --graph-bg-end: #102f20;
      --graph-glow: #244e2c;
      --graph-node-bg: #123824;
      --graph-node-border: #447156;
      --graph-selected-border: #b5ed55;
      --graph-selected-inset: rgba(181, 237, 85, .30);
      --graph-running-bg: #493912;
      --graph-running-border: #d5aa3e;
      --graph-succeeded-bg: #123d25;
      --graph-succeeded-border: #52a76b;
      --graph-failed-bg: #452222;
      --graph-failed-border: #c65f5f;
      --graph-waiting-bg: #383119;
      --graph-waiting-border: #9d843d;
      --graph-skipped-bg: #18383b;
      --graph-skipped-border: #4c878c;
      --graph-edge: #78a987;
      --console-bg: #04110b;
      --console-surface: #0c2618;
      --console-glow: #173d23;
      --console-line: #285139;
      --console-ink: #e1f2d0;
      --console-muted: #9fbc9c;
      --console-accent: #b8e27a;
      --console-blue: #72d5cf;
      --console-green: #a3e635;
      --console-yellow: #ffd166;
      --console-warn: #ff9f68;
      --snackbar-bg: #1d4a2b;
      --snackbar-ink: #f2fbdc;
      --snackbar-line: #65904c;
    }
    :root[data-ciwi-theme="space"] {
      --bg: #080d21;
      --bg2: #2b1855;
      --bg3: #101c3d;
      --bg-glow-a: #6137a3;
      --bg-glow-b: #167eaa;
      --card: #121a35;
      --card-glow: #242052;
      --ink: #f0f3ff;
      --muted: #aab4d5;
      --ok: #52e2a2;
      --bad: #ff6f91;
      --accent: #65d5ff;
      --accent-strong: #a8e9ff;
      --line: #39466f;
      --surface: #17203e;
      --surface-soft: #1b2748;
      --surface-subtle: #141d39;
      --surface-hover: #24345c;
      --input-bg: #0e1630;
      --code-bg: #20294b;
      --code-line: #43517d;
      --pill-bg: #26365d;
      --pill-ink: #84e3ff;
      --focus-ring: rgba(101, 213, 255, .45);
      --shadow: rgba(0, 0, 0, .42);
      --overlay: rgba(3, 5, 18, .76);
      --warn: #ffd166;
      --warn-bg: #453718;
      --warn-line: #8c7134;
      --ok-bg: #123b39;
      --ok-line: #3b8876;
      --bad-bg: #48213a;
      --bad-line: #a44e72;
      --info-bg: #172f54;
      --info-line: #426b9b;
      --graph-bg-start: #10182f;
      --graph-bg-end: #171e3b;
      --graph-glow: #292557;
      --graph-node-bg: #1a2445;
      --graph-node-border: #4d5f91;
      --graph-selected-border: #65d5ff;
      --graph-selected-inset: rgba(101, 213, 255, .28);
      --graph-running-bg: #463719;
      --graph-running-border: #cda749;
      --graph-succeeded-bg: #123a39;
      --graph-succeeded-border: #48a88d;
      --graph-failed-bg: #472039;
      --graph-failed-border: #c5587d;
      --graph-waiting-bg: #343049;
      --graph-waiting-border: #7f78a2;
      --graph-skipped-bg: #19324e;
      --graph-skipped-border: #4f81a9;
      --graph-edge: #7187be;
      --console-bg: #070b1a;
      --console-surface: #111936;
      --console-glow: #202652;
      --console-line: #303d68;
      --console-ink: #dce7ff;
      --console-muted: #98a7cb;
      --console-accent: #8ddfff;
      --console-blue: #72c9ff;
      --console-green: #72e6bc;
      --console-yellow: #ffd166;
      --console-warn: #ff9b8d;
      --snackbar-bg: #202b56;
      --snackbar-ink: #f0f5ff;
      --snackbar-line: #53669a;
    }
    * { box-sizing: border-box; }
    ::selection { background: var(--accent); color: var(--card); }
    :where(body, main, .card, p, h1, h2, h3, div, span, table, thead, tbody, tr, th, td, code, pre, input, textarea, select, label, a) {
      -webkit-user-select: text;
      user-select: text;
    }
    :where(button) {
      -webkit-user-select: none;
      user-select: none;
    }
    body {
      margin: 0;
      font-family: "Avenir Next", "Segoe UI", sans-serif;
      color: var(--ink);
      background: var(--page-background);
      background-attachment: fixed;
    }
    input, textarea, select { background-color: var(--input-bg); color: var(--ink); }
    main { max-width: 1100px; margin: 24px auto; padding: 0 16px; }
    .card {
      background: var(--card-background);
      border: 1px solid var(--line);
      border-radius: 12px;
      padding: 16px;
      margin-bottom: 16px;
      box-shadow: 0 8px 24px var(--shadow);
    }
    .ciwi-progress-surface {
      position: relative;
      isolation: isolate;
      overflow: hidden;
      --ciwi-progress-width: 0%;
      --ciwi-progress-color: color-mix(in srgb, var(--ok) 18%, transparent);
      --ciwi-progress-animation-delay: 0ms;
    }
    .ciwi-progress-surface::before {
      content: "";
      position: absolute;
      z-index: 0;
      inset: 0 auto 0 0;
      width: var(--ciwi-progress-width);
      background: var(--ciwi-progress-color);
      pointer-events: none;
      transition: width .25s linear, opacity .2s ease;
    }
    .ciwi-progress-surface > * { position: relative; z-index: 1; }
    .ciwi-progress-indeterminate::before {
      width: 22%;
      animation: ciwi-progress-scan 2s ease-in-out infinite alternate;
      animation-delay: var(--ciwi-progress-animation-delay);
    }
    .ciwi-progress-overrun::before {
      width: 100%;
      animation: ciwi-progress-pulse 2s ease-in-out infinite;
      animation-delay: var(--ciwi-progress-animation-delay);
    }
    @keyframes ciwi-progress-scan {
      from { left: 0; }
      to { left: 78%; }
    }
    @keyframes ciwi-progress-pulse {
      0%, 100% { opacity: .58; }
      50% { opacity: 1; }
    }
    @media (prefers-reduced-motion: reduce) {
      .ciwi-progress-surface::before { transition: none; animation: none; }
      .ciwi-progress-indeterminate::before { left: 39%; opacity: .72; }
      .ciwi-progress-overrun::before { opacity: .82; }
    }
    .brand { display: flex; align-items: center; gap: 12px; min-width: 0; flex: 1 1 auto; }
    .brand > div { min-width: 0; flex: 1 1 auto; }
    .brand img {
      width: 110px;
      height: 91px;
      object-fit: contain;
      display: block;
      flex: 0 0 auto;
      image-rendering: crisp-edges;
      image-rendering: pixelated;
    }
    .muted { color: var(--muted); font-size: 13px; }
    .ciwi-header-version {
      display: inline-flex;
      align-items: center;
      margin-left: .22em;
      padding: 2px 7px;
      border: 1px solid var(--line);
      border-radius: 999px;
      background: var(--pill-bg);
      color: var(--pill-ink);
      font-size: .42em;
      font-weight: 700;
      line-height: 1.2;
      vertical-align: .24em;
      white-space: nowrap;
    }
    .ciwi-header-version:empty { display:none; }
    .runtime-banner {
      margin-top: 10px;
      border: 1px solid var(--line);
      border-radius: 10px;
      padding: 8px 10px;
      font-size: 13px;
      background: var(--surface-subtle);
      color: var(--accent-strong);
      display: none;
    }
    .runtime-banner.runtime-banner-warn {
      border-color: var(--warn-line);
      background: var(--warn-bg);
      color: var(--warn);
    }
    .ciwi-modal-overlay { display: none; }
    a { color: var(--accent); text-decoration: none; }
    a:hover { text-decoration: underline; }
    button,
    a.nav-btn {
      border: 1px solid var(--line);
      border-radius: 8px;
      padding: 8px 10px;
      font-size: 14px;
      line-height: 1.1;
      background: var(--input-bg);
      color: var(--accent);
      cursor: pointer;
    }
    button:hover:not(:disabled),
    a.nav-btn:hover {
      background: var(--surface-hover);
      text-decoration: none;
    }
    button:disabled {
      opacity: 0.65;
      cursor: default;
    }
    .ciwi-icon {
      display: inline-block;
      width: 1.15em;
      height: 1.15em;
      flex: 0 0 auto;
      vertical-align: -0.2em;
      color: currentColor;
      fill: none;
      stroke: currentColor;
      stroke-width: 2;
      stroke-linecap: round;
      stroke-linejoin: round;
    }
    .ciwi-icon-only {
      display: inline-flex;
      align-items: center;
      justify-content: center;
    }
    .ciwi-icon-spin { animation: ciwi-icon-spin 1.1s linear infinite; }
    @keyframes ciwi-icon-spin { to { transform: rotate(360deg); } }
    @media (prefers-reduced-motion: reduce) {
      .ciwi-icon-spin { animation: none; }
    }
    button.secondary {
      background: var(--input-bg);
      color: var(--accent);
    }
    a.nav-btn {
      display: inline-flex;
      align-items: center;
      gap: 6px;
      font-weight: 600;
    }
    a.nav-btn .nav-emoji {
      display: inline-flex;
      align-items: center;
      justify-content: center;
      font-size: 1.15em;
      line-height: 1;
    }
`
