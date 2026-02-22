package server

const uiSharedTooltipJS = `
function ensureHoverTooltipStyles() {
  if (document.getElementById('__ciwiHoverTooltipStyles')) return;
  const style = document.createElement('style');
  style.id = '__ciwiHoverTooltipStyles';
  style.textContent = [
    '.ciwi-hover-tooltip{position:fixed;z-index:2600;display:none;max-width:min(560px,88vw);padding:8px 10px;border:1px solid #c4ddd0;border-radius:8px;background:#f8fcfa;color:#1f2a24;font-size:14px;font-weight:400;line-height:1.35;box-shadow:0 6px 18px rgba(26,40,34,.15);}',
    '.ciwi-hover-tooltip code{font-family:ui-monospace,SFMono-Regular,Menlo,Monaco,Consolas,\"Liberation Mono\",\"Courier New\",monospace;background:#eef6f1;border:1px solid #d7e6dd;border-radius:4px;padding:0 4px;font-size:.95em;}',
    '.ciwi-hover-tooltip a{color:#1f5f44;text-decoration:underline;}',
    'body.ciwi-tooltip-selecting *{user-select:none !important;}',
    'body.ciwi-tooltip-selecting .ciwi-hover-tooltip,body.ciwi-tooltip-selecting .ciwi-hover-tooltip *{user-select:text !important;}',
  ].join('');
  document.head.appendChild(style);
}

function createHoverTooltip(anchor, opts) {
  if (!anchor) return null;
  ensureHoverTooltipStyles();
  if (anchor.__ciwiHoverTooltip && typeof anchor.__ciwiHoverTooltip.destroy === 'function') {
    anchor.__ciwiHoverTooltip.destroy();
  }

  const options = opts || {};
  const html = String(options.html || '').trim();
  if (!html) return null;
  const lingerMs = Math.max(0, Number(options.lingerMs || 2000));
  const owner = String(options.owner || '').trim();
  const tip = document.createElement('div');
  tip.className = 'ciwi-hover-tooltip';
  if (owner) tip.setAttribute('data-ciwi-tooltip-owner', owner);
  tip.innerHTML = html;
  document.body.appendChild(tip);

  let hideTimer = null;
  let visible = false;
  let draggingSelection = false;

  function hasSelectionInsideTooltip() {
    const sel = window.getSelection && window.getSelection();
    if (!sel || sel.rangeCount === 0) return false;
    const text = String(sel.toString() || '').trim();
    if (!text) return false;
    const range = sel.getRangeAt(0);
    const node = range.commonAncestorContainer;
    return tip.contains(node.nodeType === 1 ? node : node.parentNode);
  }

  function positionTip() {
    const ar = anchor.getBoundingClientRect();
    const tr = tip.getBoundingClientRect();
    const margin = 8;
    let left = ar.left;
    if ((left + tr.width + margin) > window.innerWidth) {
      left = Math.max(margin, window.innerWidth - tr.width - margin);
    }
    let top = ar.bottom + 8;
    if ((top + tr.height + margin) > window.innerHeight) {
      top = Math.max(margin, ar.top - tr.height - 8);
    }
    tip.style.left = left + 'px';
    tip.style.top = top + 'px';
  }

  function clearHideTimer() {
    if (hideTimer != null) {
      clearTimeout(hideTimer);
      hideTimer = null;
    }
  }

  function showTip() {
    clearHideTimer();
    tip.style.display = 'block';
    tip.classList.add('is-visible');
    visible = true;
    positionTip();
  }

  function hideNow() {
    clearHideTimer();
    tip.style.display = 'none';
    tip.classList.remove('is-visible');
    visible = false;
  }

  function shouldKeepVisible() {
    const anchorHover = !!(anchor.matches && anchor.matches(':hover'));
    const tipHover = !!(tip.matches && tip.matches(':hover'));
    const anchorFocus = document.activeElement === anchor;
    return anchorHover || tipHover || anchorFocus || hasSelectionInsideTooltip();
  }

  function scheduleHide() {
    clearHideTimer();
    hideTimer = setTimeout(function retryHide() {
      if (shouldKeepVisible()) {
        hideTimer = setTimeout(retryHide, 150);
        return;
      }
      hideNow();
    }, lingerMs);
  }

  const onEnter = () => showTip();
  const onLeave = () => scheduleHide();
  const onSelection = () => {
    if (!visible) return;
    if (hasSelectionInsideTooltip()) clearHideTimer();
  };
  const startSelectionDrag = () => {
    draggingSelection = true;
    document.body.classList.add('ciwi-tooltip-selecting');
  };
  const stopSelectionDrag = () => {
    if (!draggingSelection) return;
    draggingSelection = false;
    document.body.classList.remove('ciwi-tooltip-selecting');
  };
  const onReposition = () => {
    if (!visible) return;
    positionTip();
  };
  const onDocumentMouseDown = (event) => {
    if (!visible) return;
    const target = event && event.target;
    if (!target) return;
    if (target === anchor || (anchor.contains && anchor.contains(target))) return;
    if (target === tip || (tip.contains && tip.contains(target))) return;
    hideNow();
  };

  anchor.addEventListener('mouseenter', onEnter);
  anchor.addEventListener('focus', onEnter);
  anchor.addEventListener('mouseleave', onLeave);
  anchor.addEventListener('blur', onLeave);
  tip.addEventListener('mouseenter', onEnter);
  tip.addEventListener('mouseleave', onLeave);
  tip.addEventListener('mousedown', startSelectionDrag);
  document.addEventListener('mousedown', onDocumentMouseDown);
  document.addEventListener('mouseup', stopSelectionDrag);
  document.addEventListener('selectionchange', onSelection);
  window.addEventListener('scroll', onReposition, true);
  window.addEventListener('resize', onReposition);

  const controller = {
    isVisible: () => visible,
    destroy: () => {
      hideNow();
      anchor.removeEventListener('mouseenter', onEnter);
      anchor.removeEventListener('focus', onEnter);
      anchor.removeEventListener('mouseleave', onLeave);
      anchor.removeEventListener('blur', onLeave);
      tip.removeEventListener('mouseenter', onEnter);
      tip.removeEventListener('mouseleave', onLeave);
      tip.removeEventListener('mousedown', startSelectionDrag);
      document.removeEventListener('mousedown', onDocumentMouseDown);
      document.removeEventListener('mouseup', stopSelectionDrag);
      document.removeEventListener('selectionchange', onSelection);
      window.removeEventListener('scroll', onReposition, true);
      window.removeEventListener('resize', onReposition);
      stopSelectionDrag();
      if (tip.parentNode) tip.parentNode.removeChild(tip);
      if (anchor.__ciwiHoverTooltip === controller) {
        delete anchor.__ciwiHoverTooltip;
      }
    },
  };
  anchor.__ciwiHoverTooltip = controller;
  return controller;
}

`
