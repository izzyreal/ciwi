(() => {
  'use strict';

  const root = document.getElementById('declarativeRoot');

  function gradientCSS(gradient) {
    if (!gradient || !Array.isArray(gradient.stops)) return '';
    const stops = gradient.stops.map(stop => stop.color + ' ' + String(stop.position) + '%').join(', ');
    return gradient.kind === 'radial'
      ? 'radial-gradient(circle, ' + stops + ')'
      : 'linear-gradient(' + String(gradient.angle || 135) + 'deg, ' + stops + ')';
  }

  function applyContractTheme(documents) {
    const selected = document.documentElement.getAttribute('data-ciwi-theme') || 'default';
    const documentTheme = (documents || []).find(item => item && item.metadata && item.metadata.name === selected)
      || (documents || []).find(item => item && item.metadata && item.metadata.name === 'default');
    if (!documentTheme) return;
    const theme = documentTheme.theme || {};
    const colors = theme.colors || {};
    const style = document.documentElement.style;
    const mapping = {
      background: '--bg', surface: '--surface', 'surface-subtle': '--surface-subtle',
      text: '--ink', 'text-muted': '--muted', accent: '--accent', 'accent-strong': '--accent-strong',
      border: '--line', success: '--ok', warning: '--warn', danger: '--bad', focus: '--focus-ring',
    };
    Object.entries(mapping).forEach(([token, variable]) => { if (colors[token]) style.setProperty(variable, colors[token]); });
    const page = gradientCSS(theme.gradients && theme.gradients.page);
    const hero = gradientCSS(theme.gradients && theme.gradients.hero);
    if (page) style.setProperty('--page-background', page);
    if (hero) style.setProperty('--chrome-card-bg', hero);
  }

  function resolve(data, path) {
    return String(path || '').split('.').reduce((current, part) => {
      if (current === null || current === undefined || !(part in Object(current))) {
        throw new Error('Binding not found: ' + path);
      }
      return current[part];
    }, data);
  }

  function renderText(text, data) {
    if (!text) return '';
    if (text.literal !== undefined && text.literal !== '') return String(text.literal);
    if (text.binding) return String(resolve(data, text.binding) ?? '');
    return String(text.template || '').replace(/\{\{\s*([^}]+?)\s*\}\}/g, (_, binding) => String(resolve(data, binding) ?? ''));
  }

  function withWebOverride(node) {
    const override = node.overrides && node.overrides.web;
    if (!override) return node;
    return Object.assign({}, node, {
      hidden: !!override.hidden,
      layout: Object.assign({}, node.layout || {}, override.layout || {}),
      style: Object.assign({}, node.style || {}, override.style || {}),
    });
  }

  function applyLayout(element, layout) {
    if (!layout) return;
    const sizes = { small: '8px', medium: '16px', large: '28px' };
    if (layout.direction === 'horizontal') element.style.flexDirection = 'row';
    if (layout.direction === 'vertical') element.style.flexDirection = 'column';
    if (layout.gap) element.style.gap = sizes[layout.gap] || layout.gap;
    if (layout.padding) element.style.padding = sizes[layout.padding] || layout.padding;
    if (layout.align) element.style.alignItems = layout.align;
    if (layout.justify) element.style.justifyContent = layout.justify;
    if (layout.wrap) element.style.flexWrap = 'wrap';
    if (layout.grow) element.style.flexGrow = '1';
  }

  function elementFor(node) {
    switch (node.component) {
      case 'page': return document.createElement('main');
      case 'section': return document.createElement('section');
      case 'card': return document.createElement('article');
      case 'disclosure': return document.createElement('details');
      case 'button': return document.createElement('button');
      case 'image': return document.createElement('img');
      case 'divider': return document.createElement('hr');
      case 'spacer': return document.createElement('span');
      default: return document.createElement('div');
    }
  }

  function bindActions(element, actions, data) {
    (actions || []).forEach(action => {
      if (action.on !== 'activate') return;
      element.tabIndex = element.tabIndex >= 0 ? element.tabIndex : 0;
      element.setAttribute('role', element.tagName === 'BUTTON' ? 'button' : 'link');
      const invoke = async () => {
        const args = Object.fromEntries(Object.entries(action.arguments || {}).map(([key, value]) => [key, renderText({ template: value }, data)]));
        if (action.confirm && !window.confirm(action.confirm.message || action.confirm.title || 'Continue?')) return;
        if (action.command === 'navigate' && args.route) {
          const inPreview = window.location.pathname.startsWith('/declarative-preview');
          const destination = inPreview
            ? (args.route === '/' ? '/declarative-preview' : '/declarative-preview' + args.route)
            : args.route;
          window.location.assign(destination);
        }
        else if (action.command === 'refresh') refresh();
        else if (action.command === 'run-pipeline') {
          const response = await fetch('/api/v1/pipelines/' + encodeURIComponent(args.pipelineDbId) + '/run-selection', {
            method: 'POST',
            headers: {'Content-Type': 'application/json', 'Idempotency-Key': crypto.randomUUID()},
            body: '{}',
          });
          if (!response.ok) throw new Error(await response.text());
          element.textContent = 'Queued';
        }
        else throw new Error('Command is not implemented by the web proof renderer: ' + action.command);
      };
      element.addEventListener('click', event => {
        if (element.tagName === 'BUTTON') event.stopPropagation();
        invoke().catch(error => window.alert(error.message || String(error)));
      });
      element.addEventListener('keydown', event => {
        if (event.key === 'Enter' || event.key === ' ') { event.preventDefault(); invoke().catch(error => window.alert(error.message || String(error))); }
      });
    });
  }

  function renderNode(rawNode, data) {
    const node = withWebOverride(rawNode);
    if (node.hidden) return document.createDocumentFragment();
    if (node.visible) {
      const equal = String(resolve(data, node.visible.binding)) === String(node.visible.equals || 'true');
      if (node.visible.not ? equal : !equal) return document.createDocumentFragment();
    }
    if (node.repeat) {
      const list = resolve(data, node.repeat.source);
      const fragment = document.createDocumentFragment();
      (Array.isArray(list) ? list : []).forEach(item => {
        const itemData = Object.assign({}, data, { [node.repeat.as]: item });
        const clone = Object.assign({}, node, { repeat: null });
        fragment.appendChild(renderNode(clone, itemData));
      });
      return fragment;
    }
    const element = elementFor(node);
    element.classList.add('dsl-' + node.component);
    if (node.id) element.id = node.id;
    const style = node.style || {};
    if (style.role) element.classList.add('dsl-' + style.role);
    if (style.tone) element.classList.add('dsl-' + style.tone);
    if (style.emphasis) element.classList.add('dsl-' + style.emphasis);
    if (style.truncate) element.classList.add('dsl-truncate');
    applyLayout(element, node.layout);
    if (node.component === 'disclosure') {
      const summary = document.createElement('summary');
      summary.textContent = renderText(node.text, data) || 'Details';
      element.appendChild(summary);
    } else if (node.component === 'image' && node.image) {
      element.src = node.image.asset === 'ciwi-logo' ? '/ciwi-logo.png' : node.image.asset;
      element.alt = node.image.description || '';
    } else if (node.text) {
      element.textContent = renderText(node.text, data);
    }
    bindActions(element, node.actions, data);
    (node.children || []).forEach(child => element.appendChild(renderNode(child, data)));
    return element;
  }

  async function refresh() {
    try {
      const projectMatch = window.location.pathname.match(/^\/declarative-preview\/projects\/(\d+)\/?$/);
      const screenName = projectMatch ? 'project-details' : 'front-page';
      const viewURL = projectMatch ? '/api/v1/views/projects/' + encodeURIComponent(projectMatch[1]) : '/api/v1/views/front-page';
      const bindingRoot = projectMatch ? 'projectDetails' : 'frontPage';
      const [screenResponse, themeResponse, viewResponse] = await Promise.all([
        fetch('/ui/contracts/screens/' + screenName + '.json'),
        fetch('/ui/contracts/themes.json'),
        fetch(viewURL),
      ]);
      if (!screenResponse.ok || !themeResponse.ok || !viewResponse.ok) throw new Error('Could not load declarative view data');
      const [documentContract, themes, view] = await Promise.all([screenResponse.json(), themeResponse.json(), viewResponse.json()]);
      applyContractTheme(themes);
      const fragment = renderNode(documentContract.screen.root, { [bindingRoot]: view });
      root.replaceChildren(fragment);
    } catch (error) {
      const message = document.createElement('div');
      message.className = 'dsl-error';
      message.textContent = error.message || String(error);
      root.replaceChildren(message);
    }
  }

  refresh();
})();
