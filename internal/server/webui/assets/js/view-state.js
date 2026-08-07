(() => {
  'use strict';

  function captureViewState(root, documentRef, windowRef) {
    const doc = documentRef || document;
    const win = windowRef || window;
    const scrollPositions = [];
    root.querySelectorAll('[id]').forEach(element => {
      if (element.scrollTop || element.scrollLeft) {
        scrollPositions.push({id: element.id, top: element.scrollTop, left: element.scrollLeft});
      }
    });
    let focus = null;
    const active = doc.activeElement;
    if (active && active.id && root.contains(active)) {
      focus = {id: active.id};
      if (typeof active.selectionStart === 'number' && typeof active.selectionEnd === 'number') {
        focus.selectionStart = active.selectionStart;
        focus.selectionEnd = active.selectionEnd;
        focus.selectionDirection = active.selectionDirection || 'none';
      }
    }
    return {
      page: {top: Number(win.scrollY || 0), left: Number(win.scrollX || 0)},
      scrollPositions,
      focus,
    };
  }

  function restoreViewState(root, state, documentRef, windowRef) {
    const doc = documentRef || document;
    const win = windowRef || window;
    const snapshot = state || {};
    (Array.isArray(snapshot.scrollPositions) ? snapshot.scrollPositions : []).forEach(position => {
      const element = doc.getElementById(position.id);
      if (!element || !root.contains(element)) return;
      element.scrollTop = Number(position.top || 0);
      element.scrollLeft = Number(position.left || 0);
    });
    if (snapshot.focus && snapshot.focus.id) {
      const element = doc.getElementById(snapshot.focus.id);
      if (element && root.contains(element) && typeof element.focus === 'function') {
        element.focus({preventScroll: true});
        if (typeof element.setSelectionRange === 'function' && typeof snapshot.focus.selectionStart === 'number') {
          element.setSelectionRange(snapshot.focus.selectionStart, snapshot.focus.selectionEnd, snapshot.focus.selectionDirection);
        }
      }
    }
    const page = snapshot.page || {};
    win.scrollTo(Number(page.left || 0), Number(page.top || 0));
  }

  function persistentBooleanState(storage, key) {
    let values = {};
    try {
      const parsed = JSON.parse(storage && storage.getItem(key) || '{}');
      if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) values = parsed;
    } catch (_) {}
    return {
      has: stateKey => Object.prototype.hasOwnProperty.call(values, stateKey),
      get: (stateKey, fallback) => Object.prototype.hasOwnProperty.call(values, stateKey) ? !!values[stateKey] : !!fallback,
      set: (stateKey, expanded) => {
        if (!stateKey) return;
        values[stateKey] = !!expanded;
        try { if (storage) storage.setItem(key, JSON.stringify(values)); } catch (_) {}
      },
      snapshot: () => ({...values}),
    };
  }

  window.ciwiCaptureViewState = captureViewState;
  window.ciwiRestoreViewState = restoreViewState;
  window.ciwiDisclosureState = persistentBooleanState(
    typeof localStorage === 'object' ? localStorage : null,
    'ciwi.declarative.disclosures.v1',
  );
})();
