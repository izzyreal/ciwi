package server

const uiThemeJS = `
const ciwiThemeStorageKey = 'ciwi.ui.theme.v1';
const ciwiThemeNames = new Set(['default', 'jungle', 'space']);

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
  document.documentElement.style.colorScheme = normalized === 'default' ? 'light' : 'dark';
  if (persist !== false) {
    try { localStorage.setItem(ciwiThemeStorageKey, normalized); } catch (_) {}
  }
  window.dispatchEvent(new CustomEvent('ciwi-theme-change', { detail: { theme: normalized } }));
  return normalized;
}

ciwiApplyTheme(ciwiStoredTheme(), false);
window.addEventListener('storage', event => {
  if (event.key === ciwiThemeStorageKey) ciwiApplyTheme(event.newValue, false);
});
`
