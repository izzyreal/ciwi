package server

const uiSharedModalJS = `
function ensureModalBaseStyles() {
  if (document.getElementById('__ciwiModalBaseStyles')) return;
  const style = document.createElement('style');
  style.id = '__ciwiModalBaseStyles';
  style.textContent = [
    '.ciwi-modal-overlay{position:fixed;inset:0;background:rgba(10,27,20,.45);display:none;align-items:center;justify-content:center;z-index:2000;padding:12px;}',
    '.ciwi-modal{--ciwi-modal-width:70vw;--ciwi-modal-height:70vh;width:var(--ciwi-modal-width);height:var(--ciwi-modal-height);background:#fff;border:1px solid #c4ddd0;border-radius:12px;box-shadow:0 24px 56px rgba(15,31,24,.24);display:grid;grid-template-rows:auto 1fr;overflow:hidden;max-width:96vw;max-height:96vh;}',
    '.ciwi-modal-head{display:flex;align-items:center;justify-content:space-between;gap:8px;border-bottom:1px solid #c4ddd0;padding:12px;background:#f7fcf9;}',
    '.ciwi-modal-title{font-size:18px;font-weight:700;}',
    '.ciwi-modal-subtitle{font-size:12px;color:#5f6f67;}',
    '.ciwi-modal-body{padding:12px;overflow:hidden;min-height:0;}',
  ].join('');
  document.head.appendChild(style);
}

function openModalOverlay(overlay, width, height) {
  if (!overlay) return;
  ensureModalBaseStyles();
  const panel = overlay.querySelector('.ciwi-modal');
  if (panel) {
    if (width) panel.style.setProperty('--ciwi-modal-width', width);
    if (height) panel.style.setProperty('--ciwi-modal-height', height);
  }
  overlay.style.display = 'flex';
  overlay.setAttribute('aria-hidden', 'false');
}

function closeModalOverlay(overlay) {
  if (!overlay) return;
  overlay.style.display = 'none';
  overlay.setAttribute('aria-hidden', 'true');
}

function wireModalCloseBehavior(overlay, onClose) {
  if (!overlay) return;
  if (typeof onClose === 'function') {
    overlay.__ciwiModalOnClose = onClose;
  } else {
    overlay.__ciwiModalOnClose = null;
  }
  if (overlay.__ciwiModalCloseBound) return;
  ensureModalBaseStyles();
  if (overlay.getAttribute('aria-hidden') !== 'false') {
    overlay.style.display = 'none';
    overlay.setAttribute('aria-hidden', 'true');
  }
  overlay.__ciwiModalCloseBound = true;
  let pointerDownOnOverlay = false;
  function hasActiveTextSelection() {
    const sel = window.getSelection && window.getSelection();
    if (!sel) return false;
    const text = String(sel.toString() || '').trim();
    return text.length > 0;
  }
  overlay.addEventListener('mousedown', (ev) => {
    pointerDownOnOverlay = (ev.target === overlay);
  });
  overlay.addEventListener('click', (ev) => {
    if (ev.target !== overlay) return;
    if (!pointerDownOnOverlay) return;
    if (hasActiveTextSelection()) return;
    const closeFn = overlay.__ciwiModalOnClose;
    if (typeof closeFn === 'function') closeFn(); else closeModalOverlay(overlay);
  });
  document.addEventListener('mouseup', () => {
    pointerDownOnOverlay = false;
  });
  document.addEventListener('keydown', (ev) => {
    if (ev.key !== 'Escape') return;
    if (overlay.style.display !== 'flex') return;
    const closeFn = overlay.__ciwiModalOnClose;
    if (typeof closeFn === 'function') closeFn(); else closeModalOverlay(overlay);
  });
}

function ensureConfirmDialogStyles() {
  if (document.getElementById('__ciwiConfirmDialogStyles')) return;
  const style = document.createElement('style');
  style.id = '__ciwiConfirmDialogStyles';
  style.textContent = [
    '.ciwi-confirm-modal{height:auto;grid-template-rows:auto auto auto;max-width:min(520px,92vw);}',
    '.ciwi-confirm-body{padding:14px 16px 6px;color:#1f2a24;font-size:14px;line-height:1.4;overflow-wrap:anywhere;word-break:break-word;}',
    '.ciwi-confirm-actions{padding:8px 16px 14px;display:flex;gap:8px;justify-content:flex-end;flex-wrap:wrap;}',
    '.ciwi-confirm-actions .secondary{background:#fff;color:#27473b;border:1px solid #c4ddd0;}',
  ].join('');
  document.head.appendChild(style);
}

function ensureConfirmDialog() {
  ensureModalBaseStyles();
  ensureConfirmDialogStyles();
  let overlay = document.getElementById('__ciwiConfirmOverlay');
  if (overlay) return overlay;
  overlay = document.createElement('div');
  overlay.id = '__ciwiConfirmOverlay';
  overlay.className = 'ciwi-modal-overlay';
  overlay.setAttribute('aria-hidden', 'true');
  overlay.innerHTML = [
    '<div class="ciwi-modal ciwi-confirm-modal" role="dialog" aria-modal="true" aria-label="Confirm action">',
    '  <div class="ciwi-modal-head">',
    '    <div style="font-weight:700;" id="__ciwiConfirmTitle">Confirm</div>',
    '  </div>',
    '  <div class="ciwi-confirm-body" id="__ciwiConfirmMessage"></div>',
    '  <div class="ciwi-confirm-actions">',
    '    <button type="button" id="__ciwiConfirmCancel" class="secondary">Cancel</button>',
    '    <button type="button" id="__ciwiConfirmOk">OK</button>',
    '  </div>',
    '</div>',
  ].join('');
  document.body.appendChild(overlay);
  return overlay;
}

function showConfirmDialog(opts) {
  const options = opts || {};
  const message = String(options.message || '').trim();
  if (!message) return Promise.resolve(false);
  const title = String(options.title || 'Confirm').trim() || 'Confirm';
  const okLabel = String(options.okLabel || 'OK').trim() || 'OK';
  const cancelLabel = String(options.cancelLabel || 'Cancel').trim() || 'Cancel';
  const overlay = ensureConfirmDialog();
  const titleEl = document.getElementById('__ciwiConfirmTitle');
  const msgEl = document.getElementById('__ciwiConfirmMessage');
  const okBtn = document.getElementById('__ciwiConfirmOk');
  const cancelBtn = document.getElementById('__ciwiConfirmCancel');
  if (!titleEl || !msgEl || !okBtn || !cancelBtn) return Promise.resolve(false);

  titleEl.textContent = title;
  msgEl.textContent = message;
  okBtn.textContent = okLabel;
  cancelBtn.textContent = cancelLabel;
  okBtn.disabled = false;
  cancelBtn.disabled = false;

  return new Promise((resolve) => {
    let settled = false;
    const settle = (value) => {
      if (settled) return;
      settled = true;
      okBtn.onclick = null;
      cancelBtn.onclick = null;
      closeModalOverlay(overlay);
      resolve(!!value);
    };
    wireModalCloseBehavior(overlay, () => settle(false));
    okBtn.onclick = () => settle(true);
    cancelBtn.onclick = () => settle(false);
    openModalOverlay(overlay, '460px', 'auto');
    setTimeout(() => okBtn.focus(), 0);
  });
}

function ensureAlertDialog() {
  ensureModalBaseStyles();
  ensureConfirmDialogStyles();
  let overlay = document.getElementById('__ciwiAlertOverlay');
  if (overlay) return overlay;
  overlay = document.createElement('div');
  overlay.id = '__ciwiAlertOverlay';
  overlay.className = 'ciwi-modal-overlay';
  overlay.setAttribute('aria-hidden', 'true');
  overlay.innerHTML = [
    '<div class="ciwi-modal ciwi-confirm-modal" role="dialog" aria-modal="true" aria-label="Message">',
    '  <div class="ciwi-modal-head">',
    '    <div style="font-weight:700;" id="__ciwiAlertTitle">Message</div>',
    '  </div>',
    '  <div class="ciwi-confirm-body" id="__ciwiAlertMessage"></div>',
    '  <div class="ciwi-confirm-actions">',
    '    <button type="button" id="__ciwiAlertOk">OK</button>',
    '  </div>',
    '</div>',
  ].join('');
  document.body.appendChild(overlay);
  return overlay;
}

function showAlertDialog(opts) {
  const options = opts || {};
  const message = String(options.message || '').trim();
  if (!message) return Promise.resolve();
  const title = String(options.title || 'Message').trim() || 'Message';
  const okLabel = String(options.okLabel || 'OK').trim() || 'OK';
  const overlay = ensureAlertDialog();
  const titleEl = document.getElementById('__ciwiAlertTitle');
  const msgEl = document.getElementById('__ciwiAlertMessage');
  const okBtn = document.getElementById('__ciwiAlertOk');
  if (!titleEl || !msgEl || !okBtn) return Promise.resolve();

  titleEl.textContent = title;
  msgEl.textContent = message;
  okBtn.textContent = okLabel;
  okBtn.disabled = false;

  return new Promise((resolve) => {
    let settled = false;
    const settle = () => {
      if (settled) return;
      settled = true;
      okBtn.onclick = null;
      closeModalOverlay(overlay);
      resolve();
    };
    wireModalCloseBehavior(overlay, settle);
    okBtn.onclick = settle;
    openModalOverlay(overlay, '460px', 'auto');
    setTimeout(() => okBtn.focus(), 0);
  });
}

`
