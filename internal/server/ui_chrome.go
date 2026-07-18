package server

const uiPageChromeCSS = `
    :root {
      --bg: #f2f7f4;
      --bg2: #d9efe2;
      --card: #ffffff;
      --ink: #1f2a24;
      --muted: #5f6f67;
      --ok: #1f8a4c;
      --bad: #b23a48;
      --accent: #157f66;
      --line: #c4ddd0;
    }
    * { box-sizing: border-box; }
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
      background: radial-gradient(circle at 20% 0%, var(--bg2), var(--bg));
    }
    main { max-width: 1100px; margin: 24px auto; padding: 0 16px; }
    .card {
      background: var(--card);
      border: 1px solid var(--line);
      border-radius: 12px;
      padding: 16px;
      margin-bottom: 16px;
      box-shadow: 0 8px 24px rgba(21,127,102,.08);
    }
    .ciwi-progress-surface {
      position: relative;
      isolation: isolate;
      overflow: hidden;
      --ciwi-progress-width: 0%;
      --ciwi-progress-color: rgba(45, 145, 99, .13);
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
    .runtime-banner {
      margin-top: 10px;
      border: 1px solid var(--line);
      border-radius: 10px;
      padding: 8px 10px;
      font-size: 13px;
      background: #f7fcf9;
      color: #295748;
      display: none;
    }
    .runtime-banner.runtime-banner-warn {
      border-color: #e7d5aa;
      background: #fff9eb;
      color: #7c5a1f;
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
      background: #ffffff;
      color: var(--accent);
      cursor: pointer;
    }
    button:hover:not(:disabled),
    a.nav-btn:hover {
      background: #f4fbf7;
      text-decoration: none;
    }
    button:disabled {
      opacity: 0.65;
      cursor: default;
    }
    button.secondary {
      background: #ffffff;
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
      font-size: 1.28em;
      line-height: 0.9;
    }
`
