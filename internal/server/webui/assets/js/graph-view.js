(function () {
  'use strict';

  window.ciwiCreateGraphViewRenderer = function ({
    resolve, renderText, icon: declarativeIcon, bindActions, renderNode,
    childRenderContext, rendererKeyPart, annotateRendererElement,
    disposeRenderedNode, graphRuntimeStates, viewStates, saveViewStates,
  }) {
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
	const seen = new Set();
	nodes.forEach(node => {
	  if (!node.id.trim()) throw new Error('Empty graph node key');
	  if (seen.has(node.id)) throw new Error('Duplicate graph node key "' + node.id + '"');
	  seen.add(node.id);
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

    function renderDefinitionGraph(node, data, selection, context, viewportState) {
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
		selection.remember(selection.value);
	}
      const layout = layoutDefinitionGraph(graphNodes);
      const wrapper = document.createElement('div');
      wrapper.className = 'dsl-definition-graph';
      const toolbar = document.createElement('div');
      toolbar.className = 'dsl-definition-graph-toolbar';
      const viewport = document.createElement('div');
      viewport.className = 'dsl-definition-graph-viewport';
	viewport.dataset.ciwiGraphViewportKey = context.identity;
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
			selection.remember(graphNode.id);
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
	let mountedScaler = scaler;
	let mountedStage = stage;
      let scale = Number(viewportState.scale || 1);
      const clamp = value => Math.min(1.5, Math.max(0.45, value));
      const scaleLabel = document.createElement('span');
      scaleLabel.className = 'dsl-definition-graph-scale';
      const applyScale = next => {
        scale = clamp(next);
	  viewportState.scale = scale;
	  mountedScaler.style.width = Math.round(layout.width * scale) + 'px';
	  mountedScaler.style.height = Math.round(layout.height * scale) + 'px';
	  mountedStage.style.transform = 'scale(' + scale + ')';
        scaleLabel.textContent = Math.round(scale * 100) + '%';
      };
	applyScale(scale);
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
	let mountedViewport = viewport;
      const fitViewport = target => applyScale(Math.min(
	  (Math.max(1, target.clientWidth) - 32) / layout.width,
	  388 / layout.height,
      ));
	const fit = () => fitViewport(mountedViewport);
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
	let layoutFrame = 0;
	const disposeViewport = () => {
	  if (layoutFrame) cancelAnimationFrame(layoutFrame);
	  layoutFrame = 0;
	};
	const mountViewport = (target, adoptedScaler = mountedScaler, adoptedStage = mountedStage) => {
	  disposeViewport();
	  mountedViewport = target;
	  mountedScaler = adoptedScaler;
	  mountedStage = adoptedStage;
	  target.__ciwiDispose = disposeViewport;
	  target.__ciwiAdoptGraphViewport = mountViewport;
	  if (!viewportState.initialized) {
		layoutFrame = requestAnimationFrame(() => {
		  layoutFrame = 0;
		  fitViewport(target);
		  viewportState.initialized = true;
		});
	  }
	};
	mountViewport(viewport);
      return wrapper;
    }

    function renderGraphView(element, node, data, context) {
      const stateKey = renderText({template: node.graphView.stateKey}, data);
	const runtimeKey = context.session.screenName + ':' + stateKey;
	let runtime = graphRuntimeStates.get(runtimeKey);
	if (!runtime) {
	  runtime = {selectedID: '', scale: 1, initialized: false};
	  graphRuntimeStates.set(runtimeKey, runtime);
	}
	element.__ciwiStatefulContents = 'graph-view';
	element.__ciwiGraphRuntime = runtime;
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
	let selectedID = runtime.selectedID;
      const renderBody = () => {
	  disposeRenderedNode(body);
        body.replaceChildren();
	  if (mode === 'graph') body.appendChild(renderDefinitionGraph(node, data, {
		value: selectedID,
		remember: id => {
		  selectedID = id;
		  runtime.selectedID = id;
		},
		onChange: id => {
			selectedID = id;
			runtime.selectedID = id;
			renderBody();
		},
	  }, context, runtime));
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

    return {render: renderGraphView};
  };
})();
