
const ciwiThemeStorageKey = 'ciwi.ui.theme.v1';
const ciwiThemeNames = new Set([
  'default',
  'jungle',
  'space',
  'pina-colada',
  'pina-colada-dark',
  'mango-kent',
  'mango-kent-dark',
  'mango-chaunsa',
  'mango-chaunsa-dark',
  'mango-alphonso',
  'mango-alphonso-dark',
  'yellow-dragon-fruit',
  'yellow-dragon-fruit-dark',
  'dragon-fruit',
  'dragon-fruit-dark',
  'cherimoya',
  'cherimoya-dark',
  'durian',
  'durian-dark',
  'rambutan',
  'rambutan-dark',
  'lychee',
  'lychee-dark',
]);
const ciwiDarkThemeNames = new Set([
  'jungle', 'space', 'pina-colada-dark', 'mango-kent-dark', 'mango-chaunsa-dark',
  'mango-alphonso-dark', 'yellow-dragon-fruit-dark', 'dragon-fruit-dark',
  'cherimoya-dark', 'durian-dark', 'rambutan-dark', 'lychee-dark',
]);
let ciwiThemeContractsPromise;
let ciwiThemeContractDocuments = null;
const ciwiThemeDimensionVariables = {
  small: '--ciwi-space-small', medium: '--ciwi-space-medium', large: '--ciwi-space-large',
  page: '--ciwi-page-max', 'page-inset': '--ciwi-page-inset',
  'section-padding': '--ciwi-section-padding', 'card-padding': '--ciwi-card-padding',
  'hero-padding': '--ciwi-hero-padding', 'surface-radius': '--ciwi-surface-radius',
  'control-radius': '--ciwi-control-radius', 'control-padding-x': '--ciwi-control-padding-x',
  'control-padding-y': '--ciwi-control-padding-y', 'text-body': '--ciwi-text-body',
  'text-control': '--ciwi-text-control', 'text-code': '--ciwi-text-code',
  'text-badge': '--ciwi-text-badge', 'text-subtitle': '--ciwi-text-subtitle',
  'text-heading': '--ciwi-text-heading', 'text-title': '--ciwi-text-title',
  'image-brand-width': '--ciwi-image-brand-width', 'image-brand-height': '--ciwi-image-brand-height',
};

function ciwiVersionedUIResource(path) {
  return typeof window.ciwiUIResourceURL === 'function' ? window.ciwiUIResourceURL(path) : path;
}

function ciwiThemeContracts() {
  if (!ciwiThemeContractsPromise) {
    ciwiThemeContractsPromise = fetch(ciwiVersionedUIResource('/ui/contracts/themes.json'))
      .then(response => response.ok ? response.json() : Promise.reject(new Error('theme contracts unavailable')))
	  .then(documents => {
		ciwiThemeContractDocuments = documents;
		return documents;
	  })
      .catch(() => []);
  }
  return ciwiThemeContractsPromise;
}

function ciwiThemeGradientCSS(gradient) {
  if (!gradient || !Array.isArray(gradient.stops)) return '';
  const stops = gradient.stops.map(stop => stop.color + ' ' + String(stop.position) + '%').join(', ');
  return gradient.kind === 'radial'
    ? 'radial-gradient(circle, ' + stops + ')'
    : 'linear-gradient(' + String(gradient.angle || 135) + 'deg, ' + stops + ')';
}

function ciwiApplyContractThemeDocuments(documents, requestedName) {
	ciwiThemeContractDocuments = documents;
    const name = ciwiNormalizeTheme(requestedName || document.documentElement.getAttribute('data-ciwi-theme'));
    if (document.documentElement.getAttribute('data-ciwi-theme') !== name) return;
    const documentTheme = documents.find(item => item && item.metadata && item.metadata.name === name)
      || documents.find(item => item && item.metadata && item.metadata.name === 'default');
    const theme = documentTheme && documentTheme.theme;
    const colors = theme && theme.colors;
    if (!colors || !theme) return;
    const mapping = {
      background: '--bg', 'background-start': '--bg2', 'background-end': '--bg3',
      'background-glow-a': '--bg-glow-a', 'background-glow-b': '--bg-glow-b',
      surface: '--surface', 'surface-raised': '--surface-raised', 'surface-subtle': '--surface-subtle',
      'surface-glow': '--card-glow', text: '--ink', 'text-muted': '--muted',
      accent: '--accent', 'accent-strong': '--accent-strong', border: '--line',
      success: '--ok', warning: '--warn', danger: '--bad',
      'awaiting-surface': '--awaiting-bg', 'awaiting-border': '--awaiting-line',
      'awaiting-text': '--awaiting-ink',
      'pill-background': '--pill-bg', 'pill-text': '--pill-ink',
      'notice-background': '--snackbar-bg', 'notice-text': '--snackbar-ink', 'notice-border': '--snackbar-line',
      'console-background': '--console-bg', 'console-surface': '--console-surface',
      'console-border': '--console-line', 'console-text': '--console-ink',
      'console-muted': '--console-muted', 'console-accent': '--console-accent',
      'console-success': '--console-green',
    };
    const style = document.documentElement.style;
    Object.entries(mapping).forEach(([token, variable]) => {
      if (colors[token]) style.setProperty(variable, colors[token]);
    });
    if (colors.surface) style.setProperty('--card', colors.surface);
    Object.entries(ciwiThemeDimensionVariables).forEach(([token, variable]) => {
      if (theme.dimensions && theme.dimensions[token] !== undefined) {
        style.setProperty(variable, String(theme.dimensions[token]) + 'px');
      }
    });
    const page = ciwiThemeGradientCSS(theme.gradients && theme.gradients.page);
    const hero = ciwiThemeGradientCSS(theme.gradients && theme.gradients.hero);
    if (page) style.setProperty('--page-background', page);
    if (hero) style.setProperty('--chrome-card-bg', hero);
    if (colors['surface-glow']) {
      style.setProperty('--ciwi-card-background', 'radial-gradient(circle at 100% 0%, var(--card-glow) 0%, transparent 38%), linear-gradient(145deg, var(--surface) 0%, var(--surface-subtle) 100%)');
      style.setProperty('--chrome-card-bg', 'var(--ciwi-card-background)');
    }
}

function ciwiApplyContractTheme(name) {
	if (ciwiThemeContractDocuments) {
	  ciwiApplyContractThemeDocuments(ciwiThemeContractDocuments, name);
	  return;
	}
  ciwiThemeContracts().then(documents => ciwiApplyContractThemeDocuments(documents, name));
}

function ciwiNormalizeTheme(theme) {
  const normalized = String(theme || '').trim().toLowerCase();
  return ciwiThemeNames.has(normalized) ? normalized : 'default';
}

function ciwiStoredTheme() {
  try { return ciwiNormalizeTheme(localStorage.getItem(ciwiThemeStorageKey)); } catch (_) { return 'default'; }
}

function ciwiApplyTheme(theme, persist) {
  const normalized = ciwiNormalizeTheme(theme);
  document.documentElement.setAttribute('data-ciwi-theme', normalized);
  document.documentElement.style.colorScheme = ciwiDarkThemeNames.has(normalized) ? 'dark' : 'light';
  if (persist !== false) {
    try { localStorage.setItem(ciwiThemeStorageKey, normalized); } catch (_) {}
  }
  ciwiApplyContractTheme(normalized);
  window.dispatchEvent(new CustomEvent('ciwi-theme-change', { detail: { theme: normalized } }));
  return normalized;
}

ciwiApplyTheme(ciwiStoredTheme(), false);
window.ciwiApplyContractThemeDocuments = ciwiApplyContractThemeDocuments;
window.addEventListener('storage', event => {
  if (event.key === ciwiThemeStorageKey) ciwiApplyTheme(event.newValue, false);
});
