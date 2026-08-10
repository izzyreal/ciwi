const ciwiThemeStorageKey = 'ciwi.ui.theme.v1';
const ciwiThemeNamePattern = /^[a-z][a-z0-9-]*$/;
let ciwiThemeContractDocuments = null;

function ciwiVersionedUIResource(path) {
  return typeof window.ciwiUIResourceURL === 'function' ? window.ciwiUIResourceURL(path) : path;
}

function ciwiKnownTheme(name, documents) {
  const themes = Array.isArray(documents) ? documents : ciwiThemeContractDocuments;
  return Array.isArray(themes) && themes.some(item => item && item.metadata && item.metadata.name === name);
}

function ciwiNormalizeTheme(theme, documents) {
  const normalized = String(theme || '').trim().toLowerCase();
  if (!ciwiThemeNamePattern.test(normalized)) return 'default';
  if (Array.isArray(documents) && !ciwiKnownTheme(normalized, documents)) return 'default';
  return normalized;
}

function ciwiStoredTheme() {
  try { return ciwiNormalizeTheme(localStorage.getItem(ciwiThemeStorageKey)); } catch (_) { return 'default'; }
}

function ciwiApplyTheme(theme, persist) {
  const normalized = ciwiNormalizeTheme(theme, ciwiThemeContractDocuments);
  document.documentElement.setAttribute('data-ciwi-theme', normalized);
  if (persist !== false) {
    try { localStorage.setItem(ciwiThemeStorageKey, normalized); } catch (_) {}
  }
  window.dispatchEvent(new CustomEvent('ciwi-theme-change', {detail: {theme: normalized}}));
  return normalized;
}

function ciwiApplyContractThemeDocuments(documents, requestedName) {
  ciwiThemeContractDocuments = Array.isArray(documents) ? documents : [];
  const normalized = ciwiNormalizeTheme(requestedName || ciwiStoredTheme(), ciwiThemeContractDocuments);
  if (document.documentElement.getAttribute('data-ciwi-theme') !== normalized) {
    document.documentElement.setAttribute('data-ciwi-theme', normalized);
  }
  return normalized;
}

ciwiApplyTheme(ciwiStoredTheme(), false);
window.ciwiApplyContractThemeDocuments = ciwiApplyContractThemeDocuments;
window.addEventListener('storage', event => {
  if (event.key === ciwiThemeStorageKey) ciwiApplyTheme(event.newValue, false);
});
