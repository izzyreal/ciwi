(() => {
  'use strict';

  const root = document.getElementById('declarativeRoot');
  let outputWatchGeneration = 0;
  let outputEventSource = null;
  let completedOutputJobID = '';
  let programmaticOutputScroll = false;
  let routeLoadGeneration = 0;
  const maxOutputCharacters = 1024 * 1024;
  const maxLogViewCacheBytes = 4 * 1024 * 1024;
  const logViewStates = new Map();
  let fullLogSearchGeneration = 0;
  let fullLogSearchTimer = 0;
  let currentDocument = null;
  let currentData = null;
  let currentRouteMatch = null;
  let currentPath = '';
  let routeContractPromise = null;
  let themeContractPromise = null;
  let controlsContractPromise = null;
  let activeControls = null;
  let compactViewport = false;
  let condensedDisclosureViewport = false;
  let responsiveViewportBound = false;
  const screenContractPromises = new Map();
  const browserViewCache = new Map();
  let screenContractsPreloaded = false;
  let changeRefreshScheduler = null;
  let browserSelectControl = null;
  let graphViewRenderer = null;
  let treeViewRenderer = null;
  let domReconciler = null;
  let viewBindings = null;
  const determinateProgressLimit = .999;
  const indeterminateProgressCycleMs = 4000;
  const overrunProgressCycleMs = 2000;
  const disclosureStates = window.ciwiDisclosureState;
  const viewStorageKey = 'ciwi.declarative.views.v1';
  const viewStates = loadViewStates();
  const graphRuntimeStates = new Map();
  let committedActionBindings = new Map();
  let committedRenderSignature = '';
  let pendingHistoryNavigation = null;
  let browserNavigationSequence = 0;

  function rendererKeyPart(value) {
	return encodeURIComponent(String(value ?? '').trim());
  }

  function createRenderSession(screenName, actionBindings) {
	return {
	  screenName: rendererKeyPart(screenName || 'unknown'),
	  actionBindings: actionBindings || new Map(),
	};
  }

  function rootRenderContext(session) {
	const screenScope = 'screen:' + session.screenName;
	return {session, path: screenScope + '/root', parentScope: screenScope, repeatIdentity: '', inRepeat: false};
  }

  function childRenderContext(context, segment, repeatIdentity = '') {
	return {
	  session: context.session,
	  path: context.identity + '/' + segment,
	  parentScope: context.identity,
	  repeatIdentity,
	  inRepeat: context.inRepeat || !!repeatIdentity,
	};
  }

  function nodeRenderIdentity(node, data, context) {
	if (context.repeatIdentity) return context.repeatIdentity;
	if (node.disclosure && node.disclosure.stateKey) {
	  const stateKey = renderText({template: node.disclosure.stateKey}, data).trim();
	  if (stateKey) return context.parentScope + '/disclosure:' + rendererKeyPart(stateKey);
	}
	if (node.id) {
	  const idScope = context.inRepeat ? context.parentScope : 'screen:' + context.session.screenName;
	  return idScope + '/id:' + rendererKeyPart(node.id);
	}
	return context.path;
  }

  function annotateRendererElement(element, component, identity) {
	if (!element || element.nodeType !== Node.ELEMENT_NODE) return element;
	element.dataset.ciwiNodeKey = identity;
	element.dataset.ciwiComponent = component;
	element.dataset.ciwiTag = String(element.localName || element.tagName || '').toLowerCase();
	return element;
  }

  function repeatedItems(node, data, scopePath) {
	const list = resolve(data, node.repeat.source);
	const seen = new Set();
	return (Array.isArray(list) ? list : []).map((item, index) => {
	  const itemData = Object.assign({}, data, {[node.repeat.as]: item});
	  const key = String(resolve(itemData, node.repeat.key) ?? '').trim();
	  if (!key) throw new Error('Empty repeat key at ' + scopePath + ' for item ' + String(index));
	  if (seen.has(key)) throw new Error('Duplicate repeat key "' + key + '" at ' + scopePath);
	  seen.add(key);
	  return {itemData, key};
	});
  }

  function uiResourceURL(path) {
	return typeof window.ciwiUIResourceURL === 'function' ? window.ciwiUIResourceURL(path) : path;
  }

  function semanticProgressAt(progress, nowMs) {
    const model = progress && typeof progress === 'object' ? progress : {};
    let state = String(model.state || 'none');
    let fraction = Math.max(0, Math.min(1, Number(model.fraction || 0)));
    if (state === 'determinate') {
      const elapsed = Math.max(0, Number(nowMs || Date.now()) - Number(model.snapshot_unix_ms || 0));
      fraction = Math.max(0, Math.min(determinateProgressLimit, fraction + elapsed * Math.max(0, Number(model.rate_per_ms || 0))));
    }
    return {state, fraction};
  }

  function semanticProgressAnimationDelay(state, nowMs) {
    const cycle = state === 'indeterminate'
      ? indeterminateProgressCycleMs
      : (state === 'overrun' ? overrunProgressCycleMs : 0);
    if (!cycle) return '';
    const now = Math.max(0, Number(nowMs || Date.now()));
    return String(-(now % cycle)) + 'ms';
  }

  function updateSemanticProgress(element, nowMs) {
    const model = semanticProgressAt(element.__ciwiSemanticProgress, nowMs);
	const previousState = element.__ciwiSemanticProgressState || '';
	element.classList.toggle('ciwi-progress-indeterminate', model.state === 'indeterminate');
	element.classList.toggle('ciwi-progress-overrun', model.state === 'overrun');
	element.classList.toggle('ciwi-progress-complete', model.state === 'complete');
	if (model.state !== previousState) {
	  const delay = semanticProgressAnimationDelay(model.state, nowMs);
	  if (delay) element.style.setProperty('--ciwi-progress-animation-delay', delay);
	  else element.style.removeProperty('--ciwi-progress-animation-delay');
	  element.__ciwiSemanticProgressState = model.state;
	}
    if (model.state !== 'indeterminate') {
      const visible = model.state !== 'none' && model.state !== 'waiting';
      element.style.setProperty('--ciwi-progress-width', visible ? String(model.fraction * 100) + '%' : '0%');
    }
  }

  function bindSemanticProgress(element, progress) {
    if (!element || !progress || typeof progress !== 'object') return;
    element.classList.add('ciwi-progress-surface');
    element.__ciwiSemanticProgress = progress;
    updateSemanticProgress(element, Date.now());
  }

  function updateTimestampPulse(element, nowMs) {
    const timestamp = Number(element && element.__ciwiPulseTimestamp || 0);
    element.style.opacity = String(window.ciwiHeartbeat.opacity(timestamp, nowMs));
  }

  function bindTimestampPulse(element, timestamp) {
    if (!element) return;
    element.classList.add('dsl-pulse');
    element.__ciwiPulseTimestamp = Number(timestamp || 0);
    updateTimestampPulse(element, Date.now());
  }

  window.setInterval(() => {
    document.querySelectorAll('.ciwi-progress-surface').forEach(element => {
      if (element.__ciwiSemanticProgress) updateSemanticProgress(element, Date.now());
    });
    document.querySelectorAll('.dsl-pulse').forEach(element => updateTimestampPulse(element, Date.now()));
  }, 250);

  function loadViewStates() {
    try {
      const parsed = JSON.parse(localStorage.getItem(viewStorageKey) || '{}');
      return parsed && typeof parsed === 'object' && !Array.isArray(parsed) ? parsed : {};
    } catch (_) { return {}; }
  }

  function saveViewStates() {
    try { localStorage.setItem(viewStorageKey, JSON.stringify(viewStates)); } catch (_) {}
  }

  function gradientCSS(gradient) {
    if (!gradient || !Array.isArray(gradient.stops)) return '';
    const stops = gradient.stops.map(stop => stop.color + ' ' + String(stop.position) + '%').join(', ');
    return gradient.kind === 'radial'
      ? 'radial-gradient(circle, ' + stops + ')'
      : 'linear-gradient(' + String(gradient.angle || 135) + 'deg, ' + stops + ')';
  }

  function applyContractTheme(documents) {
    if (typeof window.ciwiApplyContractThemeDocuments === 'function') {
      window.ciwiApplyContractThemeDocuments(documents || []);
    }
  }

  function controlsContract() {
	if (!controlsContractPromise) {
	  controlsContractPromise = fetch(uiResourceURL('/ui/contracts/controls.json')).then(async response => {
		if (!response.ok) throw new Error(await response.text());
		return response.json();
	  });
	}
	return controlsContractPromise;
  }

  function applyControlsContract(documentContract) {
	activeControls = documentContract && documentContract.controls;
	if (!activeControls) return;
	const style = document.documentElement.style;
	const metric = (variable, value) => style.setProperty(variable, String(value) + 'px');
	metric('--ciwi-button-min-height', activeControls.button.minimumHeight.web);
	metric('--ciwi-button-padding-x', activeControls.button.paddingX.web);
	metric('--ciwi-button-padding-y', activeControls.button.paddingY.web);
	metric('--ciwi-button-icon-size', activeControls.button.iconSize.web);
	metric('--ciwi-button-icon-gap', activeControls.button.iconGap.web);
	metric('--ciwi-button-icon-only-size', activeControls.button.iconOnlySize.web);
	metric('--ciwi-badge-padding-x', activeControls.badge.paddingX);
	metric('--ciwi-badge-padding-y', activeControls.badge.paddingY);
	style.setProperty('--ciwi-badge-tint', String(activeControls.badge.tintOpacity * 100) + '%');
	style.setProperty('--ciwi-badge-border-opacity', String(activeControls.badge.borderOpacity * 100) + '%');
	metric('--ciwi-input-min-height', activeControls.input.minimumHeight.web);
	metric('--ciwi-input-padding-x', activeControls.input.paddingX.web);
	metric('--ciwi-input-padding-y', activeControls.input.paddingY.web);
	style.setProperty('--ciwi-input-placeholder', activeControls.input.placeholderColor);
	metric('--ciwi-select-chevron-size', activeControls.select.chevronSize);
	metric('--ciwi-select-chevron-gap', activeControls.select.chevronGap);
	metric('--ciwi-select-min-height', activeControls.select.minimumHeight);
	metric('--ciwi-select-menu-padding', activeControls.select.menuPadding);
	metric('--ciwi-select-menu-item-gap', activeControls.select.menuItemGap);
	metric('--ciwi-select-option-gap', activeControls.select.optionGap);
	metric('--ciwi-select-option-padding-x', activeControls.select.optionPaddingX);
	metric('--ciwi-select-option-padding-y', activeControls.select.optionPaddingY);
	metric('--ciwi-select-option-min-height', activeControls.select.optionMinimumHeight);
	metric('--ciwi-select-indicator-width', activeControls.select.selectionIndicatorWidth);
	metric('--ciwi-disclosure-chevron-size', activeControls.disclosure.chevronSize);
	metric('--ciwi-disclosure-chevron-gap', activeControls.disclosure.chevronGap);
	style.setProperty('--ciwi-progress-tint', String(activeControls.progress.tintOpacity * 100) + '%');
	updateResponsiveViewport();
	if (!responsiveViewportBound) {
	  responsiveViewportBound = true;
	  window.addEventListener('resize', updateResponsiveViewport, {passive: true});
	}
  }

  function updateResponsiveViewport() {
	if (!activeControls || !activeControls.viewport) return;
	const width = window.innerWidth;
	const nextCompact = width <= Number(activeControls.viewport.compactMaximumWidth);
	const nextCondensed = width <= Number(activeControls.viewport.condensedDisclosureMaximumWidth);
	const changed = nextCompact !== compactViewport || nextCondensed !== condensedDisclosureViewport;
	compactViewport = nextCompact;
	condensedDisclosureViewport = nextCondensed;
	document.documentElement.classList.toggle('ciwi-compact', compactViewport);
	document.documentElement.classList.toggle('ciwi-condensed-disclosure', condensedDisclosureViewport);
	if (changed && currentDocument && currentData) renderCurrent();
  }

  function appendPositionedIcon(element, label, icon, position) {
	if (position === 'trailing') element.append(label, icon);
	else element.append(icon, label);
  }

  function resolve(data, path) {
    return String(path || '').split('.').reduce((current, part) => {
      if (current === null || current === undefined || !(part in Object(current))) {
        const rootName = String(path || '').split('.')[0];
        const rootBinding = data && data[rootName];
        if (rootBinding && rootBinding.ready === false) return emptyLoadingBindingValue(path);
        throw new Error('Binding not found: ' + path);
      }
      return current[part];
    }, data);
  }

  function emptyLoadingBindingValue(path) {
	const field = String(path || '').split('.').pop() || '';
	if (['projects', 'queued_executions', 'history_executions', 'pipelines', 'visible_pipelines', 'pipeline_chains',
	  'structure_filters', 'timeline', 'job_properties', 'cache_statistics', 'release_summary', 'output_groups',
	  'rows', 'nodes', 'filters', 'children', 'issues', 'agents', 'requirements', 'executions', 'sections', 'jobs',
	  'steps', 'depends_on', 'needs', 'themes', 'connection_modes', 'modes', 'source_refs', 'eligible_agents',
	  'shells', 'connections', 'update_versions', 'rollback_versions'].includes(field)) return [];
	if (['progress', 'server', 'project', 'agent', 'selected_timeline_item', 'scheduling_diagnosis',
	  'host_tool_requirements', 'container_tool_requirements', 'run_context', 'artifacts', 'test_report',
	  'coverage_report', 'structure_root'].includes(field)) return {};
	return '';
  }

  function renderText(text, data) {
    if (!text) return '';
    if (text.literal !== undefined && text.literal !== '') return String(text.literal);
    if (text.binding) return String(resolve(data, text.binding) ?? '');
    return String(text.template || '').replace(/\{\{\s*([^}]+?)\s*\}\}/g, (_, binding) => String(resolve(data, binding) ?? ''));
  }

  function semanticTone(value) {
    switch (String(value || '').trim().toLowerCase()) {
      case 'succeeded': case 'success': case 'passed': case 'complete': case 'completed': case 'online': return 'success';
      case 'failed': case 'failure': case 'error': case 'cancelled': case 'canceled': case 'offline': return 'danger';
      case 'warning': case 'queued': case 'waiting': case 'pending': case 'not reached': case 'stale': case 'deactivated': return 'warning';
      case 'accent': case 'running': case 'leased': case 'in progress': case 'active': return 'accent';
      case 'muted': return 'muted';
      default: return 'muted';
    }
  }

  function withWebOverride(node) {
	const overrides = [];
	if (node.overrides && node.overrides.web) overrides.push(node.overrides.web);
	if (compactViewport && node.overrides && node.overrides.compact) overrides.push(node.overrides.compact);
	if (!overrides.length) return node;
	return overrides.reduce((resolved, override) => Object.assign({}, resolved, {
	  hidden: !!resolved.hidden || !!override.hidden,
	  layout: Object.assign({}, resolved.layout || {}, override.layout || {}),
	  style: Object.assign({}, resolved.style || {}, override.style || {}),
	}), node);
  }

  function applyLayout(element, layout) {
    if (!layout) return;
    const sizes = {
      small: 'var(--ciwi-space-small)', medium: 'var(--ciwi-space-medium)', large: 'var(--ciwi-space-large)',
      'section-padding': 'var(--ciwi-section-padding)',
    };
	const cssLength = value => /^\d+(?:\.\d+)?$/.test(String(value || '').trim()) ? String(value).trim() + 'px' : value;
    if (layout.direction === 'horizontal') element.style.flexDirection = 'row';
    if (layout.direction === 'vertical') element.style.flexDirection = 'column';
    if (layout.gap) element.style.gap = sizes[layout.gap] || cssLength(layout.gap);
    if (layout.padding) {
      const padding = sizes[layout.padding] || cssLength(layout.padding);
      element.style.padding = padding;
      element.style.setProperty('--dsl-layout-padding', padding);
    }
    if (layout.align) {
      const alignment = layout.align === 'middle' ? 'center' : layout.align;
      element.style.alignItems = alignment;
      element.classList.add('dsl-align-' + alignment);
    }
    if (layout.justify) element.style.justifyContent = layout.justify;
    if (layout.wrap) element.style.flexWrap = 'wrap';
    if (layout.grow) {
      element.style.flexGrow = '1';
      element.style.flexBasis = '0';
	  element.style.alignSelf = 'stretch';
	  element.style.width = 'auto';
    }
    if (layout.minWidth) element.style.minWidth = /^\d+$/.test(layout.minWidth) ? layout.minWidth + 'px' : layout.minWidth;
    if (layout.maxWidth && layout.maxWidth !== 'page') element.style.maxWidth = /^\d+$/.test(layout.maxWidth) ? layout.maxWidth + 'px' : layout.maxWidth;
    if (layout.minHeight) element.style.minHeight = /^\d+$/.test(layout.minHeight) ? layout.minHeight + 'px' : layout.minHeight;
    if (layout.maxHeight) element.style.maxHeight = /^\d+$/.test(layout.maxHeight) ? layout.maxHeight + 'px' : layout.maxHeight;
  }

  function runSelectionFromArguments(args) {
    return {
      pipeline_job_id: args.pipelineJobId || '',
      dry_run: args.dryRun === 'true',
      source_ref: args.sourceRef || '',
      agent_id: args.agentId || '',
      execution_mode: args.executionMode || '',
    };
  }

  function showResponseNotice(result) {
    if (result && result.notice && typeof window.ciwiShowNotice === 'function') {
      window.ciwiShowNotice(result.notice);
    }
  }

  function showResponseMessageNotice(result) {
	const message = String(result && result.message || '').trim();
	if (message && typeof window.ciwiShowNotice === 'function') window.ciwiShowNotice({message});
  }

  function routePath() {
	return String(window.location.pathname || '/');
  }

  function browserHistoryState(path, returnPath) {
	browserNavigationSequence += 1;
	return {ciwi: true, navigationID: browserNavigationSequence, path: String(path || '/'), returnPath: String(returnPath || '')};
  }

  function browserNavigationFallback(path) {
	const candidate = String(path || '').trim();
	if (!candidate.startsWith('/') || /^\/projects\/(?:0|$)/.test(candidate)) return '/';
	return candidate;
  }

  async function navigateBrowserBack(fallbackPath) {
	const state = window.history.state || {};
	const returnPath = state.ciwi === true ? String(state.returnPath || '').trim() : '';
	if (returnPath && returnPath !== routePath()) {
	  if (pendingHistoryNavigation) throw new Error('Navigation is already in progress');
	  await new Promise((resolve, reject) => {
		pendingHistoryNavigation = {resolve, reject};
		window.history.back();
	  });
	  return;
	}
	await navigateBrowser(browserNavigationFallback(fallbackPath), {replace: true});
  }

  function routeSegments(path) {
	return String(path || '/').split('/').filter(Boolean);
  }

  function matchRoutePattern(route, path) {
	const expected = routeSegments(route.pattern);
	const actual = routeSegments(path);
	if (expected.length !== actual.length) return null;
	const params = {};
	for (let index = 0; index < expected.length; index += 1) {
	  const segment = expected[index];
	  if (segment.startsWith('{') && segment.endsWith('}')) {
		params[segment.slice(1, -1)] = decodeURIComponent(actual[index]);
	  } else if (segment !== actual[index]) {
		return null;
	  }
	}
	return {route, params};
  }

  async function resolveBrowserRoute(path = routePath()) {
	if (!routeContractPromise) {
	  routeContractPromise = fetch(uiResourceURL('/ui/contracts/routes.json')).then(async response => {
		if (!response.ok) throw new Error(await response.text());
		return response.json();
	  });
	}
	const documentContract = await routeContractPromise;
	if (!screenContractsPreloaded) {
	  screenContractsPreloaded = true;
	  (documentContract.routes || []).forEach(route => {
		if ((route.platforms || []).includes('web')) screenContract(route.screen).catch(() => {});
	  });
	}
	for (const route of (documentContract.routes || [])) {
	  if (!(route.platforms || []).includes('web')) continue;
	  const match = matchRoutePattern(route, path);
	  if (match) return match;
	}
	throw new Error('Unsupported declarative route: ' + routePath());
  }

  function screenContract(name) {
	if (!screenContractPromises.has(name)) {
	  screenContractPromises.set(name, fetch(uiResourceURL('/ui/contracts/screens/' + encodeURIComponent(name) + '.json')).then(async response => {
		if (!response.ok) throw new Error(await response.text());
		return response.json();
	  }));
	}
	return screenContractPromises.get(name);
  }

  function themeContracts() {
	if (!themeContractPromise) {
	  themeContractPromise = fetch(uiResourceURL('/ui/contracts/themes.json')).then(async response => {
		if (!response.ok) throw new Error(await response.text());
		return response.json();
	  });
	}
	return themeContractPromise;
  }

  function runOptionsViewURL(sourceRef, agentID, routeMatch = currentRouteMatch) {
    let path = '';
	if (routeMatch && routeMatch.route.name === 'pipeline-run-options') path = '/api/v1/views/run-options/pipelines/' + encodeURIComponent(routeMatch.params.pipelineId);
    if (routeMatch && routeMatch.route.name === 'legacy-pipeline-run-options') path = '/api/v1/views/run-options/pipelines/' + encodeURIComponent(routeMatch.params.pipelineId);
    if (routeMatch && routeMatch.route.name === 'chain-run-options') path = '/api/v1/views/run-options/projects/' + encodeURIComponent(routeMatch.params.projectId) + '/chains/' + encodeURIComponent(routeMatch.params.chainId);
    if (!path) return '';
    const query = new URLSearchParams();
    if (sourceRef) query.set('source_ref', sourceRef);
    if (agentID) query.set('agent_id', agentID);
    return path + (query.size ? '?' + query.toString() : '');
  }

  function elementFor(node) {
    switch (node.component) {
      case 'page': return document.createElement('main');
      case 'section': return document.createElement('section');
      case 'card': return document.createElement('article');
      case 'disclosure': return document.createElement('details');
      case 'button': return document.createElement('button');
      case 'select': return document.createElement('button');
      case 'input': return document.createElement(node.input && node.input.multiline ? 'textarea' : 'input');
      case 'image': return document.createElement('img');
      case 'divider': return document.createElement('hr');
      case 'spacer': return document.createElement('span');
      default: return document.createElement('div');
    }
  }

  function elementContainsTextSelection(element) {
    const selection = window.getSelection && window.getSelection();
    if (!selection || selection.isCollapsed || !selection.anchorNode || !selection.focusNode) return false;
    return element.contains(selection.anchorNode) || element.contains(selection.focusNode);
  }

  function renderActionConfirmation(confirmation, data) {
	if (!confirmation) return null;
	return Object.fromEntries(Object.entries(confirmation).map(([key, value]) => [
	  key,
	  typeof value === 'string' ? renderText({template: value}, data) : value,
	]));
  }

  function bindActions(element, actions, data, context) {
	const bindings = [];
    (actions || []).forEach(action => {
      const invoke = async (actionData, actionElement) => {
        const args = Object.fromEntries(Object.entries(action.arguments || {}).map(([key, value]) => [key, renderText({ template: value }, actionData)]));
        if (!window.ciwiConfirmAction(renderActionConfirmation(action.confirm, actionData))) return;
		const invokedPath = routePath();
		const invokedNavigationID = Number(window.history.state && window.history.state.navigationID || 0);
        const execute = async runtime => {
		let navigatedAfterSuccess = false;
		if (action.command === 'navigate' && args.route) {
		  const previousDisabled = !!actionElement.disabled;
		  actionElement.disabled = true;
		  actionElement.setAttribute('aria-busy', 'true');
		  actionElement.classList.add('ciwi-action-pending');
		  try {
			await navigateBrowser(args.route, {section: args.section || ''});
		  } finally {
			actionElement.disabled = previousDisabled;
			actionElement.removeAttribute('aria-busy');
			actionElement.classList.remove('ciwi-action-pending');
		  }
        }
		else if (action.command === 'navigate-back') {
		  await navigateBrowserBack(args.fallbackRoute);
		}
        else if (action.command === 'open-url' && args.url) window.open(args.url, '_blank', 'noopener,noreferrer');
		else if (action.command === 'refresh') await refresh({throwOnError: true, showLoading: true});
		else if (action.command === 'change-theme') {
		  const selectedTheme = ciwiApplyTheme(args.theme);
		  const settings = currentData && currentData.settings;
		  if (settings) {
			settings.selected_theme = selectedTheme;
			const selected = (settings.themes || []).find(theme => theme.name === selectedTheme);
			settings.selected_theme_description = selected ? selected.description : '';
			document.querySelectorAll('[data-ciwi-binding="settings.selected_theme_description"]').forEach(target => {
			  target.textContent = settings.selected_theme_description;
			});
		  }
        }
		else if (action.command === 'set-project-structure-filter') {
		  const details = currentData && currentData.projectDetails;
		  if (!details) throw new Error('Project structure is unavailable');
		  viewBindings.applyProjectStructureFilter(details, args.value || 'all-pipelines');
		  renderCurrent();
		}
		else if (action.command === 'set-report-filter') {
		  const report = currentData && currentData.jobDetails && currentData.jobDetails.test_report;
		  if (!report) throw new Error('Test report is unavailable');
		  report.filter = args.value || 'all';
		  renderCurrent();
		}
		else if (action.command === 'download-artifact') {
		  const jobID = encodeURIComponent(args.jobExecutionId || '');
		  const kind = String(args.kind || 'all');
		  const artifactPath = String(args.path || '');
		  let url = '/api/v1/jobs/' + jobID + '/artifacts/download-all';
		  if (kind === 'prefix') url = '/api/v1/jobs/' + jobID + '/artifacts/download?prefix=' + encodeURIComponent(artifactPath);
		  if (kind === 'file') url = '/artifacts/' + jobID + '/' + artifactPath.split('/').map(encodeURIComponent).join('/');
		  const anchor = document.createElement('a');
		  anchor.href = url;
		  if (kind === 'file') {
			anchor.target = '_blank';
			anchor.rel = 'noopener noreferrer';
		  } else {
			anchor.download = '';
		  }
		  document.body.appendChild(anchor);
		  anchor.click();
		  anchor.remove();
		}
		else if (action.command === 'download-job-log') {
		  const jobID = encodeURIComponent(args.jobExecutionId || '');
		  const format = args.format === 'raw' ? 'raw' : 'clean';
		  const anchor = document.createElement('a');
		  anchor.href = '/api/v1/jobs/' + jobID + '/log?format=' + format;
		  anchor.download = '';
		  document.body.appendChild(anchor);
		  anchor.click();
		  anchor.remove();
		}
        else if (action.command === 'select-timeline-item') {
          data.jobDetails.selected_timeline_item = data.item;
          renderCurrent();
          revealBrowserOutputGroup(data.jobDetails, args.id);
        }
        else if (action.command === 'change-output-search') {
          data.jobDetails.output_search = args.query || '';
		  if (data.jobDetails.interactive_log_available) scheduleFullLogSearch(data.jobDetails);
		  else {
			updateOutputSearch(data.jobDetails, 0);
			patchJobOutputRegion(data.jobDetails);
		  }
        }
        else if (action.command === 'find-output') {
		  if (data.jobDetails.interactive_log_available) {
			await findFullLogMatch(data.jobDetails, args.direction === 'previous' ? -1 : 1);
		  } else {
			updateOutputSearch(data.jobDetails, args.direction === 'previous' ? -1 : 1);
			patchJobOutputRegion(data.jobDetails);
			selectBrowserOutputMatch(data.jobDetails);
		  }
        }
        else if (action.command === 'copy-output') {
		  const response = await fetch('/api/v1/jobs/' + encodeURIComponent(data.jobDetails.id || '') + '/log?format=clean');
		  if (!response.ok) throw new Error(await response.text());
		  await navigator.clipboard.writeText(await response.text());
        }
        else if (action.command === 'toggle-output-tailing') {
		  setOutputTailing(data.jobDetails, !data.jobDetails.output_tailing);
		  if (data.jobDetails.output_tailing) {
			if (data.jobDetails.interactive_log_available) {
			  logViewStates.forEach(state => {
				if (state.jobID !== String(data.jobDetails.id || '')) return;
				const last = state.chunks.length ? Number(state.chunks[state.chunks.length - 1].id) : 0;
				loadLogViewPage(state, state.hasAfter && last ? 'after' : 'tail', last);
			  });
			} else scrollJobOutputToEnd(document.getElementById('job-output-groups'));
		  }
        }
		else if (action.command === 'set-disclosures') {
          const expanded = args.expanded === 'true';
          document.querySelectorAll('[data-disclosure-key^="' + CSS.escape(args.prefix || '') + '"]').forEach(details => {
            details.open = expanded;
			disclosureStates.set(details.dataset.disclosureKey, expanded);
		  });
		  requestAnimationFrame(updateDeclarativeOutputCollapseButtons);
        }
		else if (action.command === 'set-run-option') {
		  const options = currentData && currentData.runOptions;
		  if (!options) throw new Error('Run options are unavailable');
		  if (args.field === 'sourceRef') {
		    options.selected_source_ref = args.value || '';
		    options.selected_agent_id = '';
		  } else if (args.field === 'agentId') {
		    options.selected_agent_id = args.value || '';
		    renderCurrent();
		    return;
		  } else {
		    throw new Error('Unsupported run option');
		  }
		  const response = await fetch(runOptionsViewURL(options.selected_source_ref, options.selected_agent_id), {signal: runtime.signal});
		  if (!response.ok) throw new Error(await response.text());
		  const refreshedOptions = viewBindings.markBrowserViewReady(await response.json());
		  currentData = {runOptions: refreshedOptions, client: viewBindings.browserClientBinding()};
		  browserViewCache.set(routePath(), refreshedOptions);
		  renderCurrent();
		}
		else if (action.command === 'set-managed-yaml-field') {
		  const editor = currentData && currentData.managedYAML;
		  if (!editor || args.field !== 'yaml') throw new Error('Managed YAML editor is unavailable');
		  editor.yaml = args.value || '';
		  editor.result = '';
		}
		else if (action.command === 'validate-managed-yaml') {
		  const response = await fetch('/api/v1/projects/managed-yaml/validate', {
			method: 'POST', headers: {'Content-Type': 'application/json'},
			body: JSON.stringify({project_id: Number(args.projectId || 0), yaml: args.yaml || ''}), signal: runtime.signal,
		  });
		  if (!response.ok) throw new Error(await response.text());
		  const result = await response.json();
		  const editor = currentData.managedYAML;
		  editor.project_name = result.project_name || editor.project_name;
		  editor.result = 'Valid · ' + String(Number(result.pipelines || 0)) + ' pipeline(s), ' + String(Number(result.pipeline_chains || 0)) + ' chain(s)';
		  editor.result_tone = 'success';
		  renderCurrent();
		}
		else if (action.command === 'save-managed-yaml') {
		  const projectID = Number(args.projectId || 0);
		  const path = projectID > 0
			? '/api/v1/projects/' + encodeURIComponent(projectID) + '/managed-yaml'
			: '/api/v1/projects/managed-yaml';
		  const body = projectID > 0
			? {revision: args.revision || '', yaml: args.yaml || ''}
			: {yaml: args.yaml || ''};
		  const response = await fetch(path, {
			method: projectID > 0 ? 'PUT' : 'POST',
			headers: ciwiActionHeaders(runtime, {'Content-Type': 'application/json'}),
			body: JSON.stringify(body), signal: runtime.signal,
		  });
		  if (!response.ok) throw new Error(await response.text());
		  const result = await response.json();
		  await navigateBrowser('/managed-yaml/' + encodeURIComponent(result.project_id));
		}
		else if (action.command === 'set-agent-script-field') {
		  const script = currentData && currentData.agentScript;
		  if (!script) throw new Error('Agent script editor is unavailable');
		  if (args.field === 'shell') {
			script.selected_shell = args.value || '';
			const shell = script.shells.find(option => option.value === script.selected_shell);
			if (shell) script.script = shell.example_script || '';
		  } else if (args.field === 'script') script.script = args.value || '';
		  else throw new Error('Unknown agent script field');
		  script.result = '';
		  renderCurrent();
		}
		else if (action.command === 'run-agent-script') {
		  const response = await fetch('/api/v1/agents/' + encodeURIComponent(args.agentId) + '/actions', {
			method: 'POST', headers: ciwiActionHeaders(runtime, {'Content-Type': 'application/json'}),
			body: JSON.stringify({action: 'run-script', shell: args.shell || '', script: args.script || ''}), signal: runtime.signal,
		  });
		  if (!response.ok) throw new Error(await response.text());
		  const result = await response.json();
		  if (!result.job_execution_id) throw new Error('Script response did not include a job identifier');
		  showResponseNotice(result);
		  await navigateBrowser('/jobs/' + encodeURIComponent(result.job_execution_id));
		}
		else if (action.command === 'set-vault-field') {
		  const vault = currentData && currentData.vault;
		  if (!vault || !Object.prototype.hasOwnProperty.call(vault, args.field)) throw new Error('Vault editor is unavailable');
		  vault[args.field] = args.value || '';
		  vault.result = '';
		}
		else if (action.command === 'save-vault-connection') {
		  const response = await fetch('/api/v1/vault/connections', {
			method: 'POST', headers: ciwiActionHeaders(runtime, {'Content-Type': 'application/json'}),
			body: JSON.stringify({
			  name: args.name || '', url: args.url || '', auth_method: 'approle',
			  approle_mount: args.approleMount || 'approle', role_id: args.roleId || '', secret_id_env: args.secretIdEnv || '',
			}), signal: runtime.signal,
		  });
		  if (!response.ok) throw new Error(await response.text());
		  await refresh();
		}
		else if (action.command === 'test-vault-connection') {
		  const response = await fetch('/api/v1/vault/connections/' + encodeURIComponent(args.id) + '/test', {
			method: 'POST', headers: {'Content-Type': 'application/json'}, body: '{}', signal: runtime.signal,
		  });
		  if (!response.ok) throw new Error(await response.text());
		  const result = await response.json();
		  currentData.vault.result = (result.ok ? 'OK: ' : 'FAILED: ') + String(result.message || '');
		  currentData.vault.result_tone = result.ok ? 'success' : 'danger';
		  renderCurrent();
		}
		else if (action.command === 'delete-vault-connection') {
		  const response = await fetch('/api/v1/vault/connections/' + encodeURIComponent(args.id), {
			method: 'DELETE', headers: ciwiActionHeaders(runtime), signal: runtime.signal,
		  });
		  if (!response.ok) throw new Error(await response.text());
		  await refresh();
		}
        else if (action.command === 'run-pipeline') {
          const response = await fetch('/api/v1/pipelines/' + encodeURIComponent(args.pipelineDbId) + '/run-selection', {
            method: 'POST',
            headers: ciwiActionHeaders(runtime, {'Content-Type': 'application/json'}),
            body: JSON.stringify(runSelectionFromArguments(args)),
            signal: runtime.signal,
          });
          if (!response.ok) throw new Error(await response.text());
		  const result = await response.json();
		  showResponseNotice(result);
		  if (args.backOnSuccess === 'true' && routePath() === invokedPath && Number(window.history.state && window.history.state.navigationID || 0) === invokedNavigationID) {
			await navigateBrowserBack(args.fallbackRoute);
			navigatedAfterSuccess = true;
		  }
        }
		else if (action.command === 'run-chain') {
		  const path = '/api/v1/projects/' + encodeURIComponent(args.projectId) + '/pipeline-chains/' + encodeURIComponent(args.chainId) + '/run';
		  const response = await fetch(path, {
		    method: 'POST', headers: ciwiActionHeaders(runtime, {'Content-Type': 'application/json'}),
		    body: JSON.stringify(runSelectionFromArguments(args)),
		    signal: runtime.signal,
		  });
		  if (!response.ok) throw new Error(await response.text());
		  const result = await response.json();
		  showResponseNotice(result);
		  if (args.backOnSuccess === 'true' && routePath() === invokedPath && Number(window.history.state && window.history.state.navigationID || 0) === invokedNavigationID) {
			await navigateBrowserBack(args.fallbackRoute);
			navigatedAfterSuccess = true;
		  }
		}
		else if (action.command === 'agent-action') {
		  const response = await fetch('/api/v1/agents/' + encodeURIComponent(args.agentId) + '/actions', {
		    method: 'POST',
		    headers: ciwiActionHeaders(runtime, {'Content-Type': 'application/json'}),
		    body: JSON.stringify({action: args.action}),
		    signal: runtime.signal,
		  });
		  if (!response.ok) throw new Error(await response.text());
		  const result = await response.json();
		  showResponseMessageNotice(result);
		  if (args.action === 'delete' && args.successRoute) {
			await navigateBrowser(args.successRoute, {replace: true});
			navigatedAfterSuccess = true;
		  } else await refresh();
		}
		else if (action.command === 'project-action') {
		  const path = '/api/v1/projects/' + encodeURIComponent(args.projectId) + (args.action === 'reload' ? '/reload' : '');
		  const response = await fetch(path, {
		    method: args.action === 'delete' ? 'DELETE' : 'POST',
		    headers: ciwiActionHeaders(runtime, {'Content-Type': 'application/json'}),
		    body: args.action === 'delete' ? undefined : '{}',
		    signal: runtime.signal,
		  });
		  if (!response.ok) throw new Error(await response.text());
		  await refresh();
		}
		else if (action.command === 'set-project-import-field') {
		  const settings = currentData && currentData.settings;
		  if (!settings) throw new Error('Settings are unavailable');
		  const binding = {repoUrl: 'import_repo_url', repoRef: 'import_repo_ref', configFile: 'import_config_file'}[args.field];
		  if (!binding) throw new Error('Unknown project import field');
		  settings[binding] = args.value || '';
		}
		else if (action.command === 'set-connection-field') {
		  const connection = (currentData && currentData.connection) || (currentData && currentData.settings);
		  if (!connection) throw new Error('Connection settings are unavailable');
		  if (args.field === 'mode') {
		    const mode = args.value === 'explicit' ? 'explicit' : 'discover';
		    if (currentData.connection) {
		      connection.mode = mode;
		      connection.explicit = mode === 'explicit';
		    } else {
		      connection.connection_mode = mode;
		      connection.connection_explicit = mode === 'explicit';
		    }
		  } else if (args.field === 'endpoint') {
		    if (currentData.connection) connection.endpoint = args.value || '';
		    else connection.connection_endpoint = args.value || '';
		  } else throw new Error('Unknown connection field');
		  renderCurrent();
		}
		else if (action.command === 'save-connection' || action.command === 'retry-connection') {
		  if (currentData && currentData.connection) {
		    currentData.connection.status = 'Native connection changes are demonstrated by the desktop client.';
		    currentData.connection.status_tone = 'muted';
		    renderCurrent();
		  }
		}
		else if (action.command === 'import-project') {
		  const response = await fetch('/api/v1/projects/import', {
		    method: 'POST',
		    headers: ciwiActionHeaders(runtime, {'Content-Type': 'application/json'}),
		    body: JSON.stringify({repo_url: args.repoUrl, repo_ref: args.repoRef, config_file: args.configFile}),
		    signal: runtime.signal,
		  });
		  if (!response.ok) throw new Error(await response.text());
		  await refresh();
		}
		else if (action.command === 'set-server-update-option') {
		  const settings = currentData && currentData.settings;
		  if (!settings) throw new Error('Settings are unavailable');
		  const binding = args.field === 'rollback' ? 'selected_rollback_version' : 'selected_update_version';
		  settings[binding] = args.value || '';
		}
		else if (action.command === 'check-server-updates') {
		  const response = await fetch('/api/v1/update/check', {method: 'POST', headers: {'Content-Type': 'application/json'}, body: '{}', signal: runtime.signal});
		  if (!response.ok) throw new Error(await response.text());
		  const result = await response.json();
		  const settings = currentData.settings;
		  const versions = Array.isArray(result.available_versions) ? result.available_versions : [];
		  settings.update_versions = declarativeVersionOptions(versions, 'No newer versions available');
		  settings.selected_update_version = versions[0] || '';
		  settings.update_result = result.update_available
		    ? 'Update available: ' + result.current_version + ' → ' + result.latest_version
		    : (result.message || 'Up to date (' + result.current_version + ')');
		  settings.update_result_tone = 'success';
		  renderCurrent();
		}
		else if (action.command === 'refresh-rollback-versions') {
		  const response = await fetch('/api/v1/update/tags', {signal: runtime.signal});
		  if (!response.ok) throw new Error(await response.text());
		  const result = await response.json();
		  const versions = (Array.isArray(result.tags) ? result.tags : []).filter(version => isDeclarativeLowerVersion(version, result.current_version));
		  const settings = currentData.settings;
		  settings.rollback_versions = declarativeVersionOptions(versions, 'No lower versions available');
		  settings.selected_rollback_version = versions[0] || '';
		  settings.rollback_result = 'Found ' + versions.length + ' rollback version(s)';
		  settings.rollback_result_tone = 'success';
		  renderCurrent();
		}
		else if (action.command === 'server-update-action') {
		  const path = args.action === 'restart' ? '/api/v1/server/restart' : (args.action === 'rollback' ? '/api/v1/update/rollback' : '/api/v1/update/apply');
		  const settings = currentData.settings;
		  if (args.action === 'apply') {
			settings.update_versions = declarativeVersionOptions([], 'Click "Check for updates"');
			settings.selected_update_version = '';
			renderCurrent();
		  }
		  const response = await fetch(path, {
		    method: 'POST', headers: ciwiActionHeaders(runtime, {'Content-Type': 'application/json'}),
		    body: JSON.stringify(args.action === 'restart' ? {} : {target_version: args.targetVersion || ''}),
		    signal: runtime.signal,
		  });
		  if (!response.ok) throw new Error(await response.text());
		  const result = await response.json();
		  const field = args.action === 'rollback' ? 'rollback_result' : 'update_result';
		  settings[field] = result.message || 'Request accepted';
		  settings[field + '_tone'] = 'success';
		  renderCurrent();
		}
		else if (action.command === 'clear-queue') {
		  const response = await fetch('/api/v1/jobs/clear-queue', {method: 'POST', headers: ciwiActionHeaders(runtime, {'Content-Type': 'application/json'}), body: '{}', signal: runtime.signal});
		  if (!response.ok) throw new Error(await response.text());
		}
		else if (action.command === 'remove-execution') {
		  const response = await fetch('/api/v1/jobs/' + encodeURIComponent(args.jobExecutionId), {method: 'DELETE', headers: ciwiActionHeaders(runtime), signal: runtime.signal});
		  if (!response.ok) throw new Error(await response.text());
		}
		else if (action.command === 'flush-history' || action.command === 'delete-execution') {
		  const ids = action.command === 'delete-execution'
		    ? String(args.jobExecutionIds || '').split(',').map(value => value.trim()).filter(Boolean)
		    : null;
		  const body = ids === null ? {} : {job_execution_ids: ids};
		  const response = await fetch('/api/v1/jobs/flush-history', {
		    method: 'POST', headers: ciwiActionHeaders(runtime, {'Content-Type': 'application/json'}), body: JSON.stringify(body), signal: runtime.signal,
		  });
		  if (!response.ok) throw new Error(await response.text());
		  await refresh();
		}
		else if (action.command === 'cancel-execution') {
		  const response = await fetch('/api/v1/jobs/' + encodeURIComponent(args.jobExecutionId) + '/cancel', {method: 'POST', headers: ciwiActionHeaders(runtime), signal: runtime.signal});
		  if (!response.ok) throw new Error(await response.text());
		  await refresh();
		}
		else if (action.command === 'rerun-execution') {
		  const response = await fetch('/api/v1/jobs/' + encodeURIComponent(args.jobExecutionId) + '/rerun', {method: 'POST', headers: ciwiActionHeaders(runtime), signal: runtime.signal});
		  if (!response.ok) throw new Error(await response.text());
		  const result = await response.json();
		  const rerunID = result && result.job_execution && result.job_execution.id;
		  if (!rerunID) throw new Error('Rerun response did not include an execution identifier');
		  showResponseNotice(result);
		  await navigateBrowser('/jobs/' + encodeURIComponent(rerunID));
		}
        else throw new Error('Command is not implemented by the web proof renderer: ' + action.command);
		if (runtime.refreshOnSuccess && !navigatedAfterSuccess) await refresh({throwOnError: true});
        };
        if (typeof window.ciwiRunAction === 'function') return window.ciwiRunAction(action.command, args, actionElement, execute);
        const idempotencyKey = typeof window.ciwiActionID === 'function' ? window.ciwiActionID() : '';
        return execute({signal: undefined, idempotencyKey});
      };
      if (action.on === 'activate') {
        element.tabIndex = element.tabIndex >= 0 ? element.tabIndex : 0;
        element.setAttribute('role', element.tagName === 'BUTTON' ? 'button' : 'link');
      }
	  bindings.push({action, data, invoke});
    });
	if (!bindings.length || !context || !context.identity) return;
	element.dataset.ciwiActionKey = context.identity;
	context.session.actionBindings.set(context.identity, bindings);
  }

  function delegatedActionElement(event) {
	const target = event.target && event.target.closest ? event.target.closest('[data-ciwi-action-key]') : null;
	return target && root.contains(target) ? target : null;
  }

  function reportDelegatedActionError(error) {
	window.alert(error && error.message ? error.message : String(error));
  }

  function invokeDelegatedActions(element, eventName) {
	const key = String(element.dataset.ciwiActionKey || '');
	const bindings = committedActionBindings.get(key) || [];
	bindings.filter(binding => binding.action.on === eventName).forEach(binding => {
	  let actionData = binding.data;
	  if (eventName === 'change') {
		const selected = element.options && element.selectedIndex >= 0 ? element.options[element.selectedIndex] : null;
		actionData = element.tagName === 'INPUT' || element.tagName === 'TEXTAREA'
		  ? Object.assign({}, binding.data, {input: {value: element.value}})
		  : Object.assign({}, binding.data, {selection: {
			value: element.value,
			label: selected ? selected.textContent : (element.dataset.selectedLabel || element.value),
		  }});
	  }
	  binding.invoke(actionData, element).catch(reportDelegatedActionError);
	});
  }

  root.addEventListener('click', event => {
	const element = delegatedActionElement(event);
	if (!element) return;
	const bindings = committedActionBindings.get(String(element.dataset.ciwiActionKey || '')) || [];
	if (!bindings.some(binding => binding.action.on === 'activate')) return;
	const summary = element.closest('summary');
	if (element.tagName === 'BUTTON' || summary) event.stopPropagation();
	// A click on any descendant of <summary> performs the summary's built-in
	// toggle. A child action owns that gesture, so suppress the default toggle.
	if (summary) event.preventDefault();
	if (element.tagName !== 'BUTTON' && elementContainsTextSelection(element)) return;
	invokeDelegatedActions(element, 'activate');
  }, true);

  root.addEventListener('keydown', event => {
	if (event.key !== 'Enter' && event.key !== ' ') return;
	const element = delegatedActionElement(event);
	if (!element) return;
	const bindings = committedActionBindings.get(String(element.dataset.ciwiActionKey || '')) || [];
	if (!bindings.some(binding => binding.action.on === 'activate')) return;
	event.preventDefault();
	event.stopPropagation();
	invokeDelegatedActions(element, 'activate');
  }, true);

  const handleDelegatedChange = event => {
	const element = delegatedActionElement(event);
	if (!element) return;
	const expectedEvent = element.id === 'job-output-search' ? 'input' : 'change';
	if (event.type !== expectedEvent) return;
	invokeDelegatedActions(element, 'change');
  };
  root.addEventListener('input', handleDelegatedChange);
  root.addEventListener('change', handleDelegatedChange);

  function declarativeIcon(name) {
    const icon = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
    icon.classList.add('dsl-icon');
	icon.dataset.ciwiIcon = String(name || '');
    icon.setAttribute('aria-hidden', 'true');
    const use = document.createElementNS('http://www.w3.org/2000/svg', 'use');
    use.setAttribute('href', uiResourceURL('/ui/icons.svg') + '#icon-' + name);
    icon.appendChild(use);
    return icon;
  }

  function renderNode(rawNode, data, context) {
    const node = withWebOverride(rawNode);
    if (node.hidden) return document.createDocumentFragment();
    if (node.visible) {
	  const equal = node.visible.empty
	    ? String(resolve(data, node.visible.binding)) === ''
	    : String(resolve(data, node.visible.binding)) === String(node.visible.equals || 'true');
      if (node.visible.not ? equal : !equal) return document.createDocumentFragment();
    }
    if (node.repeat && node.component !== 'list' && node.component !== 'scroller') {
      const fragment = document.createDocumentFragment();
      repeatedItems(node, data, context.path).forEach(({itemData, key}) => {
        const clone = Object.assign({}, node, { repeat: null });
		const repeatIdentity = context.path + '/repeat:' + rendererKeyPart(key);
        fragment.appendChild(renderNode(clone, itemData, {
		  session: context.session,
		  path: context.path,
		  parentScope: context.parentScope,
		  repeatIdentity,
		  inRepeat: true,
		}));
      });
      return fragment;
    }
    const element = elementFor(node);
	const identity = nodeRenderIdentity(node, data, context);
	context = Object.assign({}, context, {identity});
	annotateRendererElement(element, node.component, identity);
    element.classList.add('dsl-' + node.component);
    if (node.id) element.id = node.id;
    const style = node.style || {};
    if (style.role) element.classList.add('dsl-' + style.role);
	if (style.role === 'floating-collapse') element.hidden = true;
    const tone = style.toneBinding ? semanticTone(resolve(data, style.toneBinding)) : style.tone;
    if (tone) element.classList.add('dsl-' + tone);
    if (style.emphasis) element.classList.add('dsl-' + style.emphasis);
    if (style.truncate) element.classList.add('dsl-truncate');
	applyLayout(element, node.layout);
	if (node.component === 'graph-view' && node.graphView) {
	  graphViewRenderer.render(element, node, data, context);
	  return element;
	}
	if (node.component === 'tree-view' && node.treeView) {
	  treeViewRenderer.render(element, node, data, context);
	  return element;
	}
	if (node.component === 'log-view' && node.logView) {
	  renderBrowserLogView(element, node.logView, data);
	  return element;
	}
	if (node.enabled) {
	  const equal = node.enabled.empty
	    ? String(resolve(data, node.enabled.binding)) === ''
	    : String(resolve(data, node.enabled.binding)) === String(node.enabled.equals || 'true');
	  element.disabled = node.enabled.not ? equal : !equal;
	}
	if ('disabled' in element) element.__ciwiRenderedDisabled = !!element.disabled;
    if (node.component === 'disclosure') {
      const summary = document.createElement('summary');
	  summary.classList.add('dsl-disclosure-chevron-' + activeControls.disclosure.chevronPosition);
	  if (style.role === 'execution-row' && node.image) {
	    const image = document.createElement('img');
	    image.className = 'dsl-execution-row-image';
	    image.src = node.image.asset === 'ciwi-logo' ? uiResourceURL('/ciwi-logo.png') : node.image.asset;
	    image.alt = node.image.description || '';
	    summary.appendChild(image);
		const statusTone = semanticTone(resolve(data, node.style.toneBinding));
		const statusIcon = {success: 'circle-check', danger: 'circle-x', warning: 'clock', accent: 'loader-2'}[statusTone] || 'clock';
	    const status = declarativeIcon(statusIcon);
	    status.classList.add('dsl-execution-row-status', 'dsl-status-' + statusTone);
	    summary.appendChild(status);
	    const label = document.createElement('span');
	    label.className = 'dsl-execution-row-label';
	    label.textContent = renderText(node.text, data) || 'Details';
	    summary.appendChild(label);
	  } else {
		if (node.image) {
		  const imageSource = node.image.binding
			? String(resolve(data, node.image.binding) || '')
			: (node.image.asset === 'ciwi-logo' ? uiResourceURL('/ciwi-logo.png') : node.image.asset);
		  if (imageSource) {
			const image = document.createElement('img');
			image.className = 'dsl-disclosure-image';
			image.src = imageSource;
			image.alt = node.image.description || '';
			if (node.image.binding) image.addEventListener('error', () => { image.style.visibility = 'hidden'; }, {once: true});
			summary.appendChild(image);
		  }
		}
		const label = document.createElement('span');
		label.className = 'dsl-disclosure-label';
		label.textContent = renderText(node.text, data) || 'Details';
		summary.appendChild(label);
      }
      element.appendChild(summary);
      if (node.disclosure) {
        const stateKey = node.disclosure.stateKey ? renderText({template: node.disclosure.stateKey}, data) : '';
		const defaultExpanded = node.disclosure.defaultExpandedBinding
		  ? !!resolve(data, node.disclosure.defaultExpandedBinding)
		  : !!node.disclosure.defaultExpanded;
        if (stateKey) {
          element.dataset.disclosureKey = stateKey;
          element.open = disclosureStates.get(stateKey, defaultExpanded);
          element.addEventListener('toggle', () => {
			disclosureStates.set(stateKey, element.open);
			requestAnimationFrame(updateDeclarativeOutputCollapseButtons);
          });
        } else {
          element.open = defaultExpanded;
        }
		(node.disclosure.summary || []).forEach((summaryNode, index) => {
		  const summaryElement = renderNode(
		    summaryNode,
		    data,
		    childRenderContext(context, 'summary:' + String(index)),
		  );
		  if (style.role === 'execution-row' && summaryElement.classList) {
		    if (summaryNode.component === 'text') summaryElement.classList.add('dsl-execution-row-summary');
		    if (summaryNode.component === 'button') summaryElement.classList.add('dsl-execution-row-action');
		  }
		  summary.appendChild(summaryElement);
		});
      }
    } else if (node.component === 'icon' && node.icon) {
      element.appendChild(declarativeIcon(node.icon));
      element.setAttribute('role', 'img');
      element.setAttribute('aria-label', node.icon === 'heart' ? 'Heartbeat' : node.icon);
      if (node.pulse && node.pulse.binding) bindTimestampPulse(element, resolve(data, node.pulse.binding));
    } else if (node.component === 'image' && node.image) {
	  const imageSource = node.image.binding
		? String(resolve(data, node.image.binding) || '')
		: (node.image.asset === 'ciwi-logo' ? uiResourceURL('/ciwi-logo.png') : node.image.asset);
	  if (!imageSource) return document.createDocumentFragment();
	  element.src = imageSource;
      element.alt = node.image.description || '';
	  if (node.image.binding) element.addEventListener('error', () => { element.style.visibility = 'hidden'; }, {once: true});
    } else if (node.component === 'select' && node.select) {
      const options = resolve(data, node.select.options);
      const current = String(resolve(data, node.select.value));
	  const renderedOptions = (Array.isArray(options) ? options : []).map(item => {
        const optionData = Object.assign({}, data, {[node.select.as]: item});
		return {
		  value: String(resolve(optionData, node.select.optionValue)),
		  label: String(resolve(optionData, node.select.optionLabel)),
		};
      });
	  browserSelectControl.configure(element, renderedOptions, current);
    } else if (node.component === 'input' && node.input) {
	  if (!node.input.multiline) element.type = 'text';
	  if (node.input.minLines) {
		const minimumLines = Math.max(1, Number(node.input.minLines));
		element.rows = minimumLines;
		element.style.minHeight = String(minimumLines * 24) + 'px';
	  }
      element.value = String(resolve(data, node.input.value) ?? '');
	  element.__ciwiRenderedValue = element.value;
      element.placeholder = node.input.placeholder || '';
    } else if (node.text) {
      const text = renderText(node.text, data);
	  if (node.text.binding) element.dataset.ciwiBinding = node.text.binding;
      if ((node.id === 'job-output-system-text' || node.id === 'job-output-group-text') && data.jobDetails) {
		const itemID = node.id === 'job-output-group-text' && data.outputGroup ? String(data.outputGroup.id || '') : '';
		renderBrowserOutputText(element, text, itemID, data.jobDetails);
	  } else {
		element.textContent = text;
	  }
    }
    if (node.component === 'button' && style.role === 'icon-button') {
	  const accessibleLabel = element.textContent || 'Action';
	  element.setAttribute('aria-label', accessibleLabel);
	  element.title = accessibleLabel;
	  element.textContent = '';
	}
	if (node.component === 'button' && style.role !== 'icon-button' && style.role !== 'tailing-toggle') {
	  const copy = element.textContent;
	  element.textContent = '';
	  const label = document.createElement('span');
	  label.className = 'dsl-button-label';
	  const current = document.createElement('span');
	  current.className = 'dsl-button-label-current';
	  current.textContent = copy;
	  label.appendChild(current);
	  element.appendChild(label);
	}
    if (node.component === 'button' && node.icon) {
	  const icon = declarativeIcon(node.icon);
	  const label = element.querySelector('.dsl-button-label');
	  if (label) appendPositionedIcon(element, label, icon, activeControls.button.iconPosition);
	  else element.prepend(icon);
    }
    bindActions(element, node.actions, data, context);
	if (node.component === 'button' && node.actions && node.actions.length && typeof window.ciwiReservePendingLabel === 'function') {
	  window.ciwiReservePendingLabel(element, node.actions[0].command);
	}
	const childrenTarget = element;
    if ((node.component === 'list' || node.component === 'scroller') && node.repeat) {
	  repeatedItems(node, data, context.path).forEach(({itemData, key}) => {
		(node.children || []).forEach((child, index) => {
		  const repeatIdentity = context.identity + '/repeat:' + rendererKeyPart(key) + '/child:' + String(index);
		  childrenTarget.appendChild(renderNode(
			child,
			itemData,
			childRenderContext(context, 'children:' + String(index), repeatIdentity),
		  ));
		});
      });
    } else {
	  (node.children || []).forEach((child, index) => childrenTarget.appendChild(renderNode(
		child,
		data,
		childRenderContext(context, 'children:' + String(index)),
	  )));
    }
    if (node.progress && node.progress.binding) {
      const target = node.component === 'disclosure' ? element.querySelector(':scope > summary') : element;
      bindSemanticProgress(target || element, resolve(data, node.progress.binding));
    }
    return element;
  }

  function renderCurrent() {
    if (!currentDocument || !currentData) return;
	const session = createRenderSession(currentDocument.metadata && currentDocument.metadata.name);
	const nextRoot = renderNode(currentDocument.screen.root, currentData, rootRenderContext(session));
	const nextSignature = session.screenName + ':' + currentPath;
	const previousRoot = root.childNodes.length === 1 ? root.firstChild : null;
	const reconcile = committedRenderSignature === nextSignature && domReconciler.compatible(previousRoot, nextRoot);
	const viewState = reconcile ? null : window.ciwiCaptureViewState(root);
	if (reconcile) domReconciler.reconcile(previousRoot, nextRoot);
	else {
	  domReconciler.dispose(root);
	  root.replaceChildren(nextRoot);
	}
	committedActionBindings = session.actionBindings;
	committedRenderSignature = nextSignature;
	if (viewState) window.ciwiRestoreViewState(root, viewState);
	if (currentData.jobDetails) bindJobOutputScrollIntent(currentData.jobDetails);
	requestAnimationFrame(updateDeclarativeOutputCollapseButtons);
  }

  async function navigateBrowser(path, options = {}) {
	const targetPath = String(path || '/');
	const previousPath = currentPath || routePath();
	const previousState = window.history.state;
	if (!options.fromHistory) {
	  const nextState = browserHistoryState(targetPath, options.replace ? '' : previousPath);
	  if (options.replace) window.history.replaceState(nextState, '', targetPath);
	  else window.history.pushState(nextState, '', targetPath);
	}
	try {
	  await refresh({throwOnError: true, showLoading: true});
	  if (options.section) {
		const target = document.getElementById(String(options.section));
		if (target) target.scrollIntoView({block: 'start'});
	  }
	} catch (error) {
	  window.history.replaceState(previousState || browserHistoryState(previousPath, ''), '', previousPath);
	  throw error;
	}
  }

  window.ciwiNavigate = navigateBrowser;

  function declarativeVersionOptions(versions, emptyLabel) {
	const values = (Array.isArray(versions) ? versions : []).map(value => String(value || '').trim()).filter(Boolean);
	return values.length ? values.map(value => ({value, label: value})) : [{value: '', label: emptyLabel}];
  }

  function compareDeclarativeVersions(candidate, current) {
	const parse = value => {
	  const match = String(value || '').trim().replace(/^v/, '').match(/^(\d+)\.(\d+)\.(\d+)/);
	  return match ? [Number(match[1]), Number(match[2]), Number(match[3])] : null;
	};
	const left = parse(candidate);
	const right = parse(current);
	if (!left || !right) return null;
	for (let index = 0; index < 3; index += 1) {
	  if (left[index] !== right[index]) return left[index] < right[index] ? -1 : 1;
	}
	return 0;
  }

  function isDeclarativeLowerVersion(candidate, current) {
	return compareDeclarativeVersions(candidate, current) === -1;
  }

  function logViewKey(jobID, itemID) {
	return String(jobID || '') + '\n' + String(itemID || '');
  }

  function logViewState(jobID, itemID, view) {
	const key = logViewKey(jobID, itemID);
	let state = logViewStates.get(key);
	if (!state) {
	  state = {
		key, jobID: String(jobID || ''), itemID: String(itemID || ''), view,
		chunks: [], bytes: 0, hasBefore: false, hasAfter: false, terminal: false,
		loaded: false, loading: false, touched: Date.now(), elements: new Set(), match: null,
	  };
	  logViewStates.set(key, state);
	}
	state.view = view;
	state.touched = Date.now();
	return state;
  }

  function renderLogText(pre, state) {
	const text = state.chunks.map(chunk => String(chunk.text || '')).join('');
	const match = state.match;
	if (!match) {
	  pre.textContent = text || (state.loaded ? '(no output)' : 'Loading output…');
	  return;
	}
	const anchorIndex = state.chunks.findIndex(chunk => Number(chunk.id) === Number(match.chunk_id));
	if (anchorIndex < 0) {
	  pre.textContent = text;
	  return;
	}
	let start = Number(match.start_rune || 0);
	for (let index = 0; index < anchorIndex; index += 1) {
	  start += Array.from(String(state.chunks[index].text || '')).length;
	}
	const runes = Array.from(text);
	const length = Math.max(0, Number(match.end_rune || 0) - Number(match.start_rune || 0));
	if (start < 0 || start + length > runes.length || length === 0) {
	  pre.textContent = text;
	  return;
	}
	pre.appendChild(document.createTextNode(runes.slice(0, start).join('')));
	const mark = document.createElement('mark');
	mark.className = 'ciwi-search-hit ciwi-search-hit-active';
	mark.textContent = runes.slice(start, start + length).join('');
	pre.appendChild(mark);
	pre.appendChild(document.createTextNode(runes.slice(start + length).join('')));
  }

  function paintLogViewState(state, preserve) {
	state.elements.forEach(element => {
	  if (!element.isConnected) {
		state.elements.delete(element);
		return;
	  }
	  const oldHeight = element.scrollHeight;
	  const oldTop = element.scrollTop;
	  element.textContent = '';
	  const pre = document.createElement('pre');
	  pre.className = 'dsl-log-view-text';
	  renderLogText(pre, state);
	  element.appendChild(pre);
	  if (preserve === 'before') element.scrollTop = oldTop + Math.max(0, element.scrollHeight - oldHeight);
	  else if (preserve === 'tail') element.scrollTop = element.scrollHeight;
	  if (state.match) requestAnimationFrame(() => {
		const mark = element.querySelector('.ciwi-search-hit-active');
		if (mark) mark.scrollIntoView({block: 'center', inline: 'nearest'});
	  });
	});
  }

  function trimLogViewCache() {
	let total = 0;
	logViewStates.forEach(state => { total += state.bytes; });
	if (total <= maxLogViewCacheBytes) return;
	const states = Array.from(logViewStates.values()).sort((left, right) => left.touched - right.touched);
	for (const state of states) {
	  let changed = false;
	  while (total > maxLogViewCacheBytes && state.chunks.length > 1) {
		const matchIndex = state.match ? state.chunks.findIndex(chunk => Number(chunk.id) === Number(state.match.chunk_id)) : -1;
		const removeFromEnd = (matchIndex >= 0 ? matchIndex < state.chunks.length / 2 : false) || state.lastMode === 'before' || state.lastMode === 'head';
		const chunk = removeFromEnd ? state.chunks.pop() : state.chunks.shift();
		state.bytes -= Number(chunk.byte_count || new TextEncoder().encode(String(chunk.text || '')).length);
		total -= Number(chunk.byte_count || new TextEncoder().encode(String(chunk.text || '')).length);
		if (removeFromEnd) state.hasAfter = true;
		else state.hasBefore = true;
		changed = true;
	  }
	  if (changed) paintLogViewState(state, 'before');
	  if (total <= maxLogViewCacheBytes) break;
	}
  }

  async function loadLogViewPage(state, mode, cursor) {
	if (!state) return;
	if (state.loading) {
	  state.pendingLoad = {mode, cursor};
	  return;
	}
	state.loading = true;
	state.touched = Date.now();
	try {
	  const query = new URLSearchParams({item_id: state.itemID, mode});
	  if (cursor) query.set('cursor', String(cursor));
	  const response = await fetch('/api/v1/views/jobs/' + encodeURIComponent(state.jobID) + '/log/page?' + query.toString());
	  if (!response.ok) throw new Error(await response.text());
	  const page = await response.json();
	  const byID = new Map(state.chunks.map(chunk => [Number(chunk.id), chunk]));
	  (Array.isArray(page.chunks) ? page.chunks : []).forEach(chunk => byID.set(Number(chunk.id), chunk));
	  state.chunks = Array.from(byID.values()).sort((left, right) => Number(left.id) - Number(right.id));
	  state.lastMode = mode;
	  state.bytes = state.chunks.reduce((sum, chunk) => sum + Number(chunk.byte_count || new TextEncoder().encode(String(chunk.text || '')).length), 0);
	  state.hasBefore = !!page.has_before;
	  state.hasAfter = !!page.has_after;
	  state.terminal = !!page.terminal;
	  state.loaded = true;
	  const preserve = mode === 'before' ? 'before' : ((mode === 'tail' || (mode === 'after' && state.view && state.view.output_tailing)) ? 'tail' : 'none');
	  paintLogViewState(state, preserve);
	  trimLogViewCache();
	} catch (error) {
	  state.loaded = true;
	  state.elements.forEach(element => {
		element.textContent = 'Unable to load output: ' + (error.message || String(error));
	  });
	} finally {
	  state.loading = false;
	  const pending = state.pendingLoad;
	  state.pendingLoad = null;
	  if (pending) loadLogViewPage(state, pending.mode, pending.cursor);
	}
  }

  function renderBrowserLogView(element, logView, data) {
	const jobID = String(resolve(data, logView.jobExecutionId) || '');
	const itemID = logView.itemId ? String(resolve(data, logView.itemId) || '') : '';
	const view = data.jobDetails;
	const state = logViewState(jobID, itemID, view);
	element.dataset.logKey = state.key;
	element.tabIndex = 0;
	state.elements.add(element);
	paintLogViewState(state, 'none');
	element.addEventListener('scroll', () => {
	  state.touched = Date.now();
	  if (element.scrollTop < 96 && state.hasBefore && state.chunks.length) {
		loadLogViewPage(state, 'before', Number(state.chunks[0].id));
	  }
	  if (element.scrollHeight - element.clientHeight - element.scrollTop < 96 && state.hasAfter && state.chunks.length) {
		loadLogViewPage(state, 'after', Number(state.chunks[state.chunks.length - 1].id));
	  }
	  if (view && view.output_tailing && element.scrollHeight - element.clientHeight - element.scrollTop > 3) {
		setOutputTailing(view, false);
	  }
	}, {passive: true});
	if (!state.loaded && !state.loading) {
	  loadLogViewPage(state, jobOutputStartsAtTail(view) ? 'tail' : 'head', 0);
	}
  }

  function declarativePersistedUpdateBinding(status) {
	const source = status && typeof status === 'object' ? status : {};
	const current = String(source.update_current_version || '').trim();
	const latest = String(source.update_latest_version || '').trim();
	const message = String(source.update_message || '').trim();
	const checked = String(source.update_last_checked_utc || '').trim() !== '';
	const available = String(source.update_available || '') === '1' && compareDeclarativeVersions(latest, current) === 1;
	const upToDate = checked && message.toLowerCase() === 'already up to date';
	const versions = available ? [latest] : [];
	return {
	  updateVersions: declarativeVersionOptions(versions, upToDate ? 'No newer versions available' : 'Click "Check for updates"'),
	  selectedUpdateVersion: available ? latest : '',
	  updateResult: available ? 'Update available: ' + current + ' → ' + latest : (upToDate ? 'Up to date (' + current + ')' : ''),
	};
  }

  function outputMatchRanges(output, query) {
    if (!query) return [];
    const source = String(output || '').toLocaleLowerCase();
    const needle = String(query).toLocaleLowerCase();
    const matches = [];
    for (let offset = 0; offset <= source.length - needle.length;) {
      const index = source.indexOf(needle, offset);
      if (index < 0) break;
      matches.push([index, index + needle.length]);
      offset = index + needle.length;
    }
    return matches;
  }

  function jobOutputSources(view) {
    const sources = [{itemID: '', text: String(view.system_output || '')}];
    (Array.isArray(view.output_groups) ? view.output_groups : []).forEach(group => {
      sources.push({itemID: String(group.id || ''), text: String(group.output || '')});
    });
    return sources;
  }

  function groupedOutputMatches(view) {
    const matches = [];
    jobOutputSources(view).forEach(source => {
      outputMatchRanges(source.text, view.output_search).forEach(range => {
        matches.push({itemID: source.itemID, start: range[0], end: range[1]});
      });
    });
    return matches;
  }

  function updateOutputSearch(view, direction) {
    const matches = groupedOutputMatches(view);
    if (!matches.length) {
      view.output_match_index = 0;
      view.output_search_count = '0/0';
      return;
    }
    const current = Number(view.output_match_index || 0);
    view.output_match_index = direction > 0
      ? (current + 1) % matches.length
      : (direction < 0 ? (current - 1 + matches.length) % matches.length : Math.min(current, matches.length - 1));
    view.output_search_count = String(view.output_match_index + 1) + '/' + String(matches.length);
  }

  function browserOutputGroup(view, itemID) {
    return (Array.isArray(view.output_groups) ? view.output_groups : []).find(group => String(group.id || '') === String(itemID || ''));
  }

  function revealBrowserOutputGroup(view, itemID) {
    const group = browserOutputGroup(view, itemID);
    if (!group) return null;
    const target = document.querySelector('[data-disclosure-key="' + CSS.escape(String(group.state_key || '')) + '"]');
    if (!target) return null;
    target.open = true;
    disclosureStates.set(String(group.state_key || ''), true);
    target.scrollIntoView({block: 'nearest'});
    return target;
  }

  function selectBrowserOutputMatch(view) {
    const matches = groupedOutputMatches(view);
    const match = matches[Number(view.output_match_index || 0)];
    if (!match) return;
    const disclosure = match.itemID ? revealBrowserOutputGroup(view, match.itemID) : null;
    const target = disclosure || document.getElementById('job-output-system');
    const active = target && target.querySelector('.ciwi-search-hit-active');
    if (active) active.scrollIntoView({block: 'center', inline: 'nearest', behavior: 'smooth'});
  }

  async function updateFullLogSearch(view, selectedIndex) {
	const query = String(view.output_search || '');
	const generation = ++fullLogSearchGeneration;
	if (Array.from(query).length < 3) {
	  view.output_match_index = 0;
	  view.output_total_matches = 0;
	  view.output_search_count = query ? 'Enter 3+ characters' : '0/0';
	  updateJobOutputSearchCount(view);
	  return;
	}
	const response = await fetch('/api/v1/views/jobs/' + encodeURIComponent(view.id) + '/log/search', {
	  method: 'POST', headers: {'Content-Type': 'application/json'},
	  body: JSON.stringify({query, selected_index: Math.max(0, Number(selectedIndex || 0))}),
	});
	if (!response.ok) throw new Error(await response.text());
	const result = await response.json();
	if (generation !== fullLogSearchGeneration || query !== String(view.output_search || '')) return;
	view.output_match_index = Number(result.selected_index || 0);
	view.output_total_matches = Number(result.total_matches || 0);
	view.output_search_count = view.output_total_matches
	  ? String(view.output_match_index + 1) + '/' + String(view.output_total_matches)
	  : '0/0';
	updateJobOutputSearchCount(view);
	if (!result.match) return;
	if (result.match.item_id) revealBrowserOutputGroup(view, result.match.item_id);
	const state = logViewState(view.id, result.match.item_id || '', view);
	state.match = result.match;
	await loadLogViewPage(state, 'around', Number(result.match.chunk_id));
	requestAnimationFrame(() => {
	  state.elements.forEach(element => {
		const mark = element.querySelector('.ciwi-search-hit-active');
		if (mark) mark.scrollIntoView({block: 'center', inline: 'nearest', behavior: 'smooth'});
	  });
	});
  }

  function scheduleFullLogSearch(view) {
	window.clearTimeout(fullLogSearchTimer);
	fullLogSearchTimer = window.setTimeout(() => {
	  updateFullLogSearch(view, 0).catch(error => {
		view.output_search_count = error.message || String(error);
		updateJobOutputSearchCount(view);
	  });
	}, 250);
  }

  function findFullLogMatch(view, direction) {
	const total = Number(view.output_total_matches || 0);
	let target = Number(view.output_match_index || 0);
	if (total > 0) target = (target + (direction < 0 ? -1 : 1) + total) % total;
	return updateFullLogSearch(view, target);
  }

  function renderBrowserOutputText(element, text, itemID, view) {
    const ranges = outputMatchRanges(text, view.output_search);
    const active = groupedOutputMatches(view)[Number(view.output_match_index || 0)];
    let cursor = 0;
    ranges.forEach(range => {
      if (range[0] > cursor) element.appendChild(document.createTextNode(text.slice(cursor, range[0])));
      const mark = document.createElement('mark');
      mark.className = 'ciwi-search-hit';
      if (active && String(active.itemID) === String(itemID) && active.start === range[0] && active.end === range[1]) {
        mark.classList.add('ciwi-search-hit-active');
      }
      mark.textContent = text.slice(range[0], range[1]);
      element.appendChild(mark);
      cursor = range[1];
    });
    if (cursor < text.length) element.appendChild(document.createTextNode(text.slice(cursor)));
  }

  function updateDeclarativeOutputCollapseButtons() {
    const container = document.getElementById('job-output-groups');
    if (!container) return;
    container.querySelectorAll('details.dsl-output-group').forEach(details => {
      const button = details.querySelector(':scope > .dsl-floating-collapse');
      if (!button) return;
      button.hidden = !details.open || details.scrollHeight <= container.clientHeight;
    });
  }

  function outputIsAtBottom(element) {
	return !element || element.scrollHeight - element.clientHeight - element.scrollTop <= 3;
  }

  function jobOutputStartsAtTail(view) {
	return ['queued', 'leased', 'running', 'waiting', 'in progress', 'active']
	  .includes(String(view && view.status || '').trim().toLowerCase());
  }

  function setOutputTailing(view, enabled) {
	view.output_tailing = !!enabled;
	view.tailing_label = view.output_tailing ? 'Tailing: On' : 'Tailing: Off';
	view.tailing_tone = view.output_tailing ? 'success' : 'warning';
	const button = document.getElementById('job-output-tailing-toggle');
	if (button) {
	  button.classList.toggle('dsl-success', view.output_tailing);
	  button.classList.toggle('dsl-warning', !view.output_tailing);
	  const label = button.querySelector('.dsl-button-label');
	  if (label) label.textContent = view.tailing_label;
	}
  }

  function bindJobOutputScrollIntent(view) {
	const container = document.getElementById('job-output-groups');
	if (!container) return;
	container.__ciwiOutputView = view;
	if (container.dataset.ciwiScrollIntent === '1') return;
	container.dataset.ciwiScrollIntent = '1';
	container.addEventListener('scroll', () => {
	  const currentView = container.__ciwiOutputView;
	  if (!currentView || programmaticOutputScroll || !currentView.output_tailing || outputIsAtBottom(container)) return;
	  setOutputTailing(currentView, false);
	}, {passive: true});
  }

  function scrollJobOutputToEnd(element) {
	if (!element) return;
	programmaticOutputScroll = true;
	element.scrollTop = element.scrollHeight;
	requestAnimationFrame(() => { programmaticOutputScroll = false; });
  }

  function updateJobOutputSearchCount(view) {
	const count = document.getElementById('job-output-search-count');
	if (count) count.textContent = String(view.output_search_count || '0/0');
  }

  function patchJobOutputRegion(view) {
	renderCurrent();
	const currentScroller = document.getElementById('job-output-groups');
	bindJobOutputScrollIntent(view);
	if (view.output_tailing) scrollJobOutputToEnd(currentScroller);
	updateJobOutputSearchCount(view);
	requestAnimationFrame(updateDeclarativeOutputCollapseButtons);
  }

  function initializeJobOutputView(view, previousView) {
	const previousGroups = new Map((Array.isArray(previousView && previousView.output_groups) ? previousView.output_groups : [])
	  .map(group => [String(group.id || ''), group]));
	const previousCursor = Number(previousView && previousView.output_after_event_id || 0);
	view.system_output = previousView ? String(previousView.system_output || '') : '';
	view.output_after_event_id = Number.isFinite(previousCursor) && previousCursor >= 0 ? previousCursor : 0;
    (Array.isArray(view.output_groups) ? view.output_groups : []).forEach(group => {
      const previousGroup = previousGroups.get(String(group.id || ''));
      group.output = previousGroup ? String(previousGroup.output || '') : '';
      group.empty_output_label = group.output ? '' : (group.reached ? '(no output)' : '(step was not reached)');
      group.yaml_literal = group.yaml_literal || '(none)';
      group.expanded_command = group.expanded_command || '(none)';
      group.details = group.details || '(none)';
    });
    rebuildJobOutputText(view);
  }

  function appendBoundedOutput(view, itemID, text) {
	if (!text) return false;
    const group = itemID ? browserOutputGroup(view, itemID) : null;
    const fieldOwner = group || view;
    const field = group ? 'output' : 'system_output';
    let output = String(fieldOwner[field] || '') + String(text);
    if (output.length > maxOutputCharacters) {
      output = '[ciwi: earlier output omitted]\n' + output.slice(output.length - maxOutputCharacters);
    }
    fieldOwner[field] = output;
	return true;
  }

  function rebuildJobOutputText(view) {
    const sections = [];
    if (view.system_output) sections.push('System messages\n' + view.system_output);
    (Array.isArray(view.output_groups) ? view.output_groups : []).forEach(group => {
      const body = String(group.output || '') || String(group.empty_output_label || '');
      sections.push(String(group.title || 'Execution item') + '\n' + body);
    });
    view.output = sections.join('\n\n');
  }

  function mergeJobOutputBatch(view, batch) {
	let changed = false;
    (Array.isArray(batch.events) ? batch.events : []).forEach(event => {
      const itemID = String(event.item_id || '');
      if (event.type === 'system-message' || event.type === 'output') {
		changed = appendBoundedOutput(view, event.type === 'system-message' ? '' : itemID, event.text || '') || changed;
      }
      if (event.type === 'finished') {
        const group = browserOutputGroup(view, itemID);
        if (group) {
		  changed = true;
          group.reached = true;
          group.status = event.error ? 'failed' : 'succeeded';
          group.status_label = event.error ? 'Failed' : 'Succeeded';
          group.error = event.error || '';
          group.exit_code = event.exit_code || '';
          group.empty_output_label = group.output ? '' : '(no output)';
        }
      }
    });
	if (!changed) return false;
    rebuildJobOutputText(view);
    updateOutputSearch(view, 0);
	return true;
  }

  function stopJobOutputWatch() {
	if (outputEventSource) outputEventSource.close();
	outputEventSource = null;
  }

  function setOutputWatchGeneration(generation) {
	stopJobOutputWatch();
	outputWatchGeneration = generation;
  }

  function watchJobOutput(jobID, generation) {
	const activeJob = currentData && currentData.jobDetails;
	let afterEventID = activeJob && String(activeJob.id || '') === String(jobID)
	  ? Number(activeJob.output_after_event_id || 0) : 0;
	if (!Number.isFinite(afterEventID) || afterEventID < 0) afterEventID = 0;
	if (typeof window.EventSource !== 'function') throw new Error('Live output requires EventSource support');
	stopJobOutputWatch();
	const source = new EventSource('/api/v1/views/jobs/' + encodeURIComponent(jobID) + '/output/stream?after_event_id=' + String(afterEventID));
	outputEventSource = source;
	const currentJob = () => {
	  if (generation !== outputWatchGeneration) return null;
	  const view = currentData && currentData.jobDetails;
	  return view && String(view.id || '') === String(jobID) ? view : null;
	};
	source.addEventListener('output', event => {
	  const view = currentJob();
	  if (!view) { source.close(); return; }
	  try {
		const batch = JSON.parse(event.data || '{}');
		const nextEventID = Number(batch.next_event_id || event.lastEventId || afterEventID);
		if (Number.isFinite(nextEventID) && nextEventID >= afterEventID) {
		  afterEventID = nextEventID;
		  view.output_after_event_id = nextEventID;
		}
		if (mergeJobOutputBatch(view, batch)) patchJobOutputRegion(view);
	  } catch (error) {
		console.error('Invalid job output event', error);
	  }
	});
	source.addEventListener('complete', () => {
	  if (currentJob()) {
		completedOutputJobID = String(jobID);
		scheduleChangeRefresh();
	  }
	  source.close();
	  if (outputEventSource === source) outputEventSource = null;
	});
	source.addEventListener('stream-error', event => {
	  const view = currentJob();
	  if (!view) return;
	  let message = 'Output stream failed';
	  try { message = JSON.parse(event.data || '{}').message || message; } catch (_) {}
	  appendBoundedOutput(view, '', message + '\n');
	  rebuildJobOutputText(view);
	  patchJobOutputRegion(view);
	  source.close();
	  if (outputEventSource === source) outputEventSource = null;
	});
  }

  function watchFullJobLog(jobID, generation) {
	if (typeof window.EventSource !== 'function') throw new Error('Live output requires EventSource support');
	stopJobOutputWatch();
	const source = new EventSource('/api/v1/views/jobs/' + encodeURIComponent(jobID) + '/log/stream');
	outputEventSource = source;
	const currentJob = () => {
	  if (generation !== outputWatchGeneration) return null;
	  const view = currentData && currentData.jobDetails;
	  return view && String(view.id || '') === String(jobID) ? view : null;
	};
	source.addEventListener('change', event => {
	  const view = currentJob();
	  if (!view) { source.close(); return; }
	  try {
		const descriptor = JSON.parse(event.data || '{}');
		const streams = new Map((Array.isArray(descriptor.streams) ? descriptor.streams : [])
		  .map(stream => [String(stream.item_id || ''), stream]));
		logViewStates.forEach(state => {
		  if (state.jobID !== String(jobID)) return;
		  const stream = streams.get(state.itemID);
		  if (!stream) return;
		  state.terminal = !!descriptor.terminal;
		  const last = state.chunks.length ? Number(state.chunks[state.chunks.length - 1].id) : 0;
		  if (Number(stream.last_chunk_id || 0) <= last) return;
		  if (!view.output_tailing) {
			state.hasAfter = true;
			return;
		  }
		  loadLogViewPage(state, last ? 'after' : 'tail', last);
		});
	  } catch (error) {
		console.error('Invalid full job log event', error);
	  }
	});
	source.addEventListener('complete', () => {
	  if (currentJob()) {
		completedOutputJobID = String(jobID);
		scheduleChangeRefresh();
	  }
	  source.close();
	  if (outputEventSource === source) outputEventSource = null;
	});
	source.addEventListener('stream-error', event => {
	  let message = 'Output stream failed';
	  try { message = JSON.parse(event.data || '{}').message || message; } catch (_) {}
	  logViewStates.forEach(state => {
		if (state.jobID !== String(jobID)) return;
		state.elements.forEach(element => { element.dataset.logError = message; });
	  });
	  source.close();
	  if (outputEventSource === source) outputEventSource = null;
	});
  }

  function activeWatchTopics() {
	const topics = new Set();
	const sources = currentDocument && currentDocument.screen && currentDocument.screen.dataSources;
	(Array.isArray(sources) ? sources : []).forEach(source => {
	  (Array.isArray(source.watchTopics) ? source.watchTopics : []).forEach(topic => topics.add(String(topic)));
	});
	return topics;
  }

  function scheduleChangeRefresh() {
	if (changeRefreshScheduler) changeRefreshScheduler.schedule();
  }

  function pendingActionRemovesCurrentRoute() {
	if (!currentRouteMatch || currentRouteMatch.route.name !== 'agent-details' || typeof window.ciwiActiveOperations !== 'function') return false;
	const agentID = String(currentRouteMatch.params.agentId || '');
	return window.ciwiActiveOperations().some(operation => operation.command === 'agent-action' &&
	  String(operation.arguments && operation.arguments.action || '') === 'delete' &&
	  String(operation.arguments && operation.arguments.agentId || '') === agentID &&
	  String(operation.arguments && operation.arguments.successRoute || '') !== '');
  }

  function startChangeWatch() {
	if (typeof window.EventSource !== 'function') return;
	const source = new EventSource('/api/v1/ui/changes');
	source.onmessage = event => {
	  try {
		const change = JSON.parse(event.data || '{}');
		const watched = activeWatchTopics();
		if (change.resync_required) {
		  scheduleChangeRefresh();
		  return;
		}
		const topics = (change.topics || []).map(String);
		if (currentRouteMatch && currentRouteMatch.route.name === 'job-details') {
		  const view = currentData && currentData.jobDetails;
		  const jobID = String(view && view.id || currentRouteMatch.params.jobId || '');
		  const changedIDs = (change.job_execution_ids || []).map(String);
		  if (changedIDs.length && !changedIDs.includes(jobID)) return;
		  if (topics.includes('job-output')) return;
		  if (!changedIDs.length && topics.some(topic => topic === 'queue' || topic === 'history')) return;
		  if (topics.includes('agent-eligibility') && String(view && view.status || '') !== 'queued') return;
		}
		if (topics.some(topic => watched.has(topic)) && !pendingActionRemovesCurrentRoute()) scheduleChangeRefresh();
	  } catch (_) {}
	};
  }

  async function refresh(options = {}) {
    const scheduler = changeRefreshScheduler;
    if (scheduler) scheduler.beginRefresh();
    try {
    const loadGeneration = ++routeLoadGeneration;
    const generation = outputWatchGeneration + 1;
	let loadingCommitted = false;
    try {
	  const nextRouteMatch = await resolveBrowserRoute();
	  const routeName = nextRouteMatch.route.name;
	  const screenName = nextRouteMatch.route.screen;
	  const bindingRoot = nextRouteMatch.route.bindingRoot;
	  const projectMatch = routeName === 'project-details';
	  const jobMatch = routeName === 'job-details';
	  const settingsMatch = routeName === 'settings';
	  const agentDetailsMatch = routeName === 'agent-details';
	  const agentsMatch = routeName === 'agents';
	  const connectionMatch = routeName === 'connection';
	  const managedYAMLMatch = routeName === 'managed-yaml' || routeName === 'managed-yaml-new';
	  const agentScriptMatch = routeName === 'agent-script';
	  const vaultMatch = routeName === 'vault';
	  const runOptionsMatch = routeName === 'pipeline-run-options' || routeName === 'legacy-pipeline-run-options' || routeName === 'chain-run-options';
	  let viewURL = '/api/v1/views/front-page';
	  if (projectMatch) viewURL = '/api/v1/views/projects/' + encodeURIComponent(nextRouteMatch.params.projectId);
	  if (jobMatch) viewURL = '/api/v1/views/jobs/' + encodeURIComponent(nextRouteMatch.params.jobId);
	  if (settingsMatch) viewURL = '/api/v1/server-info';
	  if (agentDetailsMatch) viewURL = '/api/v1/views/agents/' + encodeURIComponent(nextRouteMatch.params.agentId);
	  if (agentsMatch) viewURL = '/api/v1/views/agents';
	  if (managedYAMLMatch && routeName === 'managed-yaml') viewURL = '/api/v1/projects/' + encodeURIComponent(nextRouteMatch.params.projectId) + '/managed-yaml';
	  if (managedYAMLMatch && routeName === 'managed-yaml-new') viewURL = '';
	  if (agentScriptMatch) viewURL = '/api/v1/views/agents/' + encodeURIComponent(nextRouteMatch.params.agentId);
	  if (vaultMatch) viewURL = '/api/v1/vault/connections';
	  if (runOptionsMatch) viewURL = runOptionsViewURL('', '', nextRouteMatch);
	  const viewPromise = viewURL
		? fetch(viewURL)
		: Promise.resolve(new Response('{}', {status: 200, headers: {'Content-Type': 'application/json'}}));
	  const settingsProjectsPromise = settingsMatch ? fetch('/api/v1/projects') : null;
	  const settingsUpdateStatusPromise = settingsMatch ? fetch('/api/v1/update/status') : null;
	  const documentPromise = screenContract(screenName);
	  const themesPromise = themeContracts();
	  const controlsPromise = controlsContract();
	  const cacheKey = routePath();
	  const cachedView = browserViewCache.get(cacheKey);
	  if (cachedView) {
		cachedView.loading = false;
		cachedView.ready = true;
		cachedView.load_error = '';
	  }
	  const loadingView = options.showLoading ? (cachedView || viewBindings.browserLoadingBinding(nextRouteMatch)) : null;
	  if (loadingView) {
		const [loadingDocument, loadingThemes, loadingControls] = await Promise.all([documentPromise, themesPromise, controlsPromise]);
		if (loadGeneration !== routeLoadGeneration) return false;
		applyContractTheme(loadingThemes);
		applyControlsContract(loadingControls);
		setOutputWatchGeneration(generation);
		currentRouteMatch = nextRouteMatch;
		currentPath = routePath();
		currentDocument = loadingDocument;
		currentData = {[bindingRoot]: loadingView, client: viewBindings.browserClientBinding()};
		renderCurrent();
		loadingCommitted = true;
	  }
	  const [documentContract, themes, controls, viewResponse] = await Promise.all([documentPromise, themesPromise, controlsPromise, viewPromise]);
	  if (!viewResponse.ok) throw new Error(await viewResponse.text());
	  const responseView = await viewResponse.json();
      applyContractTheme(themes);
	  applyControlsContract(controls);
      let view = responseView;
	  if (managedYAMLMatch) view = viewBindings.managedYAMLBinding(responseView);
	  if (agentScriptMatch) view = viewBindings.agentScriptBinding(responseView, nextRouteMatch.params.agentId);
	  if (vaultMatch) view = viewBindings.vaultBinding(responseView);
	  if (projectMatch) {
		viewBindings.decorateProjectDetails(view);
	  }
	  if (routeName === 'front-page') {
		viewBindings.decorateFrontPageProjects(view.projects);
      }
      if (settingsMatch) {
        const selectedTheme = ciwiStoredTheme();
        const themeOptions = themes.map(theme => ({
          name: theme.metadata.name,
          title: theme.metadata.title || theme.metadata.name,
          description: theme.metadata.description || '',
        }));
        const selected = themeOptions.find(theme => theme.name === selectedTheme);
		const projectsResponse = await settingsProjectsPromise;
		if (!projectsResponse.ok) throw new Error(await projectsResponse.text());
		const updateStatusResponse = await settingsUpdateStatusPromise;
		if (!updateStatusResponse.ok) throw new Error(await updateStatusResponse.text());
		const projectsPayload = await projectsResponse.json();
		const updateStatusPayload = await updateStatusResponse.json();
		const updateStatus = updateStatusPayload.status || {};
		const persistedUpdate = declarativePersistedUpdateBinding(updateStatus);
		const projects = Array.isArray(projectsPayload.projects) ? projectsPayload.projects : [];
		projects.forEach(project => {
		  project.action_status = '';
		  project.action_tone = 'muted';
		  const updatedUTC = String(project.updated_utc || '').trim();
		  const updatedMilliseconds = Number(project.updated_unix_ms || 0);
		  const updatedLabel = updatedUTC
			? viewBindings.declarativeExecutionTimestamp(updatedUTC)
			: (updatedMilliseconds > 0 ? viewBindings.declarativeExecutionTimestamp(new Date(updatedMilliseconds).toISOString()) : '');
		  project.updated_label = updatedLabel || 'Unknown';
		});
		view = {
		  server: responseView, themes: themeOptions, projects,
		  client_version: String(responseView.version || ''), server_version: String(responseView.version || ''), server_connected: true,
		  selected_theme: selectedTheme, selected_theme_description: selected ? selected.description : '',
		  import_repo_url: '', import_repo_ref: '', import_config_file: 'ciwi-project.yaml',
		  update_supported: String(updateStatus.update_server_self_update_supported || '') === '1',
		  update_capability_notice: updateStatus.update_server_mode === 'dev' ? 'Running in dev mode. Updates disabled.' : (updateStatus.update_server_self_update_reason || ''),
		  update_status_label: ['Current: ' + (updateStatus.update_current_version || ''), updateStatus.update_message ? 'Message: ' + updateStatus.update_message : ''].filter(Boolean).join(' · '),
		  blocked_agent_notice: updateStatus.update_agent_non_service_agents ? 'Agents requiring manual update: ' + updateStatus.update_agent_non_service_agents : '',
		  update_versions: persistedUpdate.updateVersions, selected_update_version: persistedUpdate.selectedUpdateVersion,
		  rollback_versions: declarativeVersionOptions([], 'Refresh versions'), selected_rollback_version: '',
		  update_result: persistedUpdate.updateResult, update_result_tone: persistedUpdate.updateResult ? 'success' : 'muted',
		  rollback_result: '', rollback_result_tone: 'muted',
		  connection_mode: 'discover', connection_endpoint: '', connection_explicit: false,
		  connection_modes: [{value: 'discover', label: 'Automatic discovery'}, {value: 'explicit', label: 'Explicit endpoint'}],
		};
      }
	  if (connectionMatch) {
		view = {
		  mode: 'discover', endpoint: '', explicit: false, can_back: true,
		  status: 'Native connection state is local to the desktop client.', status_tone: 'muted',
		  modes: [{value: 'discover', label: 'Automatic discovery'}, {value: 'explicit', label: 'Explicit endpoint'}],
		};
	  }
      if (jobMatch) {
		const previousJob = currentData && currentData.jobDetails;
		const sameJob = previousJob && String(previousJob.id || '') === String(view.id || '');
		if (!sameJob) completedOutputJobID = '';
		viewBindings.decorateJobDetails(view);
		view.output_search = sameJob ? String(previousJob.output_search || '') : '';
		view.output_match_index = sameJob ? Number(previousJob.output_match_index || 0) : 0;
		initializeJobOutputView(view, sameJob ? previousJob : null);
		updateOutputSearch(view, 0);
		view.output_tailing = sameJob ? !!previousJob.output_tailing : jobOutputStartsAtTail(view);
		view.tailing_label = view.output_tailing ? 'Tailing: On' : 'Tailing: Off';
		view.tailing_tone = view.output_tailing ? 'success' : 'warning';
		const timeline = Array.isArray(view.timeline) ? view.timeline : [];
		const previousSelectionID = sameJob && previousJob.selected_timeline_item
		  ? String(previousJob.selected_timeline_item.id || '') : '';
		view.selected_timeline_item = timeline.find(item => String(item.id || '') === previousSelectionID)
		  || timeline.find(item => ['running', 'in progress', 'failed'].includes(String(item.status || '').toLowerCase()))
		  || timeline[0]
		  || {id:'', title:'No execution steps reported', description:'', status:'', status_label:'', duration:'', exit_code:'', error:''};
      }
	  if (!jobMatch) completedOutputJobID = '';
	  if (loadGeneration !== routeLoadGeneration) return false;
	  viewBindings.markBrowserViewReady(view);
	  setOutputWatchGeneration(generation);
	  currentRouteMatch = nextRouteMatch;
	  currentPath = routePath();
	  currentDocument = documentContract;
	  currentData = { [bindingRoot]: view, client: viewBindings.browserClientBinding() };
	  browserViewCache.set(cacheKey, view);
      renderCurrent();
      if (jobMatch) {
		const jobID = nextRouteMatch.params.jobId;
		if (completedOutputJobID !== String(jobID)) try {
		  if (view.interactive_log_available) watchFullJobLog(jobID, generation);
		  else watchJobOutput(jobID, generation);
		} catch (error) {
          if (generation !== outputWatchGeneration) return;
          if (currentData && currentData.jobDetails) {
			if (currentData.jobDetails.interactive_log_available) {
			  currentData.jobDetails.output_search_count = 'Output stream failed: ' + (error.message || String(error));
			  updateJobOutputSearchCount(currentData.jobDetails);
			} else {
			  appendBoundedOutput(currentData.jobDetails, '', 'Output stream failed: ' + (error.message || String(error)) + '\n');
			  rebuildJobOutputText(currentData.jobDetails);
			  patchJobOutputRegion(currentData.jobDetails);
			}
          }
		}
      }
    } catch (error) {
	  if (loadingCommitted && loadGeneration === routeLoadGeneration && currentData) {
		const rootName = currentRouteMatch && currentRouteMatch.route.bindingRoot;
		const loadingRoot = rootName && currentData[rootName];
		if (!loadingRoot) throw error;
		loadingRoot.loading = false;
		loadingRoot.load_error = error.message || String(error);
		renderCurrent();
		return false;
	  }
	  if (options.throwOnError) throw error;
	  if (currentDocument && currentData) {
		console.error(error);
		return false;
	  }
      const message = document.createElement('div');
	  message.className = 'dsl-error';
	  message.textContent = error.message || String(error);
	  committedRenderSignature = '';
	  committedActionBindings = new Map();
	  domReconciler.dispose(root);
      root.replaceChildren(message);
	  return false;
    }
	return true;
    } finally {
	  if (scheduler) scheduler.endRefresh();
	}
  }

  viewBindings = window.ciwiCreateBrowserViewBindings({getCurrentData: () => currentData});

  domReconciler = window.ciwiCreateDOMReconciler({selectControl: () => browserSelectControl});

  treeViewRenderer = window.ciwiCreateTreeViewRenderer({
    resolve,
    renderText,
    semanticTone,
    bindActions,
    rendererKeyPart,
    annotateRendererElement,
    disclosureStates,
  });

  graphViewRenderer = window.ciwiCreateGraphViewRenderer({
    resolve,
    renderText,
    icon: declarativeIcon,
    bindActions,
    renderNode,
    childRenderContext,
    rendererKeyPart,
    annotateRendererElement,
    disposeRenderedNode: node => domReconciler.dispose(node),
    graphRuntimeStates,
    viewStates,
    saveViewStates,
  });

  browserSelectControl = window.ciwiCreateBrowserSelectControl({
    controls: () => activeControls,
    appendPositionedIcon,
    icon: declarativeIcon,
  });

  window.addEventListener('popstate', () => {
	const pending = pendingHistoryNavigation;
	pendingHistoryNavigation = null;
	navigateBrowser(routePath(), {fromHistory: true}).then(() => {
	  if (pending) pending.resolve();
	}).catch(error => {
	  if (pending) pending.reject(error);
	  else window.alert(error.message || String(error));
	});
  });
  if (!window.history.state || window.history.state.ciwi !== true || !window.history.state.navigationID) {
	window.history.replaceState(browserHistoryState(routePath(), ''), '', routePath());
  } else {
	browserNavigationSequence = Math.max(browserNavigationSequence, Number(window.history.state.navigationID || 0));
  }
  changeRefreshScheduler = window.ciwiCreateChangeRefreshScheduler({refresh: () => refresh(), delay: 100});
  refresh({showLoading: true}).finally(startChangeWatch);
})();
