package server

const uiSharedIconsJS = `
const ciwiIconNames = new Set([
  'arrow-left',
  'arrow-up',
  'chevron-down',
  'chevron-right',
  'chevron-up',
  'chevrons-down',
  'chevrons-up',
  'circle-check',
  'circle-x',
  'clock',
  'device-desktop',
  'info-circle',
  'loader-2',
  'player-play',
  'refresh',
  'settings',
  'zoom-in',
  'zoom-out',
]);

function ciwiNormalizedIconName(name) {
  const normalized = String(name || '').trim();
  return ciwiIconNames.has(normalized) ? normalized : '';
}

function ciwiIconHTML(name, options) {
  const normalized = ciwiNormalizedIconName(name);
  if (!normalized) return '';
  const opts = options || {};
  const classes = ['ciwi-icon'];
  String(opts.className || '').split(/\s+/).forEach(className => {
    if (/^[a-zA-Z0-9_-]+$/.test(className)) classes.push(className);
  });
  return '<svg class="' + classes.join(' ') + '" aria-hidden="true" focusable="false"><use href="/ui/icons.svg#icon-' + normalized + '"></use></svg>';
}

function ciwiIconElement(name, options) {
  const normalized = ciwiNormalizedIconName(name);
  if (!normalized) return null;
  const opts = options || {};
  const svg = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
  svg.classList.add('ciwi-icon');
  String(opts.className || '').split(/\s+/).forEach(className => {
    if (/^[a-zA-Z0-9_-]+$/.test(className)) svg.classList.add(className);
  });
  svg.setAttribute('aria-hidden', 'true');
  svg.setAttribute('focusable', 'false');
  const use = document.createElementNS('http://www.w3.org/2000/svg', 'use');
  use.setAttribute('href', '/ui/icons.svg#icon-' + normalized);
  svg.appendChild(use);
  return svg;
}
`
