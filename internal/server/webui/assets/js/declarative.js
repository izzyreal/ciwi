(() => {
  'use strict';

  const root = document.getElementById('declarativeRoot');
  let outputWatchGeneration = 0;
  const maxOutputCharacters = 1024 * 1024;
  let currentDocument = null;
  let currentData = null;
  const disclosureStorageKey = 'ciwi.declarative.disclosures.v1';
  const disclosureStates = loadDisclosureStates();

  function loadDisclosureStates() {
    try {
      const parsed = JSON.parse(localStorage.getItem(disclosureStorageKey) || '{}');
      return parsed && typeof parsed === 'object' && !Array.isArray(parsed) ? parsed : {};
    } catch (_) { return {}; }
  }

  function saveDisclosureStates() {
    try { localStorage.setItem(disclosureStorageKey, JSON.stringify(disclosureStates)); } catch (_) {}
  }

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

  function semanticTone(value) {
    switch (String(value || '').trim().toLowerCase()) {
      case 'succeeded': case 'success': case 'passed': case 'complete': case 'completed': return 'success';
      case 'failed': case 'failure': case 'error': case 'cancelled': case 'canceled': return 'danger';
      case 'queued': case 'waiting': case 'pending': case 'not reached': return 'warning';
      case 'running': case 'leased': case 'in progress': case 'active': return 'accent';
      default: return 'muted';
    }
  }

  function decorateExecutionCards(cards, queued) {
    (Array.isArray(cards) ? cards : []).forEach(card => {
      const summary = card.summary || {};
      card.status = Number(summary.failed || 0) > 0
        ? 'failed'
        : (queued ? (Number(summary.in_progress || 0) > 0 ? 'running' : 'waiting') : 'succeeded');
	  card.job_execution_ids_csv = (Array.isArray(card.job_execution_ids) ? card.job_execution_ids : []).join(',');
    });
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

  function runOptionsViewURL(sourceRef, agentID) {
	const projectPipelineMatch = window.location.pathname.match(/^\/declarative-preview\/run-options\/projects\/(\d+)\/pipelines\/(\d+)\/?$/);
    const pipelineMatch = window.location.pathname.match(/^\/declarative-preview\/run-options\/pipelines\/(\d+)\/?$/);
    const chainMatch = window.location.pathname.match(/^\/declarative-preview\/run-options\/projects\/(\d+)\/chains\/([^/]+)\/?$/);
    let path = '';
	if (projectPipelineMatch) path = '/api/v1/views/run-options/pipelines/' + encodeURIComponent(projectPipelineMatch[2]);
    if (pipelineMatch) path = '/api/v1/views/run-options/pipelines/' + encodeURIComponent(pipelineMatch[1]);
    if (chainMatch) path = '/api/v1/views/run-options/projects/' + encodeURIComponent(chainMatch[1]) + '/chains/' + encodeURIComponent(chainMatch[2]);
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
      case 'select': return document.createElement('select');
      case 'input': return document.createElement('input');
      case 'image': return document.createElement('img');
      case 'divider': return document.createElement('hr');
      case 'spacer': return document.createElement('span');
      default: return document.createElement('div');
    }
  }

  function bindActions(element, actions, data) {
    (actions || []).forEach(action => {
      const invoke = async actionData => {
        const args = Object.fromEntries(Object.entries(action.arguments || {}).map(([key, value]) => [key, renderText({ template: value }, actionData)]));
        if (action.confirm && !window.confirm(action.confirm.message || action.confirm.title || 'Continue?')) return;
        if (action.command === 'navigate' && args.route) {
          const inPreview = window.location.pathname.startsWith('/declarative-preview');
          const destination = inPreview
            ? (args.route === '/' ? '/declarative-preview' : '/declarative-preview' + args.route)
            : args.route;
          window.location.assign(destination);
        }
        else if (action.command === 'refresh') refresh();
        else if (action.command === 'change-theme') {
          ciwiApplyTheme(args.theme);
          await refresh();
        }
        else if (action.command === 'select-timeline-item') {
          data.jobDetails.selected_timeline_item = data.item;
          renderCurrent();
        }
        else if (action.command === 'change-output-search') {
          data.jobDetails.output_search = args.query || '';
          updateOutputSearch(data.jobDetails, 0);
          renderCurrent();
        }
        else if (action.command === 'find-output') {
          updateOutputSearch(data.jobDetails, args.direction === 'previous' ? -1 : 1);
          renderCurrent();
          selectBrowserOutputMatch(data.jobDetails);
        }
        else if (action.command === 'copy-output') {
          await navigator.clipboard.writeText(String(data.jobDetails.output || ''));
        }
        else if (action.command === 'toggle-output-tailing') {
          data.jobDetails.output_tailing = !data.jobDetails.output_tailing;
          data.jobDetails.tailing_label = data.jobDetails.output_tailing ? 'Tailing: On' : 'Tailing: Off';
          renderCurrent();
        }
        else if (action.command === 'set-disclosures') {
          const expanded = args.expanded === 'true';
          document.querySelectorAll('[data-disclosure-key^="' + CSS.escape(args.prefix || '') + '"]').forEach(details => {
            details.open = expanded;
            disclosureStates[details.dataset.disclosureKey] = expanded;
          });
          saveDisclosureStates();
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
		  const response = await fetch(runOptionsViewURL(options.selected_source_ref, options.selected_agent_id));
		  if (!response.ok) throw new Error(await response.text());
		  currentData = {runOptions: await response.json()};
		  renderCurrent();
		}
        else if (action.command === 'run-pipeline') {
          const response = await fetch('/api/v1/pipelines/' + encodeURIComponent(args.pipelineDbId) + '/run-selection', {
            method: 'POST',
            headers: {'Content-Type': 'application/json', 'Idempotency-Key': crypto.randomUUID()},
            body: JSON.stringify(runSelectionFromArguments(args)),
          });
          if (!response.ok) throw new Error(await response.text());
          element.textContent = 'Queued';
        }
		else if (action.command === 'run-chain') {
		  const path = '/api/v1/projects/' + encodeURIComponent(args.projectId) + '/pipeline-chains/' + encodeURIComponent(args.chainId) + '/run';
		  const response = await fetch(path, {
		    method: 'POST', headers: {'Content-Type': 'application/json', 'Idempotency-Key': crypto.randomUUID()},
		    body: JSON.stringify(runSelectionFromArguments(args)),
		  });
		  if (!response.ok) throw new Error(await response.text());
		  element.textContent = 'Queued';
		}
		else if (action.command === 'clear-queue') {
		  const response = await fetch('/api/v1/jobs/clear-queue', {method: 'POST', headers: {'Content-Type': 'application/json'}, body: '{}'});
		  if (!response.ok) throw new Error(await response.text());
		  await refresh();
		}
		else if (action.command === 'flush-history' || action.command === 'delete-execution') {
		  const ids = action.command === 'delete-execution'
		    ? String(args.jobExecutionIds || '').split(',').map(value => value.trim()).filter(Boolean)
		    : null;
		  const body = ids === null ? {} : {job_execution_ids: ids};
		  const response = await fetch('/api/v1/jobs/flush-history', {
		    method: 'POST', headers: {'Content-Type': 'application/json'}, body: JSON.stringify(body),
		  });
		  if (!response.ok) throw new Error(await response.text());
		  await refresh();
		}
		else if (action.command === 'cancel-execution') {
		  const response = await fetch('/api/v1/jobs/' + encodeURIComponent(args.jobExecutionId) + '/cancel', {method: 'POST'});
		  if (!response.ok) throw new Error(await response.text());
		  await refresh();
		}
		else if (action.command === 'rerun-execution') {
		  const response = await fetch('/api/v1/jobs/' + encodeURIComponent(args.jobExecutionId) + '/rerun', {method: 'POST'});
		  if (!response.ok) throw new Error(await response.text());
		  const result = await response.json();
		  const rerunID = result && result.job_execution && result.job_execution.id;
		  if (!rerunID) throw new Error('Rerun response did not include an execution identifier');
		  window.location.assign('/declarative-preview/jobs/' + encodeURIComponent(rerunID));
		}
        else throw new Error('Command is not implemented by the web proof renderer: ' + action.command);
      };
      if (action.on === 'activate') {
        element.tabIndex = element.tabIndex >= 0 ? element.tabIndex : 0;
        element.setAttribute('role', element.tagName === 'BUTTON' ? 'button' : 'link');
        element.addEventListener('click', event => {
          if (element.tagName === 'BUTTON') event.stopPropagation();
          invoke(data).catch(error => window.alert(error.message || String(error)));
        });
        element.addEventListener('keydown', event => {
          if (event.key === 'Enter' || event.key === ' ') { event.preventDefault(); invoke(data).catch(error => window.alert(error.message || String(error))); }
        });
      } else if (action.on === 'change') {
        element.addEventListener('change', () => {
          const selected = element.options && element.selectedIndex >= 0 ? element.options[element.selectedIndex] : null;
          const actionData = element.tagName === 'INPUT'
            ? Object.assign({}, data, {input: {value: element.value}})
            : Object.assign({}, data, {selection: {value: element.value, label: selected ? selected.textContent : element.value}});
          invoke(actionData).catch(error => window.alert(error.message || String(error)));
        });
      }
    });
  }

  function renderNode(rawNode, data) {
    const node = withWebOverride(rawNode);
    if (node.hidden) return document.createDocumentFragment();
    if (node.visible) {
      const equal = String(resolve(data, node.visible.binding)) === String(node.visible.equals || 'true');
      if (node.visible.not ? equal : !equal) return document.createDocumentFragment();
    }
    if (node.repeat && node.component !== 'scroller') {
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
    const tone = style.toneBinding ? semanticTone(resolve(data, style.toneBinding)) : style.tone;
    if (tone) element.classList.add('dsl-' + tone);
    if (style.emphasis) element.classList.add('dsl-' + style.emphasis);
    if (style.truncate) element.classList.add('dsl-truncate');
    applyLayout(element, node.layout);
    if (node.component === 'disclosure') {
      const summary = document.createElement('summary');
	  if (style.role === 'execution-row' && node.image) {
	    const image = document.createElement('img');
	    image.className = 'dsl-execution-row-image';
	    image.src = node.image.asset === 'ciwi-logo' ? '/ciwi-logo.png' : node.image.asset;
	    image.alt = node.image.description || '';
	    summary.appendChild(image);
	    const status = document.createElement('span');
	    status.className = 'dsl-execution-row-status';
	    status.textContent = '●';
	    summary.appendChild(status);
	    const label = document.createElement('span');
	    label.textContent = renderText(node.text, data) || 'Details';
	    summary.appendChild(label);
	  } else {
	    summary.textContent = renderText(node.text, data) || 'Details';
      }
      element.appendChild(summary);
      if (node.disclosure) {
        const stateKey = node.disclosure.stateKey ? renderText({template: node.disclosure.stateKey}, data) : '';
        if (stateKey) {
          element.dataset.disclosureKey = stateKey;
          element.open = Object.prototype.hasOwnProperty.call(disclosureStates, stateKey)
            ? !!disclosureStates[stateKey]
            : !!node.disclosure.defaultExpanded;
          element.addEventListener('toggle', () => {
            disclosureStates[stateKey] = element.open;
            saveDisclosureStates();
          });
        } else {
          element.open = !!node.disclosure.defaultExpanded;
        }
      }
    } else if (node.component === 'image' && node.image) {
      element.src = node.image.asset === 'ciwi-logo' ? '/ciwi-logo.png' : node.image.asset;
      element.alt = node.image.description || '';
    } else if (node.component === 'select' && node.select) {
      const options = resolve(data, node.select.options);
      const current = String(resolve(data, node.select.value));
      (Array.isArray(options) ? options : []).forEach(item => {
        const optionData = Object.assign({}, data, {[node.select.as]: item});
        const option = document.createElement('option');
        option.value = String(resolve(optionData, node.select.optionValue));
        option.textContent = String(resolve(optionData, node.select.optionLabel));
        option.selected = option.value === current;
        element.appendChild(option);
      });
    } else if (node.component === 'input' && node.input) {
      element.type = 'text';
      element.value = String(resolve(data, node.input.value) ?? '');
      element.placeholder = node.input.placeholder || '';
    } else if (node.text) {
      element.textContent = renderText(node.text, data);
    }
    if (node.component === 'button' && node.icon) {
      const icon = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
      icon.classList.add('dsl-icon');
      icon.setAttribute('aria-hidden', 'true');
      const use = document.createElementNS('http://www.w3.org/2000/svg', 'use');
      use.setAttribute('href', '/ui/icons.svg#icon-' + node.icon);
      icon.appendChild(use);
      element.prepend(icon);
    }
    bindActions(element, node.actions, data);
    if (node.component === 'scroller' && node.repeat) {
      const list = resolve(data, node.repeat.source);
      (Array.isArray(list) ? list : []).forEach(item => {
        const itemData = Object.assign({}, data, {[node.repeat.as]: item});
        (node.children || []).forEach(child => element.appendChild(renderNode(child, itemData)));
      });
    } else {
      (node.children || []).forEach(child => element.appendChild(renderNode(child, data)));
    }
    return element;
  }

  function renderCurrent() {
    if (!currentDocument || !currentData) return;
    root.replaceChildren(renderNode(currentDocument.screen.root, currentData));
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

  function updateOutputSearch(view, direction) {
    const matches = outputMatchRanges(view.output, view.output_search);
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

  function selectBrowserOutputMatch(view) {
    const target = document.getElementById('job-output-text');
    const matches = outputMatchRanges(view.output, view.output_search);
    const match = matches[Number(view.output_match_index || 0)];
    if (!target || !target.firstChild || !match) return;
    const selection = window.getSelection();
    const range = document.createRange();
    range.setStart(target.firstChild, match[0]);
    range.setEnd(target.firstChild, match[1]);
    selection.removeAllRanges();
    selection.addRange(range);
  }

  async function watchJobOutput(jobID, generation) {
    let afterEventID = 0;
    let output = '';
    while (generation === outputWatchGeneration) {
      const response = await fetch('/api/v1/views/jobs/' + encodeURIComponent(jobID) + '/output?after_event_id=' + String(afterEventID));
      if (!response.ok) throw new Error(await response.text());
      const batch = await response.json();
      (Array.isArray(batch.lines) ? batch.lines : []).forEach(line => { output += String(line.text || ''); });
      if (output.length > maxOutputCharacters) {
        output = '[ciwi: earlier output omitted]\n' + output.slice(output.length - maxOutputCharacters);
      }
      const outputElement = document.getElementById('job-output-text');
      if (outputElement) outputElement.textContent = output;
      if (currentData && currentData.jobDetails) {
        currentData.jobDetails.output = output;
        updateOutputSearch(currentData.jobDetails, 0);
        if (outputElement && currentData.jobDetails.output_tailing) outputElement.scrollTop = outputElement.scrollHeight;
      }
      const emptyElement = document.getElementById('job-output-empty');
      if (emptyElement) emptyElement.hidden = output !== '';
      const nextEventID = Number(batch.next_event_id || afterEventID);
      if (Number.isFinite(nextEventID) && nextEventID >= afterEventID) afterEventID = nextEventID;
      if (batch.terminal && !batch.has_more) return;
      if (!batch.has_more) await new Promise(resolve => window.setTimeout(resolve, 500));
    }
  }

  async function refresh() {
    const generation = ++outputWatchGeneration;
    try {
      const projectMatch = window.location.pathname.match(/^\/declarative-preview\/projects\/(\d+)\/?$/);
      const jobMatch = window.location.pathname.match(/^\/declarative-preview\/jobs\/([^/]+)\/?$/);
      const settingsMatch = window.location.pathname.match(/^\/declarative-preview\/settings\/?$/);
	  const runOptionsURL = runOptionsViewURL('', '');
	  const runOptionsMatch = runOptionsURL !== '';
	  const screenName = runOptionsMatch ? 'run-options' : (projectMatch ? 'project-details' : (jobMatch ? 'job-details' : (settingsMatch ? 'settings' : 'front-page')));
      const viewURL = projectMatch
        ? '/api/v1/views/projects/' + encodeURIComponent(projectMatch[1])
		: (jobMatch ? '/api/v1/views/jobs/' + encodeURIComponent(jobMatch[1]) : (settingsMatch ? '/api/v1/server-info' : (runOptionsMatch ? runOptionsURL : '/api/v1/views/front-page')));
	  const bindingRoot = runOptionsMatch ? 'runOptions' : (projectMatch ? 'projectDetails' : (jobMatch ? 'jobDetails' : (settingsMatch ? 'settings' : 'frontPage')));
      const [screenResponse, themeResponse, viewResponse] = await Promise.all([
        fetch('/ui/contracts/screens/' + screenName + '.json'),
        fetch('/ui/contracts/themes.json'),
        fetch(viewURL),
      ]);
      if (!screenResponse.ok || !themeResponse.ok || !viewResponse.ok) throw new Error('Could not load declarative view data');
      const [documentContract, themes, responseView] = await Promise.all([screenResponse.json(), themeResponse.json(), viewResponse.json()]);
      applyContractTheme(themes);
      let view = responseView;
	  if (!projectMatch && !jobMatch && !settingsMatch && !runOptionsMatch) {
        decorateExecutionCards(view.queued_executions, true);
        decorateExecutionCards(view.history_executions, false);
      }
      if (settingsMatch) {
        const selectedTheme = ciwiStoredTheme();
        const themeOptions = themes.map(theme => ({
          name: theme.metadata.name,
          title: theme.metadata.title || theme.metadata.name,
          description: theme.metadata.description || '',
        }));
        const selected = themeOptions.find(theme => theme.name === selectedTheme);
        view = {server: responseView, themes: themeOptions, selected_theme: selectedTheme, selected_theme_description: selected ? selected.description : ''};
      }
      if (jobMatch) {
        view.output = view.output || '';
        view.output_search = '';
        view.output_search_count = '0/0';
        view.output_match_index = 0;
        view.output_tailing = false;
        view.tailing_label = 'Tailing: Off';
        view.selected_timeline_item = Array.isArray(view.timeline) && view.timeline.length
          ? view.timeline[0]
          : {id:'', title:'No execution steps reported', description:'', status:'', status_label:'', duration:'', exit_code:'', error:''};
      }
      currentDocument = documentContract;
      currentData = { [bindingRoot]: view };
      renderCurrent();
      if (jobMatch) {
        watchJobOutput(jobMatch[1], generation).catch(error => {
          if (generation !== outputWatchGeneration) return;
          const outputElement = document.getElementById('job-output-text');
          if (outputElement) outputElement.textContent = 'Output stream failed: ' + (error.message || String(error));
        });
      }
    } catch (error) {
      const message = document.createElement('div');
      message.className = 'dsl-error';
      message.textContent = error.message || String(error);
      root.replaceChildren(message);
    }
  }

  refresh();
})();
