package server

const uiSharedTextSearchJS = `
function ensureTextSearchStyles() {
  if (document.getElementById('__ciwiTextSearchStyles')) return;
  const style = document.createElement('style');
  style.id = '__ciwiTextSearchStyles';
  style.textContent = [
    '.ciwi-search-hit{background:#ffe79a;color:#2e2200;padding:0 1px;border-radius:2px;}',
    '.ciwi-search-hit.active{background:#ffc24d;outline:1px solid #946200;}',
  ].join('');
  document.head.appendChild(style);
}

function createTextSearchController(opts) {
  const options = opts || {};
  const scopeEl = options.scopeEl;
  const inputEl = options.inputEl;
  if (!scopeEl || !inputEl) return null;
  ensureTextSearchStyles();

  const prevBtn = options.prevBtn || null;
  const nextBtn = options.nextBtn || null;
  const countEl = options.countEl || null;
  const itemSelector = String(options.itemSelector || '').trim();
  const caseSensitive = !!options.caseSensitive;

  let hits = [];
  let activeIndex = -1;
  let query = '';
  let bound = false;

  function normalized(value) {
    return caseSensitive ? value : value.toLowerCase();
  }

  function updateCount() {
    if (!countEl) return;
    if (!query || hits.length === 0 || activeIndex < 0) {
      countEl.textContent = hits.length > 0 ? ('0/' + hits.length) : '0/0';
      return;
    }
    countEl.textContent = String(activeIndex + 1) + '/' + String(hits.length);
  }

  function updateNavState() {
    const disabled = hits.length === 0;
    if (prevBtn) prevBtn.disabled = disabled;
    if (nextBtn) nextBtn.disabled = disabled;
    updateCount();
  }

  function unwrapMark(mark) {
    if (!mark || !mark.parentNode) return;
    const parent = mark.parentNode;
    parent.replaceChild(document.createTextNode(mark.textContent || ''), mark);
    parent.normalize();
  }

  function clearMarks() {
    hits.forEach(unwrapMark);
    hits = [];
    activeIndex = -1;
    updateNavState();
  }

  function setActive(index, shouldScroll) {
    if (hits.length === 0) {
      activeIndex = -1;
      updateNavState();
      return;
    }
    const next = ((index % hits.length) + hits.length) % hits.length;
    if (activeIndex >= 0 && hits[activeIndex]) {
      hits[activeIndex].classList.remove('active');
    }
    activeIndex = next;
    const current = hits[activeIndex];
    if (current) {
      current.classList.add('active');
      if (shouldScroll) {
        current.scrollIntoView({ block: 'center', inline: 'nearest', behavior: 'smooth' });
      }
    }
    updateNavState();
  }

  function collectSearchRoots() {
    if (!itemSelector) return [scopeEl];
    const roots = Array.from(scopeEl.querySelectorAll(itemSelector));
    return roots.length ? roots : [scopeEl];
  }

  function markInTextNode(node, needle) {
    const text = String(node.nodeValue || '');
    if (!text) return [];
    const haystack = normalized(text);
    if (!haystack.includes(needle)) return [];

    const frag = document.createDocumentFragment();
    const localHits = [];
    let cursor = 0;
    while (cursor < text.length) {
      const pos = haystack.indexOf(needle, cursor);
      if (pos < 0) {
        frag.appendChild(document.createTextNode(text.slice(cursor)));
        break;
      }
      if (pos > cursor) {
        frag.appendChild(document.createTextNode(text.slice(cursor, pos)));
      }
      const mark = document.createElement('mark');
      mark.className = 'ciwi-search-hit';
      mark.textContent = text.slice(pos, pos + needle.length);
      frag.appendChild(mark);
      localHits.push(mark);
      cursor = pos + needle.length;
    }
    if (node.parentNode) {
      node.parentNode.replaceChild(frag, node);
    }
    return localHits;
  }

  function applySearch(rawQuery) {
    query = String(rawQuery || '').trim();
    clearMarks();
    if (!query) {
      updateNavState();
      return;
    }
    const needle = normalized(query);
    collectSearchRoots().forEach(root => {
      const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT, {
        acceptNode: function(node) {
          if (!node || !node.nodeValue || !node.nodeValue.trim()) return NodeFilter.FILTER_REJECT;
          const parent = node.parentElement;
          if (!parent) return NodeFilter.FILTER_REJECT;
          const tag = parent.tagName;
          if (tag === 'SCRIPT' || tag === 'STYLE' || tag === 'NOSCRIPT' || tag === 'MARK') {
            return NodeFilter.FILTER_REJECT;
          }
          return NodeFilter.FILTER_ACCEPT;
        },
      });
      const textNodes = [];
      while (walker.nextNode()) {
        textNodes.push(walker.currentNode);
      }
      textNodes.forEach(node => {
        const newHits = markInTextNode(node, needle);
        if (newHits.length) hits.push.apply(hits, newHits);
      });
    });
    if (hits.length > 0) {
      setActive(0, false);
    } else {
      updateNavState();
    }
  }

  function bind() {
    if (bound) return;
    bound = true;
    inputEl.addEventListener('input', () => applySearch(inputEl.value));
    inputEl.addEventListener('keydown', (ev) => {
      if (ev.key !== 'Enter') return;
      ev.preventDefault();
      if (ev.shiftKey) setActive(activeIndex - 1, true);
      else setActive(activeIndex + 1, true);
    });
    if (prevBtn) {
      prevBtn.addEventListener('click', () => setActive(activeIndex - 1, true));
    }
    if (nextBtn) {
      nextBtn.addEventListener('click', () => setActive(activeIndex + 1, true));
    }
    updateNavState();
  }

  bind();
  applySearch(inputEl.value);
  return {
    refresh: function() {
      applySearch(inputEl.value);
    },
    clear: function() {
      inputEl.value = '';
      applySearch('');
    },
    destroy: function() {
      clearMarks();
    },
  };
}
`
