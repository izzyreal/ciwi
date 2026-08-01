package webui

const uiSharedTooltipJS = `
let ciwiActiveHoverTooltip = null;
let ciwiPendingHoverTooltip = null;

function ensureHoverTooltipStyles() {
  if (document.getElementById('__ciwiHoverTooltipStyles')) return;
  const style = document.createElement('style');
  style.id = '__ciwiHoverTooltipStyles';
  style.textContent = [
    '.ciwi-hover-tooltip{position:fixed;z-index:2600;display:none;max-width:min(560px,88vw);padding:8px 10px;border:1px solid var(--line);border-radius:8px;background:var(--surface);color:var(--ink);font-size:14px;font-weight:400;line-height:1.35;box-shadow:0 6px 18px var(--shadow);}',
    '.ciwi-hover-tooltip code{font-family:ui-monospace,SFMono-Regular,Menlo,Monaco,Consolas,\"Liberation Mono\",\"Courier New\",monospace;background:var(--code-bg);border:1px solid var(--code-line);border-radius:4px;padding:0 4px;font-size:.95em;}',
    '.ciwi-hover-tooltip a{color:var(--accent);text-decoration:underline;}',
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
  const showDelayMs = Math.max(0, Number(options.showDelayMs || 0));
  const hideOnAnchorLeave = !!options.hideOnAnchorLeave;
  const owner = String(options.owner || '').trim();
  const shouldShow = (typeof options.shouldShow === 'function') ? options.shouldShow : (() => true);
  const tip = document.createElement('div');
  tip.className = 'ciwi-hover-tooltip';
  if (owner) tip.setAttribute('data-ciwi-tooltip-owner', owner);
  tip.innerHTML = html;
  document.body.appendChild(tip);

  let hideTimer = null;
  let showTimer = null;
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

  function clearShowTimer() {
    if (showTimer != null) {
      clearTimeout(showTimer);
      showTimer = null;
    }
    if (ciwiPendingHoverTooltip === controller) {
      ciwiPendingHoverTooltip = null;
    }
  }

  function showTip() {
    clearShowTimer();
    clearHideTimer();
    if (!shouldShow()) {
      hideNow();
      return;
    }
    if (ciwiPendingHoverTooltip && ciwiPendingHoverTooltip !== controller) {
      ciwiPendingHoverTooltip.cancelPendingShow();
    }
    if (ciwiActiveHoverTooltip && ciwiActiveHoverTooltip !== controller) {
      ciwiActiveHoverTooltip.hide();
    }
    tip.style.display = 'block';
    tip.classList.add('is-visible');
    visible = true;
    ciwiActiveHoverTooltip = controller;
    positionTip();
  }

  function hideNow() {
    clearShowTimer();
    clearHideTimer();
    tip.style.display = 'none';
    tip.classList.remove('is-visible');
    visible = false;
    if (ciwiActiveHoverTooltip === controller) {
      ciwiActiveHoverTooltip = null;
    }
  }

  function scheduleShow() {
    clearHideTimer();
    if (visible) return;
    if (ciwiPendingHoverTooltip && ciwiPendingHoverTooltip !== controller) {
      ciwiPendingHoverTooltip.cancelPendingShow();
    }
    if (ciwiActiveHoverTooltip && ciwiActiveHoverTooltip !== controller) {
      ciwiActiveHoverTooltip.hide();
    }
    clearShowTimer();
    if (showDelayMs === 0) {
      showTip();
      return;
    }
    ciwiPendingHoverTooltip = controller;
    showTimer = setTimeout(() => {
      showTimer = null;
      if (ciwiPendingHoverTooltip === controller) {
        ciwiPendingHoverTooltip = null;
      }
      const anchorHover = !!(anchor.matches && anchor.matches(':hover'));
      const anchorFocus = document.activeElement === anchor;
      if (anchorHover || anchorFocus) showTip();
    }, showDelayMs);
  }

  function shouldKeepVisible() {
    const anchorHover = !!(anchor.matches && anchor.matches(':hover'));
    const tipHover = !!(tip.matches && tip.matches(':hover'));
    const anchorFocus = document.activeElement === anchor;
    return anchorHover || tipHover || anchorFocus || hasSelectionInsideTooltip();
  }

  function scheduleHide() {
    clearShowTimer();
    clearHideTimer();
    hideTimer = setTimeout(function retryHide() {
      if (shouldKeepVisible()) {
        hideTimer = setTimeout(retryHide, 150);
        return;
      }
      hideNow();
    }, lingerMs);
  }

  const onEnter = () => scheduleShow();
  const onAnchorLeave = () => {
    if (hideOnAnchorLeave) {
      hideNow();
      return;
    }
    scheduleHide();
  };
  const onTipEnter = () => {
    if (!hideOnAnchorLeave) showTip();
  };
  const onTipLeave = () => scheduleHide();
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
  anchor.addEventListener('focus', showTip);
  anchor.addEventListener('mouseleave', onAnchorLeave);
  anchor.addEventListener('blur', onAnchorLeave);
  tip.addEventListener('mouseenter', onTipEnter);
  tip.addEventListener('mouseleave', onTipLeave);
  tip.addEventListener('mousedown', startSelectionDrag);
  document.addEventListener('mousedown', onDocumentMouseDown);
  document.addEventListener('mouseup', stopSelectionDrag);
  document.addEventListener('selectionchange', onSelection);
  window.addEventListener('scroll', onReposition, true);
  window.addEventListener('resize', onReposition);

  const controller = {
    isVisible: () => visible,
    show: scheduleShow,
    hide: hideNow,
    cancelPendingShow: clearShowTimer,
    destroy: () => {
      hideNow();
      anchor.removeEventListener('mouseenter', onEnter);
      anchor.removeEventListener('focus', showTip);
      anchor.removeEventListener('mouseleave', onAnchorLeave);
      anchor.removeEventListener('blur', onAnchorLeave);
      tip.removeEventListener('mouseenter', onTipEnter);
      tip.removeEventListener('mouseleave', onTipLeave);
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

function elementHasOverflow(element) {
  if (!element) return false;
  return (element.scrollWidth > (element.clientWidth + 1)) ||
    (element.scrollHeight > (element.clientHeight + 1));
}

function createOverflowTooltip(anchor, opts) {
  if (!anchor) return null;
  if (anchor.__ciwiOverflowTooltip && typeof anchor.__ciwiOverflowTooltip.destroy === 'function') {
    anchor.__ciwiOverflowTooltip.destroy();
  }

  const options = opts || {};
  const textOption = options.text;
  const owner = String(options.owner || '').trim();
  let hoverController = null;
  let renderedText = '';
  let destroyed = false;

  function resolveText() {
    const value = (typeof textOption === 'function')
      ? textOption(anchor)
      : (textOption !== undefined ? textOption : anchor.textContent);
    return String(value || '').trim();
  }

  function ensureVisible() {
    if (destroyed || !elementHasOverflow(anchor)) return;
    const text = resolveText();
    if (!text) return;
    if (hoverController && renderedText !== text) {
      hoverController.destroy();
      hoverController = null;
    }
    if (!hoverController) {
      renderedText = text;
      hoverController = createHoverTooltip(anchor, {
        html: escapeHtml(text).replace(/\r?\n/g, '<br />'),
        lingerMs: options.lingerMs,
        showDelayMs: options.showDelayMs === undefined ? 1000 : options.showDelayMs,
        hideOnAnchorLeave: true,
        owner: owner,
        shouldShow: () => elementHasOverflow(anchor),
      });
    }
    if (hoverController && typeof hoverController.show === 'function') {
      hoverController.show();
    }
  }

  anchor.addEventListener('mouseenter', ensureVisible);
  anchor.addEventListener('focus', ensureVisible);

  const controller = {
    destroy: () => {
      destroyed = true;
      anchor.removeEventListener('mouseenter', ensureVisible);
      anchor.removeEventListener('focus', ensureVisible);
      if (hoverController && typeof hoverController.destroy === 'function') {
        hoverController.destroy();
      }
      hoverController = null;
      if (anchor.__ciwiOverflowTooltip === controller) {
        delete anchor.__ciwiOverflowTooltip;
      }
    },
  };
  anchor.__ciwiOverflowTooltip = controller;
  return controller;
}

function bindOverflowTooltips(root, opts) {
  if (!root || !root.querySelectorAll) return;
  const options = opts || {};
  const ownerPrefix = String(options.ownerPrefix || 'overflow').trim() || 'overflow';
  root.querySelectorAll('[data-ciwi-overflow-text]').forEach((element, index) => {
    const text = String(element.getAttribute('data-ciwi-overflow-text') || element.textContent || '').trim();
    if (!text) return;
    createOverflowTooltip(element, {
      text: () => element.getAttribute('data-ciwi-overflow-text') || element.textContent || '',
      lingerMs: options.lingerMs,
      owner: ownerPrefix + '-' + String(index),
    });
  });
}

function destroyOverflowTooltips(root) {
  if (!root || !root.querySelectorAll) return;
  root.querySelectorAll('[data-ciwi-overflow-text]').forEach(element => {
    if (element.__ciwiOverflowTooltip && typeof element.__ciwiOverflowTooltip.destroy === 'function') {
      element.__ciwiOverflowTooltip.destroy();
    }
  });
}

`
