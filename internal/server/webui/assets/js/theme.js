
const ciwiThemeStorageKey = 'ciwi.ui.theme.v1';
const ciwiThemeNames = new Set([
  'default',
  'jungle',
  'space',
  'pina-colada',
  'mango-kent',
  'mango-chaunsa',
  'mango-alphonso',
  'yellow-dragon-fruit',
  'dragon-fruit',
]);
const ciwiDarkThemeNames = new Set(['jungle', 'space']);
let ciwiThemeContractsPromise;

function ciwiApplyContractTheme(name) {
  if (!ciwiThemeContractsPromise) {
    ciwiThemeContractsPromise = fetch('/ui/contracts/themes.json', {cache: 'force-cache'})
      .then(response => response.ok ? response.json() : Promise.reject(new Error('theme contracts unavailable')))
      .catch(() => []);
  }
  ciwiThemeContractsPromise.then(documents => {
    if (document.documentElement.getAttribute('data-ciwi-theme') !== name) return;
    const documentTheme = documents.find(item => item && item.metadata && item.metadata.name === name)
      || documents.find(item => item && item.metadata && item.metadata.name === 'default');
    const colors = documentTheme && documentTheme.theme && documentTheme.theme.colors;
    if (!colors) return;
    const mapping = {
      background: '--bg', 'background-start': '--bg2', 'background-end': '--bg3',
      'background-glow-a': '--bg-glow-a', 'background-glow-b': '--bg-glow-b',
      surface: '--card', 'surface-raised': '--surface', 'surface-subtle': '--surface-subtle',
      'surface-glow': '--card-glow', text: '--ink', 'text-muted': '--muted',
      accent: '--accent', 'accent-strong': '--accent-strong', border: '--line',
      success: '--ok', warning: '--warn', danger: '--bad',
      'pill-background': '--pill-bg', 'pill-text': '--pill-ink',
      'console-background': '--console-bg', 'console-surface': '--console-surface',
      'console-border': '--console-line', 'console-text': '--console-ink',
      'console-muted': '--console-muted', 'console-accent': '--console-accent',
    };
    const style = document.documentElement.style;
    Object.entries(mapping).forEach(([token, variable]) => {
      if (colors[token]) style.setProperty(variable, colors[token]);
    });
  });
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
window.addEventListener('storage', event => {
  if (event.key === ciwiThemeStorageKey) ciwiApplyTheme(event.newValue, false);
});
