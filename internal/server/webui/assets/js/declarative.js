(() => {
  'use strict';

  const root = document.getElementById('declarativeRoot');
  let outputWatchGeneration = 0;
  let outputEventSource = null;
  let completedOutputJobID = '';
  let programmaticOutputScroll = false;
  let routeLoadGeneration = 0;
  const maxOutputCharacters = 1024 * 1024;
  let currentDocument = null;
  let currentData = null;
  let currentRouteMatch = null;
  let currentPath = '';
  let routeContractPromise = null;
  let themeContractPromise = null;
  let controlsContractPromise = null;
  let activeControls = null;
  const screenContractPromises = new Map();
  const browserViewCache = new Map();
  let screenContractsPreloaded = false;
  let changeRefreshScheduler = null;
  let activeBrowserSelect = null;
  const determinateProgressLimit = .999;
  const indeterminateProgressCycleMs = 4000;
  const overrunProgressCycleMs = 2000;
  const disclosureStates = window.ciwiDisclosureState;
  const viewStorageKey = 'ciwi.declarative.views.v1';
  const viewStates = loadViewStates();
  let committedActionBindings = new Map();
  let committedRenderSignature = '';

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
	metric('--ciwi-button-icon-size', activeControls.button.iconSize);
	metric('--ciwi-button-icon-gap', activeControls.button.iconGap);
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

  function declarativeExecutionTimestamp(value) {
	const parsed = new Date(String(value || ''));
	if (Number.isNaN(parsed.getTime())) return '';
	const parts = Object.fromEntries(new Intl.DateTimeFormat(undefined, {
	  weekday: 'short', day: '2-digit', month: 'short', hour: '2-digit', minute: '2-digit', second: '2-digit', hourCycle: 'h23',
	}).formatToParts(parsed).map(part => [part.type, part.value]));
	return [parts.weekday, parts.day, parts.month].filter(Boolean).join(' ') + ', ' +
	  [parts.hour, parts.minute, parts.second].filter(Boolean).join(':');
  }

  function decorateFrontPageProjects(projects) {
	(Array.isArray(projects) ? projects : []).forEach(project => {
	  project.project_icon = Number(project.id || 0) > 0 && String(project.source_kind || '') !== 'managed_yaml'
		? '/api/v1/projects/' + encodeURIComponent(project.id) + '/icon'
		: '';
	});
  }

  function decorateProjectDetails(view) {
	const project = (view && view.project) || {};
	const previousProject = currentData && currentData.projectDetails && currentData.projectDetails.project;
	const previousFilter = previousProject && String(previousProject.id) === String(project.id)
	  ? String(currentData.projectDetails.structure_filter || 'all-pipelines')
	  : 'all-pipelines';
	project.project_icon = Number(project.id || 0) > 0 ? '/api/v1/projects/' + encodeURIComponent(project.id) + '/icon' : '';
	view.loading = false;
	view.ready = true;
	view.load_error = '';
	applyProjectStructureFilter(view, previousFilter);
  }

  function projectDetailsLoadingBinding(projectID) {
	const id = String(projectID || '');
	const currentProject = currentData && currentData.projectDetails && currentData.projectDetails.project;
	const frontProjects = currentData && currentData.frontPage && Array.isArray(currentData.frontPage.projects)
	  ? currentData.frontPage.projects : [];
	const source = (currentProject && String(currentProject.id || '') === id
	  ? currentProject
	  : frontProjects.find(project => String(project.id || '') === id)) || {};
	const project = Object.assign({}, source, {
	  id: source.id || Number(projectID || 0),
	  name: String(source.name || 'Project'),
	  project_icon: String(source.project_icon || ''),
	  pipeline_chains: [], has_pipeline_chains: false,
	});
	return {
	  project, pipelines: [], visible_pipelines: [],
	  structure_filter: 'all-pipelines', structure_filters: [],
	  structure_root: {id: 'project:' + id + ':loading', label: project.name, meta: '', runnable: false, project_id: id, chain_id: ''},
	  show_chain_structure: false, show_pipeline_structure: false,
	  history_executions: [], history_empty: true,
	  loading: true, ready: false, load_error: '',
	};
  }

  function browserClientBinding() {
	return {
	  connected: true, connecting: false, offline: false, address: window.location.host,
	  status: 'Connected through the browser', tone: 'success', progress: {state: 'none'},
	};
  }

  function markBrowserViewReady(view) {
	if (!view || typeof view !== 'object') return view;
	view.loading = false;
	view.ready = true;
	view.load_error = '';
	return view;
  }

  function browserLoadingBinding(routeMatch) {
	const routeName = routeMatch.route.name;
	const params = routeMatch.params || {};
	if (routeName === 'project-details') return projectDetailsLoadingBinding(params.projectId);
	const loading = {loading: true, ready: false, load_error: ''};
	if (routeName === 'front-page') return Object.assign(loading, {
	  server: {version: ''}, projects: [], queued_executions: [], history_executions: [],
	  queued_empty: false, history_empty: false,
	});
	if (routeName === 'job-details') return Object.assign(loading, {
	  id: String(params.jobId || ''), title: 'Job execution', project_icon: '', progress: {state: 'none'},
	  status: '', status_label: '', current_step: '', can_rerun: false, can_cancel: false,
	});
	if (routeName === 'settings') return Object.assign(loading, {server_version: '', themes: [], projects: []});
	if (routeName === 'agents') return Object.assign(loading, {summary: '', agents: []});
	if (routeName === 'agent-details') return Object.assign(loading, {agent: {id: String(params.agentId || '')}});
	if (routeName === 'agent-script') return Object.assign(loading, {
	  agent_id: String(params.agentId || ''), agent_label: 'Agent', shells: [], selected_shell: '', script: '', can_run: false,
	});
	if (routeName === 'managed-yaml' || routeName === 'managed-yaml-new') return Object.assign(loading, {
	  title: routeName === 'managed-yaml-new' ? 'Add Managed YAML' : 'Managed YAML',
	  project_id: Number(params.projectId || 0), project_name: '', yaml: '', revision: '', editing: false,
	});
	if (routeName === 'vault') return Object.assign(loading, {connections: []});
	if (routeName.includes('run-options')) return Object.assign(loading, {
	  project_id: Number(params.projectId || 0), pipeline_db_id: Number(params.pipelineId || 0),
	  chain_id: String(params.chainId || ''), target_label: 'Run options', target_kind: 'loading',
	  source_refs: [], eligible_agents: [], selected_source_ref: '', selected_agent_id: '',
	});
	return loading;
  }

  function decorateJobDetails(view) {
	view.project_icon = Number(view.project_id || 0) > 0
	  ? '/api/v1/projects/' + encodeURIComponent(view.project_id) + '/icon'
	  : '';
  }

  function applyProjectStructureFilter(view, requestedFilter) {
	const pipelines = Array.isArray(view && view.pipelines) ? view.pipelines : [];
	const filters = Array.isArray(view && view.structure_filters) ? view.structure_filters : [];
	const requested = String(requestedFilter || 'all-pipelines').trim() || 'all-pipelines';
	const selected = filters.find(filter => String(filter.value || '') === requested)
	  || filters.find(filter => String(filter.value || '') === 'all-pipelines');
	if (!selected) return;
	const included = new Set((Array.isArray(selected.pipeline_ids) ? selected.pipeline_ids : []).map(String));
	view.structure_filter = String(selected.value || 'all-pipelines');
	view.visible_pipelines = pipelines.filter(pipeline => included.has(String(pipeline.pipeline_id || '')));
	view.show_chain_structure = !!selected.show_chain_structure;
	view.show_pipeline_structure = !!selected.show_pipeline_structure;
	view.structure_root = Object.assign({}, selected.root || {});
  }

  function managedYAMLBinding(definition) {
	const source = definition && typeof definition === 'object' ? definition : {};
	const projectID = Number(source.project_id || 0);
	const editing = projectID > 0;
	return {
	  title: editing ? 'Edit Managed YAML' : 'Add Managed YAML',
	  project_id: projectID,
	  project_name: String(source.project_name || (editing ? '' : 'New managed project')),
	  yaml: String(source.yaml || ''),
	  revision: String(source.revision || ''),
	  editing,
	  result: '',
	  result_tone: 'muted',
	};
  }

  function agentScriptBinding(view, agentID) {
	const agent = (view && view.agent) || {};
	const shells = (Array.isArray(agent.script_shells) ? agent.script_shells : []).map(shell => ({
	  value: String(shell.value || ''),
	  label: String(shell.label || shell.value || ''),
	  example_script: String(shell.example_script || ''),
	}));
	const selected = shells.length ? shells[0].value : '';
	return {
	  agent_id: String(agentID || agent.id || ''),
	  agent_label: String(agent.hostname || agent.id || agentID || ''),
	  shells,
	  selected_shell: selected,
	  script: shells.length ? shells[0].example_script : '',
	  can_run: !!agent.can_run_script && selected !== '',
	  result: '',
	  result_tone: 'muted',
	};
  }

  function vaultBinding(view) {
	const connections = Array.isArray(view && view.connections) ? view.connections : [];
	return {
	  connections,
	  connections_empty: connections.length === 0,
	  name: 'home-vault',
	  url: '',
	  role_id: '',
	  approle_mount: 'approle',
	  secret_id_env: 'CIWI_VAULT_SECRET_ID',
	  result: '',
	  result_tone: 'muted',
	};
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
    if (layout.align) element.style.alignItems = layout.align;
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

  function routePath() {
	return String(window.location.pathname || '/');
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

  function closeBrowserSelect() {
    if (!activeBrowserSelect) return;
    const {trigger, menu, onDocumentPointer, onWindowChange} = activeBrowserSelect;
    if (menu && menu.parentNode) menu.remove();
    if (trigger) trigger.setAttribute('aria-expanded', 'false');
    document.removeEventListener('pointerdown', onDocumentPointer, true);
    window.removeEventListener('resize', onWindowChange);
    window.removeEventListener('scroll', onWindowChange, true);
    activeBrowserSelect = null;
  }

  function layoutBrowserSelectMenu(trigger, menu) {
    const rect = trigger.getBoundingClientRect();
	const visuals = activeControls.select;
    const inset = visuals.viewportInset;
	const menuGap = visuals.menuGap;
    const availableWidth = Math.max(visuals.menuMinimumWidth, window.innerWidth - inset * 2);
    menu.style.maxWidth = availableWidth + 'px';
    menu.style.minWidth = Math.min(availableWidth, rect.width) + 'px';
    menu.style.left = Math.min(Math.max(inset, rect.left), Math.max(inset, window.innerWidth - inset - menu.offsetWidth)) + 'px';
    const below = window.innerHeight - rect.bottom - inset;
    const above = rect.top - inset;
    const placeAbove = menu.offsetHeight > below && above > below;
	const availableHeight = placeAbove ? above : below;
	menu.style.maxHeight = Math.max(visuals.menuMinimumHeight, Math.min(visuals.menuMaximumHeight, availableHeight)) + 'px';
    menu.style.top = (placeAbove ? Math.max(inset, rect.top - menu.offsetHeight - menuGap) : rect.bottom + menuGap) + 'px';
  }

  function configureBrowserSelect(trigger, options, selectedValue) {
    trigger.type = 'button';
    trigger.classList.add('dsl-select');
    trigger.value = selectedValue;
    const selected = options.find(option => option.value === selectedValue) || options[0] || {label: selectedValue};
    trigger.dataset.selectedLabel = selected.label;
    trigger.setAttribute('aria-haspopup', 'listbox');
    trigger.setAttribute('aria-expanded', 'false');
    const label = document.createElement('span');
    label.className = 'dsl-select-label';
    label.textContent = selected.label;
	appendPositionedIcon(trigger, label, declarativeIcon('chevron-down'), activeControls.select.chevronPosition);

    const updateIntrinsicWidth = () => {
      if (!document.body.contains(trigger)) return;
      const context = document.createElement('canvas').getContext('2d');
      const computed = window.getComputedStyle(trigger);
      context.font = computed.font;
      const widest = options.reduce((width, option) => Math.max(width, context.measureText(option.label).width), 0);
	  const padding = parseFloat(computed.paddingLeft) + parseFloat(computed.paddingRight);
	  const visuals = activeControls.select;
	  const contentWidth = widest + padding + visuals.chevronSize + visuals.chevronGap;
      trigger.style.width = 'min(100%, ' + String(Math.ceil(contentWidth)) + 'px)';
    };
    requestAnimationFrame(updateIntrinsicWidth);

    const open = () => {
      if (activeBrowserSelect && activeBrowserSelect.trigger === trigger) {
        closeBrowserSelect();
        return;
      }
      closeBrowserSelect();
      const menu = document.createElement('div');
      menu.className = 'dsl-select-menu';
      menu.setAttribute('role', 'listbox');
      menu.setAttribute('aria-label', trigger.getAttribute('aria-label') || 'Options');
      let activeIndex = Math.max(0, options.findIndex(option => option.value === trigger.value));
      const choices = options.map((option, index) => {
        const choice = document.createElement('button');
        choice.type = 'button';
        choice.className = 'dsl-select-option';
        choice.setAttribute('role', 'option');
        choice.setAttribute('aria-selected', String(option.value === trigger.value));
        const check = document.createElement('span');
        check.className = 'dsl-select-check';
        check.textContent = option.value === trigger.value ? '✓' : '';
        const copy = document.createElement('span');
        copy.textContent = option.label;
        choice.append(check, copy);
        choice.addEventListener('click', () => {
          trigger.value = option.value;
          trigger.dataset.selectedLabel = option.label;
          label.textContent = option.label;
          closeBrowserSelect();
          trigger.dispatchEvent(new Event('change', {bubbles: true}));
          trigger.focus();
        });
        choice.addEventListener('focus', () => { activeIndex = index; });
        menu.appendChild(choice);
        return choice;
      });
      const focusAt = index => {
        activeIndex = (index + choices.length) % choices.length;
        choices[activeIndex].focus();
      };
      menu.addEventListener('keydown', event => {
        if (event.key === 'ArrowDown') { event.preventDefault(); focusAt(activeIndex + 1); }
        if (event.key === 'ArrowUp') { event.preventDefault(); focusAt(activeIndex - 1); }
        if (event.key === 'Home') { event.preventDefault(); focusAt(0); }
        if (event.key === 'End') { event.preventDefault(); focusAt(choices.length - 1); }
        if (event.key === 'Escape') { event.preventDefault(); closeBrowserSelect(); trigger.focus(); }
      });
      document.body.appendChild(menu);
      const onDocumentPointer = event => {
        if (!menu.contains(event.target) && event.target !== trigger && !trigger.contains(event.target)) closeBrowserSelect();
      };
      const onWindowChange = () => layoutBrowserSelectMenu(trigger, menu);
      activeBrowserSelect = {trigger, menu, onDocumentPointer, onWindowChange};
      trigger.setAttribute('aria-expanded', 'true');
      layoutBrowserSelectMenu(trigger, menu);
      document.addEventListener('pointerdown', onDocumentPointer, true);
      window.addEventListener('resize', onWindowChange);
      window.addEventListener('scroll', onWindowChange, true);
      choices[activeIndex] && choices[activeIndex].focus();
    };
    trigger.addEventListener('click', open);
    trigger.addEventListener('keydown', event => {
      if (['Enter', ' ', 'ArrowDown', 'ArrowUp'].includes(event.key)) {
        event.preventDefault();
        open();
      }
    });
  }

  function elementContainsTextSelection(element) {
    const selection = window.getSelection && window.getSelection();
    if (!selection || selection.isCollapsed || !selection.anchorNode || !selection.focusNode) return false;
    return element.contains(selection.anchorNode) || element.contains(selection.focusNode);
  }

  function bindActions(element, actions, data, context) {
	const bindings = [];
    (actions || []).forEach(action => {
      const invoke = async (actionData, actionElement) => {
        const args = Object.fromEntries(Object.entries(action.arguments || {}).map(([key, value]) => [key, renderText({ template: value }, actionData)]));
        if (!window.ciwiConfirmAction(action.confirm)) return;
        const execute = async runtime => {
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
		  applyProjectStructureFilter(details, args.value || 'all-pipelines');
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
          updateOutputSearch(data.jobDetails, 0);
		  patchJobOutputRegion(data.jobDetails);
        }
        else if (action.command === 'find-output') {
          updateOutputSearch(data.jobDetails, args.direction === 'previous' ? -1 : 1);
		  patchJobOutputRegion(data.jobDetails);
          selectBrowserOutputMatch(data.jobDetails);
        }
        else if (action.command === 'copy-output') {
          await navigator.clipboard.writeText(String(data.jobDetails.output || ''));
        }
        else if (action.command === 'toggle-output-tailing') {
		  setOutputTailing(data.jobDetails, !data.jobDetails.output_tailing);
		  if (data.jobDetails.output_tailing) scrollJobOutputToEnd(document.getElementById('job-output-groups'));
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
		  const refreshedOptions = markBrowserViewReady(await response.json());
		  currentData = {runOptions: refreshedOptions, client: browserClientBinding()};
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
		}
		else if (action.command === 'agent-action') {
		  const response = await fetch('/api/v1/agents/' + encodeURIComponent(args.agentId) + '/actions', {
		    method: 'POST',
		    headers: ciwiActionHeaders(runtime, {'Content-Type': 'application/json'}),
		    body: JSON.stringify({action: args.action}),
		    signal: runtime.signal,
		  });
		  if (!response.ok) throw new Error(await response.text());
		  await refresh();
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
		  const response = await fetch(path, {
		    method: 'POST', headers: ciwiActionHeaders(runtime, {'Content-Type': 'application/json'}),
		    body: JSON.stringify(args.action === 'restart' ? {} : {target_version: args.targetVersion || ''}),
		    signal: runtime.signal,
		  });
		  if (!response.ok) throw new Error(await response.text());
		  const result = await response.json();
		  const settings = currentData.settings;
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
		if (runtime.refreshOnSuccess) await refresh({throwOnError: true});
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

  function definitionGraphNodes(graph, data) {
    const values = resolve(data, graph.nodes);
    const nodes = (Array.isArray(values) ? values : []).map(value => {
      const nodeData = Object.assign({}, data, {[graph.as]: value});
      const dependencies = graph.dependencies ? resolve(nodeData, graph.dependencies) : [];
      return {
        id: String(resolve(nodeData, graph.nodeKey)),
        label: renderText(graph.nodeLabel, nodeData),
        meta: renderText(graph.nodeMeta, nodeData),
        dependencies: (Array.isArray(dependencies) ? dependencies : []).map(String),
        data: nodeData,
        level: 0,
      };
    });
    if (!graph.root) return nodes;
    const rootValue = resolve(data, graph.root.binding);
    const rootData = Object.assign({}, data, {[graph.root.as]: rootValue});
    const rootID = '__root__:' + String(resolve(rootData, graph.root.key));
    const regularIDs = new Set(nodes.map(graphNode => graphNode.id));
    nodes.forEach(graphNode => {
      if (!graphNode.dependencies.some(dependency => regularIDs.has(dependency))) graphNode.dependencies.push(rootID);
    });
    nodes.unshift({
      id: rootID,
      label: renderText(graph.root.label, rootData),
      meta: renderText(graph.root.meta, rootData),
      dependencies: [], data: rootData, root: true, level: 0,
    });
    return nodes;
  }

  function graphRootActionVisible(root, data) {
    const condition = root && root.actionVisible;
    if (!condition) return true;
    const value = resolve(data, condition.binding);
    const equal = condition.empty
      ? String(value ?? '') === ''
      : String(value ?? '') === String(condition.equals || 'true');
    return condition.not ? !equal : equal;
  }

  function layoutDefinitionGraph(nodes) {
    const nodeWidth = 210;
    const nodeHeight = 76;
    const gapX = 58;
    const gapY = 24;
    const padding = 16;
    const byID = new Map(nodes.map(node => [node.id, node]));
    const states = new Map();
    const level = node => {
      if (states.get(node.id) === 2) return node.level;
      if (states.get(node.id) === 1) return 0;
      states.set(node.id, 1);
      node.dependencies.forEach(dependency => {
        const parent = byID.get(dependency);
        if (parent) node.level = Math.max(node.level, level(parent) + 1);
      });
      states.set(node.id, 2);
      return node.level;
    };
    const columns = new Map();
    let maxLevel = 0;
    nodes.forEach(node => {
      maxLevel = Math.max(maxLevel, level(node));
      if (!columns.has(node.level)) columns.set(node.level, []);
      columns.get(node.level).push(node);
    });
    const maxRows = Math.max(1, ...Array.from(columns.values(), values => values.length));
    columns.forEach((values, column) => {
      values.sort((left, right) => left.id.localeCompare(right.id));
      const topRows = Math.floor((maxRows - values.length) / 2);
      values.forEach((node, row) => {
        node.x = padding + column * (nodeWidth + gapX);
        node.y = padding + (topRows + row) * (nodeHeight + gapY);
      });
    });
    return {
      byID, nodeWidth, nodeHeight,
      width: 2 * padding + (maxLevel + 1) * nodeWidth + maxLevel * gapX,
      height: 2 * padding + maxRows * nodeHeight + (maxRows - 1) * gapY,
    };
  }

  function renderDefinitionGraph(node, data, selection, context) {
    const graphNodes = definitionGraphNodes(node.graphView, data);
    if (!graphNodes.length) {
      const empty = document.createElement('div');
      empty.className = 'dsl-definition-graph-empty';
      empty.textContent = 'No pipelines configured.';
      return empty;
    }
	const details = Array.isArray(node.graphView.details) ? node.graphView.details : [];
	if (details.length && !graphNodes.some(graphNode => !graphNode.root && graphNode.id === selection.value)) {
		const regular = graphNodes.filter(graphNode => !graphNode.root);
		selection.value = (regular.find(graphNode => !graphNode.dependencies.some(dependency => !dependency.startsWith('__root__:'))) || regular[0] || {}).id || '';
	}
    const layout = layoutDefinitionGraph(graphNodes);
    const wrapper = document.createElement('div');
    wrapper.className = 'dsl-definition-graph';
    const toolbar = document.createElement('div');
    toolbar.className = 'dsl-definition-graph-toolbar';
    const viewport = document.createElement('div');
    viewport.className = 'dsl-definition-graph-viewport';
    const scaler = document.createElement('div');
    scaler.className = 'dsl-definition-graph-scaler';
    const stage = document.createElement('div');
    stage.className = 'dsl-definition-graph-stage';
    stage.style.width = layout.width + 'px';
    stage.style.height = layout.height + 'px';
    const edges = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
    edges.classList.add('dsl-definition-graph-edges');
    edges.setAttribute('viewBox', '0 0 ' + layout.width + ' ' + layout.height);
    graphNodes.forEach(graphNode => {
      graphNode.dependencies.forEach(dependency => {
        const parent = layout.byID.get(dependency);
        if (!parent) return;
        const startX = parent.x + layout.nodeWidth;
        const startY = parent.y + layout.nodeHeight / 2;
        const endX = graphNode.x;
        const endY = graphNode.y + layout.nodeHeight / 2;
        const middle = (startX + endX) / 2;
        const path = document.createElementNS('http://www.w3.org/2000/svg', 'path');
        path.setAttribute('d', 'M ' + startX + ' ' + startY + ' C ' + middle + ' ' + startY + ', ' + middle + ' ' + endY + ', ' + endX + ' ' + endY);
        edges.appendChild(path);
        const arrow = document.createElementNS('http://www.w3.org/2000/svg', 'polygon');
        arrow.setAttribute('points', endX + ',' + endY + ' ' + (endX - 8) + ',' + (endY - 5) + ' ' + (endX - 8) + ',' + (endY + 5));
        edges.appendChild(arrow);
      });
    });
    stage.appendChild(edges);
    graphNodes.forEach(graphNode => {
      const card = document.createElement('div');
      card.className = 'dsl-definition-graph-node' + (graphNode.id === selection.value ? ' selected' : '');
	  if (graphNode.root) card.classList.add('dsl-definition-graph-root');
      card.style.left = graphNode.x + 'px';
      card.style.top = graphNode.y + 'px';
      card.style.width = layout.nodeWidth + 'px';
      card.style.height = layout.nodeHeight + 'px';
	  if (details.length && !graphNode.root) {
		card.classList.add('selectable');
		card.tabIndex = 0;
		card.setAttribute('role', 'button');
		card.setAttribute('aria-label', 'Select ' + graphNode.label);
		const select = () => {
			if (selection.value === graphNode.id) return;
			selection.value = graphNode.id;
			selection.onChange(graphNode.id);
		};
		card.addEventListener('click', select);
		card.addEventListener('keydown', event => {
			if (event.key === 'Enter' || event.key === ' ') {
				event.preventDefault();
				select();
			}
		});
	  }
      const copy = document.createElement('div');
      copy.className = 'dsl-definition-graph-node-copy';
      const title = document.createElement('div');
      title.className = 'dsl-definition-graph-node-title';
      title.textContent = graphNode.label;
      title.title = graphNode.label;
      const meta = document.createElement('div');
      meta.className = 'dsl-definition-graph-node-meta';
      meta.textContent = graphNode.meta;
      meta.title = graphNode.meta;
      copy.append(title, meta);
      card.appendChild(copy);
      const actions = graphNode.root ? ((node.graphView.root && node.graphView.root.actions) || []) : (node.actions || []);
      const actionsVisible = !graphNode.root || graphRootActionVisible(node.graphView.root, graphNode.data);
      if (actions.length && actionsVisible) {
        const play = document.createElement('button');
        play.className = 'dsl-button dsl-icon-button dsl-definition-graph-node-play';
        const runHelp = 'Run ' + graphNode.label + ' as a new execution. Existing queued and running work is not interrupted.';
        play.setAttribute('aria-label', runHelp);
        play.title = runHelp;
        play.appendChild(declarativeIcon('player-play'));
		play.addEventListener('click', event => event.stopPropagation());
        const actionIdentity = context.identity + '/graph-node:' + rendererKeyPart(graphNode.id) + '/action';
		annotateRendererElement(play, 'button', actionIdentity);
        bindActions(play, actions, graphNode.data, {session: context.session, identity: actionIdentity});
        card.appendChild(play);
      }
      stage.appendChild(card);
    });
    scaler.appendChild(stage);
    viewport.appendChild(scaler);
    let scale = 1;
    const clamp = value => Math.min(1.5, Math.max(0.45, value));
    const scaleLabel = document.createElement('span');
    scaleLabel.className = 'dsl-definition-graph-scale';
    const applyScale = next => {
      scale = clamp(next);
      scaler.style.width = Math.round(layout.width * scale) + 'px';
      scaler.style.height = Math.round(layout.height * scale) + 'px';
      stage.style.transform = 'scale(' + scale + ')';
      scaleLabel.textContent = Math.round(scale * 100) + '%';
    };
    const control = (label, icon, action) => {
      const button = document.createElement('button');
      button.className = 'dsl-button' + (icon ? ' dsl-icon-button' : '');
      button.setAttribute('aria-label', label);
      button.title = label;
      if (icon) button.appendChild(declarativeIcon(icon));
      else button.textContent = label;
      button.addEventListener('click', action);
      return button;
    };
    const fit = () => applyScale(Math.min(
      (Math.max(1, viewport.clientWidth) - 32) / layout.width,
      388 / layout.height,
    ));
    toolbar.append(
      control('Fit', '', fit),
      control('Reset', '', () => applyScale(1)),
      control('Zoom out', 'zoom-out', () => applyScale(scale - 0.1)),
      scaleLabel,
      control('Zoom in', 'zoom-in', () => applyScale(scale + 0.1)),
    );
	wrapper.append(toolbar, viewport);
	if (details.length) {
		const selected = graphNodes.find(graphNode => !graphNode.root && graphNode.id === selection.value);
		if (selected) {
			const detail = document.createElement('div');
			detail.className = 'dsl-definition-graph-details';
			details.forEach((child, index) => detail.appendChild(renderNode(child, selected.data, childRenderContext(
			  context,
			  'graph-details:' + String(index),
			  context.identity + '/graph-node:' + rendererKeyPart(selected.id) + '/details:' + String(index),
			))));
			wrapper.appendChild(detail);
		}
	}
    requestAnimationFrame(fit);
    return wrapper;
  }

  function renderGraphView(element, node, data, context) {
    const stateKey = renderText({template: node.graphView.stateKey}, data);
    let mode = viewStates[stateKey];
    if (mode !== 'graph' && mode !== 'list') mode = node.graphView.defaultMode === 'list' ? 'list' : 'graph';
    const header = document.createElement('div');
    header.className = 'dsl-graph-view-header';
    const heading = document.createElement('div');
    heading.className = 'dsl-heading';
    heading.textContent = renderText(node.text, data) || 'Structure';
    const modes = document.createElement('div');
    modes.className = 'dsl-graph-view-modes';
    const body = document.createElement('div');
    body.className = 'dsl-graph-view-body';
	let selectedID = '';
    const renderBody = () => {
      body.replaceChildren();
	  if (mode === 'graph') body.appendChild(renderDefinitionGraph(node, data, {
		value: selectedID,
		onChange: id => {
			selectedID = id;
			renderBody();
		},
	  }, context));
      else (node.children || []).forEach((child, index) => body.appendChild(renderNode(
		child,
		data,
		childRenderContext(context, 'children:' + String(index)),
	  )));
      Array.from(modes.children).forEach(button => button.setAttribute('aria-pressed', String(button.dataset.mode === mode)));
    };
    ['graph', 'list'].forEach(value => {
      const button = document.createElement('button');
      button.className = 'dsl-button dsl-graph-view-mode';
      button.dataset.mode = value;
      button.textContent = value === 'graph' ? 'Graph' : 'List';
      button.addEventListener('click', () => {
        mode = value;
        viewStates[stateKey] = value;
        saveViewStates();
        renderBody();
      });
      modes.appendChild(button);
    });
    header.append(heading, modes);
    element.append(header, body);
    renderBody();
  }

  function renderTreeView(element, node, data, context) {
	const tree = node.treeView || {};
	const filter = tree.filter ? String(resolve(data, tree.filter) || 'all') : '';
	const source = resolve(data, tree.nodes);

	function prepared(raw, depth) {
	  const itemData = Object.assign({}, data, {[tree.as]: raw});
	  const rawChildren = resolve(itemData, tree.children);
	  const children = (Array.isArray(rawChildren) ? rawChildren : []).map(child => prepared(child, depth + 1)).filter(Boolean);
	  const filterValues = tree.filterValues ? resolve(itemData, tree.filterValues) : [];
	  const values = Array.isArray(filterValues) ? filterValues.map(String) : [];
	  if (filter && filter !== 'all' && values.length && !values.includes(filter)) return null;
	  if (filter && filter !== 'all' && Array.isArray(rawChildren) && rawChildren.length && children.length === 0) return null;
	  return {raw, itemData, children, depth};
	}

	function renderEntry(entry) {
	  const itemData = entry.itemData;
	  const entryKey = String(resolve(itemData, tree.nodeKey) ?? '').trim();
	  if (!entryKey) throw new Error('Empty tree node key at ' + context.path);
	  const entryIdentity = context.identity + '/tree-node:' + rendererKeyPart(entryKey);
	  const row = document.createElement('div');
	  row.className = 'dsl-tree-row';
	  annotateRendererElement(row, 'tree-row', entryIdentity + '/row');
	  row.style.setProperty('--ciwi-tree-depth', String(entry.depth));
	  let link = tree.nodeLink ? String(resolve(itemData, tree.nodeLink) || '') : '';
	  const fileDownload = (node.actions || []).find(action => action.command === 'download-artifact');
	  if (!link && fileDownload) {
		const args = Object.fromEntries(Object.entries(fileDownload.arguments || {}).map(([key, value]) => [key, renderText({template: value}, itemData)]));
		if (String(args.kind || '') === 'file') {
		  const jobID = encodeURIComponent(args.jobExecutionId || '');
		  const artifactPath = String(args.path || '').split('/').map(encodeURIComponent).join('/');
		  link = '/artifacts/' + jobID + '/' + artifactPath;
		}
	  }
	  const label = renderText(tree.nodeLabel, itemData);
	  const labelElement = document.createElement(link ? 'a' : 'span');
	  labelElement.className = 'dsl-tree-label';
	  labelElement.textContent = label;
	  if (link) {
		labelElement.href = link;
		labelElement.target = '_blank';
		labelElement.rel = 'noopener noreferrer';
	  }
	  row.appendChild(labelElement);
	  const detail = tree.nodeDetail ? renderText(tree.nodeDetail, itemData) : '';
	  if (detail) {
		const detailElement = document.createElement('span');
		detailElement.className = 'dsl-tree-detail';
		detailElement.textContent = detail;
		const tone = tree.nodeTone ? semanticTone(resolve(itemData, tree.nodeTone)) : '';
		if (tone) detailElement.classList.add('dsl-' + tone);
		row.appendChild(detailElement);
	  }
	  const actionLabel = tree.actionLabel ? renderText(tree.actionLabel, itemData) : '';
	  if (actionLabel && Array.isArray(node.actions) && node.actions.length) {
		const button = document.createElement('button');
		button.className = 'dsl-button dsl-tree-action';
		button.type = 'button';
		button.textContent = actionLabel;
		const actionIdentity = entryIdentity + '/action';
		annotateRendererElement(button, 'button', actionIdentity);
		bindActions(button, node.actions, itemData, {session: context.session, identity: actionIdentity});
		row.appendChild(button);
	  }
	  if (!entry.children.length) return row;
	  const details = document.createElement('details');
	  details.className = 'dsl-tree-branch';
	  const key = entryKey;
	  const stateKey = String(tree.stateKey || '') + ':' + key;
	  const fallback = tree.defaultExpanded ? !!resolve(itemData, tree.defaultExpanded) : false;
	  details.open = disclosureStates.get(stateKey, fallback);
	  details.dataset.disclosureKey = stateKey;
	  details.addEventListener('toggle', () => disclosureStates.set(stateKey, details.open));
	  const summary = document.createElement('summary');
	  summary.appendChild(row);
	  details.appendChild(summary);
	  const children = document.createElement('div');
	  children.className = 'dsl-tree-children';
	  entry.children.forEach(child => children.appendChild(renderEntry(child)));
	  details.appendChild(children);
	  return details;
	}

	const preparedNodes = (Array.isArray(source) ? source : []).map(item => prepared(item, 0)).filter(Boolean);
	preparedNodes.forEach(entry => element.appendChild(renderEntry(entry)));
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
	  renderGraphView(element, node, data, context);
	  return element;
	}
	if (node.component === 'tree-view' && node.treeView) {
	  renderTreeView(element, node, data, context);
	  return element;
	}
	if (node.enabled) {
	  const equal = node.enabled.empty
	    ? String(resolve(data, node.enabled.binding)) === ''
	    : String(resolve(data, node.enabled.binding)) === String(node.enabled.equals || 'true');
	  element.disabled = node.enabled.not ? equal : !equal;
	}
    if (node.component === 'disclosure') {
      const summary = document.createElement('summary');
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
		(node.disclosure.summary || []).forEach((summaryNode, index) => summary.appendChild(renderNode(
		  summaryNode,
		  data,
		  childRenderContext(context, 'summary:' + String(index)),
		)));
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
	  configureBrowserSelect(element, renderedOptions, current);
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
	  label.textContent = copy;
	  element.appendChild(label);
	}
    if (node.component === 'button' && node.icon) {
	  const icon = declarativeIcon(node.icon);
	  const label = element.querySelector('.dsl-button-label');
	  if (label) appendPositionedIcon(element, label, icon, activeControls.button.iconPosition);
	  else element.prepend(icon);
    }
    bindActions(element, node.actions, data, context);
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

  const replaceOnRefreshComponents = new Set(['graph-view', 'tree-view', 'select']);

  function rendererNodeKey(node) {
	return node && node.nodeType === Node.ELEMENT_NODE ? String(node.dataset.ciwiNodeKey || '') : '';
  }

  function compatibleRenderedNodes(current, next) {
	if (!current || !next || current.nodeType !== next.nodeType) return false;
	if (current.nodeType === Node.TEXT_NODE || current.nodeType === Node.COMMENT_NODE) return true;
	if (current.nodeType !== Node.ELEMENT_NODE) return false;
	if (current.namespaceURI !== next.namespaceURI || current.localName !== next.localName) return false;
	const currentKey = rendererNodeKey(current);
	const nextKey = rendererNodeKey(next);
	if ((currentKey || nextKey) && currentKey !== nextKey) return false;
	const currentComponent = String(current.dataset.ciwiComponent || '');
	const nextComponent = String(next.dataset.ciwiComponent || '');
	if ((currentComponent || nextComponent) && currentComponent !== nextComponent) return false;
	if (replaceOnRefreshComponents.has(nextComponent)) return false;
	const currentIcon = String(current.dataset.ciwiIcon || '');
	const nextIcon = String(next.dataset.ciwiIcon || '');
	return !((currentIcon || nextIcon) && currentIcon !== nextIcon);
  }

  function patchRenderedAttributes(current, next) {
	Array.from(current.attributes).forEach(attribute => {
	  if (!next.hasAttribute(attribute.name)) current.removeAttribute(attribute.name);
	});
	Array.from(next.attributes).forEach(attribute => {
	  if (current.getAttribute(attribute.name) !== attribute.value) current.setAttribute(attribute.name, attribute.value);
	});
  }

  function patchRenderedElement(current, next) {
	const previousProgressState = String(current.__ciwiSemanticProgressState || '');
	const nextProgressState = String(next.__ciwiSemanticProgressState || '');
	const preserveProgressAnimation = previousProgressState !== '' && previousProgressState === nextProgressState;
	const previousProgressDelay = preserveProgressAnimation
	  ? current.style.getPropertyValue('--ciwi-progress-animation-delay')
	  : '';
	patchRenderedAttributes(current, next);
	if (preserveProgressAnimation) {
	  if (previousProgressDelay) current.style.setProperty('--ciwi-progress-animation-delay', previousProgressDelay);
	  else current.style.removeProperty('--ciwi-progress-animation-delay');
	}
	if (Object.prototype.hasOwnProperty.call(next, '__ciwiRenderedValue')) {
	  const previousValue = String(current.__ciwiRenderedValue ?? '');
	  const nextValue = String(next.__ciwiRenderedValue ?? '');
	  if (!Object.prototype.hasOwnProperty.call(current, '__ciwiRenderedValue') || previousValue !== nextValue) {
		current.value = nextValue;
	  }
	  current.__ciwiRenderedValue = nextValue;
	}
	if (Object.prototype.hasOwnProperty.call(next, '__ciwiSemanticProgress')) {
	  current.__ciwiSemanticProgress = next.__ciwiSemanticProgress;
	  current.__ciwiSemanticProgressState = preserveProgressAnimation ? previousProgressState : nextProgressState;
	}
	if (Object.prototype.hasOwnProperty.call(next, '__ciwiPulseTimestamp')) {
	  current.__ciwiPulseTimestamp = next.__ciwiPulseTimestamp;
	}
  }

  function reconcileRenderedChildren(current, next) {
	const previousChildren = Array.from(current.childNodes);
	const keyedChildren = new Map(previousChildren.map(child => [rendererNodeKey(child), child]).filter(([key]) => key));
	const nextKeys = new Set(Array.from(next.childNodes).map(rendererNodeKey).filter(Boolean));
	keyedChildren.forEach((child, key) => {
	  if (!nextKeys.has(key)) child.remove();
	});
	const retained = new Set();
	Array.from(next.childNodes).forEach((nextChild, index) => {
	  const key = rendererNodeKey(nextChild);
	  let candidate = key ? keyedChildren.get(key) : current.childNodes[index];
	  if (candidate && retained.has(candidate)) candidate = null;
	  if (candidate && !key && rendererNodeKey(candidate)) candidate = null;
	  let committedChild = nextChild;
	  if (candidate && compatibleRenderedNodes(candidate, nextChild)) {
		committedChild = reconcileRenderedNode(candidate, nextChild);
		retained.add(candidate);
	  } else if (candidate && candidate.parentNode === current) {
		candidate.replaceWith(nextChild);
	  }
	  const reference = current.childNodes[index] || null;
	  if (committedChild !== reference) current.insertBefore(committedChild, reference);
	});
	previousChildren.forEach(child => {
	  if (!retained.has(child) && child.parentNode === current) child.remove();
	});
  }

  function reconcileRenderedNode(current, next) {
	if (!compatibleRenderedNodes(current, next)) {
	  current.replaceWith(next);
	  return next;
	}
	if (current.nodeType === Node.TEXT_NODE || current.nodeType === Node.COMMENT_NODE) {
	  if (current.data !== next.data) current.data = next.data;
	  return current;
	}
	patchRenderedElement(current, next);
	reconcileRenderedChildren(current, next);
	return current;
  }

  function renderCurrent() {
    if (!currentDocument || !currentData) return;
    closeBrowserSelect();
	const session = createRenderSession(currentDocument.metadata && currentDocument.metadata.name);
	const nextRoot = renderNode(currentDocument.screen.root, currentData, rootRenderContext(session));
	const nextSignature = session.screenName + ':' + currentPath;
	const previousRoot = root.childNodes.length === 1 ? root.firstChild : null;
	const reconcile = committedRenderSignature === nextSignature && compatibleRenderedNodes(previousRoot, nextRoot);
	const viewState = reconcile ? null : window.ciwiCaptureViewState(root);
	if (reconcile) reconcileRenderedNode(previousRoot, nextRoot);
	else root.replaceChildren(nextRoot);
	committedActionBindings = session.actionBindings;
	committedRenderSignature = nextSignature;
	if (viewState) window.ciwiRestoreViewState(root, viewState);
	if (currentData.jobDetails) bindJobOutputScrollIntent(currentData.jobDetails);
	requestAnimationFrame(updateDeclarativeOutputCollapseButtons);
  }

  async function navigateBrowser(path, options = {}) {
	const targetPath = String(path || '/');
	const previousPath = currentPath || routePath();
	if (!options.fromHistory) window.history.pushState({}, '', targetPath);
	try {
	  await refresh({throwOnError: true, showLoading: true});
	  if (options.section) {
		const target = document.getElementById(String(options.section));
		if (target) target.scrollIntoView({block: 'start'});
	  }
	} catch (error) {
	  window.history.replaceState({}, '', previousPath);
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
	  updateVersions: declarativeVersionOptions(versions, upToDate ? 'No newer versions available' : 'Check for updates'),
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
    const threshold = Math.max(480, container.clientHeight);
    container.querySelectorAll('details.dsl-output-group').forEach(details => {
      const button = details.querySelector(':scope > .dsl-floating-collapse');
      if (!button) return;
      const summary = details.querySelector(':scope > summary');
      const contentHeight = details.open ? Math.max(0, details.scrollHeight - Number((summary && summary.offsetHeight) || 0)) : 0;
      button.hidden = !details.open || contentHeight <= threshold;
    });
  }

  function findDeclarativeNodeByID(node, id, path = 'root') {
	if (!node || typeof node !== 'object') return null;
	if (node.id === id) return {node, path};
	for (let index = 0; index < (node.children || []).length; index += 1) {
	  const found = findDeclarativeNodeByID(node.children[index], id, path + '/children:' + String(index));
	  if (found) return found;
	}
	return null;
  }

  function outputIsAtBottom(element) {
	return !element || element.scrollHeight - element.clientHeight - element.scrollTop <= 3;
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
	if (!container || container.dataset.ciwiScrollIntent === '1') return;
	container.dataset.ciwiScrollIntent = '1';
	container.addEventListener('scroll', () => {
	  if (programmaticOutputScroll || !view.output_tailing || outputIsAtBottom(container)) return;
	  setOutputTailing(view, false);
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
	const contractRoot = currentDocument && currentDocument.screen && currentDocument.screen.root;
	const systemMatch = findDeclarativeNodeByID(contractRoot, 'job-output-system');
	const groupsMatch = findDeclarativeNodeByID(contractRoot, 'job-output-groups');
	const previousScroller = document.getElementById('job-output-groups');
	if (!systemMatch || !groupsMatch || !previousScroller) {
	  renderCurrent();
	  return;
	}
	const session = createRenderSession(
	  currentDocument.metadata && currentDocument.metadata.name,
	  new Map(committedActionBindings),
	);
	const screenScope = 'screen:' + session.screenName;
	const subtreeActionPrefixes = [systemMatch, groupsMatch].map(match => (
	  screenScope + '/id:' + rendererKeyPart(match.node.id)
	));
	session.actionBindings.forEach((_, key) => {
	  if (subtreeActionPrefixes.some(prefix => key === prefix || key.startsWith(prefix + '/'))) {
		session.actionBindings.delete(key);
	  }
	});
	const contextForMatch = match => ({
	  session,
	  path: screenScope + '/' + match.path,
	  parentScope: screenScope + '/' + match.path.substring(0, Math.max(0, match.path.lastIndexOf('/'))),
	  repeatIdentity: '',
	  inRepeat: false,
	});
	const nextScroller = renderNode(groupsMatch.node, currentData, contextForMatch(groupsMatch));
	if (!nextScroller || nextScroller.nodeType !== Node.ELEMENT_NODE) {
	  renderCurrent();
	  return;
	}
	const previousSystem = document.getElementById('job-output-system');
	const renderedSystem = renderNode(systemMatch.node, currentData, contextForMatch(systemMatch));
	const nextSystem = renderedSystem && renderedSystem.nodeType === Node.ELEMENT_NODE ? renderedSystem : null;
	if (previousSystem && nextSystem) previousSystem.replaceWith(nextSystem);
	else if (previousSystem) previousSystem.remove();
	else if (nextSystem) previousScroller.before(nextSystem);

	const previousGroups = new Map(Array.from(previousScroller.children).map(group => [String(group.dataset.disclosureKey || ''), group]));
	const retained = new Set();
	Array.from(nextScroller.children).forEach(nextGroup => {
	  const key = String(nextGroup.dataset.disclosureKey || '');
	  const previousGroup = previousGroups.get(key);
	  if (!key || !previousGroup || previousGroup.tagName !== nextGroup.tagName) {
		previousScroller.appendChild(nextGroup);
		return;
	  }
	  retained.add(previousGroup);
	  previousGroup.className = nextGroup.className;
	  previousGroup.style.cssText = nextGroup.style.cssText;
	  previousGroup.__ciwiSemanticProgress = nextGroup.__ciwiSemanticProgress;
	  previousGroup.replaceChildren(...Array.from(nextGroup.childNodes));
	});
	previousGroups.forEach(group => {
	  if (!retained.has(group)) group.remove();
	});
	committedActionBindings = session.actionBindings;
	bindJobOutputScrollIntent(view);
	if (view.output_tailing) scrollJobOutputToEnd(previousScroller);
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
      group.is_phase = group.kind === 'phase';
      group.is_step = group.kind !== 'phase';
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
		if (topics.some(topic => watched.has(topic))) scheduleChangeRefresh();
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
	  const loadingView = options.showLoading ? (cachedView || browserLoadingBinding(nextRouteMatch)) : null;
	  if (loadingView) {
		const [loadingDocument, loadingThemes, loadingControls] = await Promise.all([documentPromise, themesPromise, controlsPromise]);
		if (loadGeneration !== routeLoadGeneration) return false;
		applyContractTheme(loadingThemes);
		applyControlsContract(loadingControls);
		setOutputWatchGeneration(generation);
		currentRouteMatch = nextRouteMatch;
		currentPath = routePath();
		currentDocument = loadingDocument;
		currentData = {[bindingRoot]: loadingView, client: browserClientBinding()};
		renderCurrent();
		loadingCommitted = true;
	  }
	  const [documentContract, themes, controls, viewResponse] = await Promise.all([documentPromise, themesPromise, controlsPromise, viewPromise]);
	  if (!viewResponse.ok) throw new Error(await viewResponse.text());
	  const responseView = await viewResponse.json();
      applyContractTheme(themes);
	  applyControlsContract(controls);
      let view = responseView;
	  if (managedYAMLMatch) view = managedYAMLBinding(responseView);
	  if (agentScriptMatch) view = agentScriptBinding(responseView, nextRouteMatch.params.agentId);
	  if (vaultMatch) view = vaultBinding(responseView);
	  if (projectMatch) {
		decorateProjectDetails(view);
	  }
	  if (routeName === 'front-page') {
		decorateFrontPageProjects(view.projects);
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
			? declarativeExecutionTimestamp(updatedUTC)
			: (updatedMilliseconds > 0 ? declarativeExecutionTimestamp(new Date(updatedMilliseconds).toISOString()) : '');
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
		decorateJobDetails(view);
		view.output_search = sameJob ? String(previousJob.output_search || '') : '';
		view.output_match_index = sameJob ? Number(previousJob.output_match_index || 0) : 0;
		initializeJobOutputView(view, sameJob ? previousJob : null);
		updateOutputSearch(view, 0);
		view.output_tailing = sameJob ? !!previousJob.output_tailing : true;
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
	  markBrowserViewReady(view);
	  setOutputWatchGeneration(generation);
	  currentRouteMatch = nextRouteMatch;
	  currentPath = routePath();
	  currentDocument = documentContract;
	  currentData = { [bindingRoot]: view, client: browserClientBinding() };
	  browserViewCache.set(cacheKey, view);
      renderCurrent();
      if (jobMatch) {
		const jobID = nextRouteMatch.params.jobId;
		if (completedOutputJobID !== String(jobID)) try {
		  watchJobOutput(jobID, generation);
		} catch (error) {
          if (generation !== outputWatchGeneration) return;
          if (currentData && currentData.jobDetails) {
            appendBoundedOutput(currentData.jobDetails, '', 'Output stream failed: ' + (error.message || String(error)) + '\n');
            rebuildJobOutputText(currentData.jobDetails);
			patchJobOutputRegion(currentData.jobDetails);
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
      root.replaceChildren(message);
	  return false;
    }
	return true;
    } finally {
	  if (scheduler) scheduler.endRefresh();
	}
  }

  window.addEventListener('popstate', () => {
	navigateBrowser(routePath(), {fromHistory: true}).catch(error => window.alert(error.message || String(error)));
  });
  changeRefreshScheduler = window.ciwiCreateChangeRefreshScheduler({refresh: () => refresh(), delay: 100});
  refresh({showLoading: true}).finally(startChangeWatch);
})();
