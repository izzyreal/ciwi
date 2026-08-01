package webui

const uiGraphCSS = `
    .structure-heading { display:flex; align-items:center; justify-content:space-between; gap:12px; margin-bottom:10px; flex-wrap:wrap; }
    .structure-view-toggle { display:flex; gap:6px; }
    .structure-view-toggle button.active { background:var(--accent-soft, var(--surface-hover)); border-color:var(--accent); color:var(--accent-strong); }
    .project-graph-toolbar { display:flex; align-items:center; justify-content:space-between; gap:10px; flex-wrap:wrap; margin-bottom:10px; }
    .project-graph-toolbar-group { display:flex; align-items:center; gap:6px; flex-wrap:wrap; }
    .project-graph-select { min-height:34px; padding:5px 30px 5px 9px; border:1px solid var(--line); border-radius:7px; background:var(--input-bg); color:var(--ink); font:inherit; }
    .project-graph-viewport { position:relative; min-height:260px; height:420px; overflow:auto; border:1px solid var(--line); border-radius:10px; background:var(--graph-background); }
    .project-graph-stage { position:relative; min-width:100%; min-height:100%; }
    .project-graph-content { position:absolute; left:0; top:0; transform-origin:top left; }
    .project-graph-edges { position:absolute; inset:0; overflow:visible; pointer-events:none; }
    .project-graph-edge { fill:none; stroke:var(--graph-edge); stroke-width:2; }
    .project-graph-node { position:absolute; box-sizing:border-box; width:210px; height:76px; border:1px solid var(--graph-node-border); border-radius:9px; background:var(--graph-node-bg); color:var(--ink); box-shadow:0 1px 2px var(--shadow); overflow:hidden; }
    .project-graph-node:hover { border-color:var(--accent); filter:brightness(1.035); }
    .project-graph-node.graph-status-running,.project-graph-node.graph-status-leased { border-color:var(--graph-running-border); background:var(--graph-running-bg); }
    .project-graph-node.graph-status-succeeded { border-color:var(--graph-succeeded-border); background:var(--graph-succeeded-bg); }
    .project-graph-node.graph-status-failed { border-color:var(--graph-failed-border); background:var(--graph-failed-bg); }
    .project-graph-node.graph-status-waiting,.project-graph-node.graph-status-queued { border-color:var(--graph-waiting-border); background:var(--graph-waiting-bg); }
    .project-graph-node.graph-status-not-reached,.project-graph-node.graph-status-unknown { opacity:.68; }
    .project-graph-node.selected { border-color:var(--graph-selected-border); box-shadow:0 1px 2px var(--shadow), inset 0 0 0 1px var(--graph-selected-inset); }
    .project-graph-node-select { position:absolute; inset:0; width:100%; height:100%; padding:10px 12px; border:0; border-radius:0; background:transparent; color:inherit; text-align:left; cursor:pointer; -webkit-user-select:text; user-select:text; }
    .project-graph-node.has-play .project-graph-node-select { padding-right:46px; }
    .project-graph-node-select:focus-visible,.project-graph-node-play:focus-visible { outline:3px solid var(--focus-ring); outline-offset:-3px; }
    .project-graph-node-play { position:absolute; z-index:2; right:8px; top:50%; transform:translateY(-50%); width:31px; height:31px; padding:0; display:flex; align-items:center; justify-content:center; border:1px solid var(--graph-node-border); border-radius:999px; background:var(--surface-soft); color:var(--accent-strong); font-size:15px; line-height:1; cursor:pointer; }
    .project-graph-node-play:hover { background:var(--surface-hover); border-color:var(--accent); }
    .project-graph-node-play:disabled { opacity:.55; cursor:wait; }
    .project-graph-node-title { display:block; font-family:ui-monospace,SFMono-Regular,Menlo,Monaco,Consolas,"Liberation Mono","Courier New",monospace; font-size:14px; font-weight:700; line-height:1.25; overflow:hidden; text-overflow:ellipsis; white-space:nowrap; }
    .project-graph-node-meta { display:block; margin-top:7px; color:var(--muted); font-size:12px; line-height:1.25; overflow:hidden; text-overflow:ellipsis; white-space:nowrap; }
    .project-graph-warning { margin:0 0 8px; padding:7px 9px; border:1px solid var(--warn-line); border-radius:7px; background:var(--warn-bg); color:var(--warn); font-size:12px; }
    .project-graph-empty { display:flex; align-items:center; justify-content:center; min-height:220px; color:var(--muted); }
    .project-graph-details { margin-top:12px; padding-top:12px; border-top:1px solid var(--line); }
    .project-graph-detail-head { display:flex; align-items:flex-start; justify-content:space-between; gap:10px; flex-wrap:wrap; margin-bottom:10px; }
    .project-graph-detail-title { min-width:0; }
    .project-graph-detail-title h3 { margin:0 0 4px; }
    .project-graph-detail-actions { display:flex; align-items:center; justify-content:flex-end; gap:7px; flex-wrap:wrap; }
    .project-job-graph-layout { display:grid; grid-template-columns:minmax(0,3fr) minmax(280px,2fr); gap:12px; align-items:start; }
    .project-job-graph .project-graph-viewport { height:300px; min-height:220px; }
    .project-job-detail { min-height:220px; padding:10px; border:1px solid var(--line); border-radius:9px; background:var(--surface-subtle); }
    .project-job-detail h4 { margin:0 0 8px; }
    .project-job-detail-meta { display:flex; flex-direction:column; gap:5px; margin-bottom:10px; font-size:12px; overflow-wrap:anywhere; }
    .project-job-detail-actions { display:flex; gap:7px; flex-wrap:wrap; }
    .project-job-detail .matrix-list { width:100%; }
    .project-step-sequence { margin-top:12px; padding-top:12px; border-top:1px solid var(--line); }
    .project-step-sequence h4 { margin:0 0 8px; }
    .project-step-track { display:flex; align-items:stretch; gap:0; overflow:auto; padding:5px 2px 10px; }
    .project-step-node { position:relative; flex:0 0 180px; min-height:72px; padding:9px 10px; border:1px solid var(--graph-node-border); border-radius:8px; background:var(--graph-node-bg); color:var(--ink); text-align:left; cursor:pointer; -webkit-user-select:text; user-select:text; }
    .project-step-node.selected { border-color:var(--graph-selected-border); background:var(--surface-hover); box-shadow:inset 0 0 0 1px var(--graph-selected-inset); }
    .project-step-node-title { display:block; font-weight:700; overflow:hidden; text-overflow:ellipsis; white-space:nowrap; }
    .project-step-node-meta { display:block; margin-top:6px; color:var(--muted); font-size:11px; }
    .project-step-arrow { flex:0 0 30px; display:flex; align-items:center; justify-content:center; color:var(--graph-edge); font-size:20px; }
    .project-step-detail { margin-top:8px; padding:9px 10px; border:1px solid var(--line); border-radius:8px; background:var(--surface-subtle); }
    .project-step-detail pre { margin:7px 0 0; max-height:180px; overflow:auto; white-space:pre-wrap; overflow-wrap:anywhere; }
    .project-matrix-chooser { display:flex; flex-direction:column; gap:8px; max-height:55vh; overflow:auto; }
    .project-graph-zoom-label { min-width:42px; text-align:center; color:var(--muted); font-size:12px; }
    @media (max-width:760px) {
      .project-graph-viewport { height:360px; }
      .project-job-graph-layout { grid-template-columns:1fr; }
      .project-job-graph .project-graph-viewport { height:270px; }
    }
`

const uiProjectGraphCSS = uiGraphCSS

const uiGraphJS = `
    const structureViewStorageKey = 'ciwi.project.structure.view.v1';
    const projectGraphMinScale = 0.35;
    const projectGraphMaxScale = 1.75;
    const projectGraphState = {
      project: null,
      chainID: 'all',
      pipelineID: '',
      jobID: '',
      stepIndex: 0,
      scale: 1,
      fitOnRender: true,
    };

    function readProjectGraphStorage(key, fallback) {
      try {
        const value = localStorage.getItem(key);
        return value === null ? fallback : value;
      } catch (_) {
        return fallback;
      }
    }

    function writeProjectGraphStorage(key, value) {
      try {
        localStorage.setItem(key, value);
      } catch (_) {
        // Browser storage may be unavailable; the graph remains fully usable.
      }
    }

    function projectGraphChainStorageKey(projectID) {
      return 'ciwi.project.graph.chain.v1.' + String(projectID || '');
    }

    function projectGraphNodeMeta(count, dependencyCount, noun) {
      const itemWord = count === 1 ? noun : noun + 's';
      const depWord = dependencyCount === 1 ? 'dependency' : 'dependencies';
      return count + ' ' + itemWord + ' · ' + dependencyCount + ' ' + depWord;
    }

    function buildProjectDAGLayout(nodes) {
      const nodeWidth = 210;
      const nodeHeight = 76;
      const gapX = 92;
      const gapY = 28;
      const padding = 28;
      const byID = new Map();
      const order = new Map();
      const warnings = [];
      nodes.forEach((node, index) => {
        byID.set(node.id, node);
        order.set(node.id, index);
      });

      const incoming = new Map();
      const outgoing = new Map();
      const rank = new Map();
      nodes.forEach(node => {
        incoming.set(node.id, 0);
        outgoing.set(node.id, []);
        rank.set(node.id, 0);
      });
      nodes.forEach(node => {
        (node.dependsOn || []).forEach(dep => {
          if (!byID.has(dep)) {
            warnings.push('Dependency ' + dep + ' referenced by ' + node.id + ' is not present.');
            return;
          }
          incoming.set(node.id, incoming.get(node.id) + 1);
          outgoing.get(dep).push(node.id);
        });
      });

      const queue = nodes.filter(node => incoming.get(node.id) === 0).map(node => node.id);
      queue.sort((a, b) => order.get(a) - order.get(b));
      const visited = [];
      while (queue.length > 0) {
        const id = queue.shift();
        visited.push(id);
        (outgoing.get(id) || []).forEach(child => {
          rank.set(child, Math.max(rank.get(child), rank.get(id) + 1));
          incoming.set(child, incoming.get(child) - 1);
          if (incoming.get(child) === 0) {
            queue.push(child);
            queue.sort((a, b) => order.get(a) - order.get(b));
          }
        });
      }

      if (visited.length !== nodes.length) {
        warnings.push('The dependency data contains a cycle; cyclic nodes use a fallback column.');
        const fallbackRank = Math.max(0, ...Array.from(rank.values()));
        nodes.forEach(node => {
          if (!visited.includes(node.id)) rank.set(node.id, fallbackRank + 1);
        });
      }

      const columns = new Map();
      nodes.forEach(node => {
        const column = rank.get(node.id) || 0;
        if (!columns.has(column)) columns.set(column, []);
        columns.get(column).push(node);
      });
      columns.forEach(column => column.sort((a, b) => order.get(a.id) - order.get(b.id)));
      const maxColumnSize = Math.max(1, ...Array.from(columns.values()).map(column => column.length));
      const contentHeight = padding * 2 + maxColumnSize * nodeHeight + Math.max(0, maxColumnSize - 1) * gapY;
      const maxRank = Math.max(0, ...Array.from(columns.keys()));
      const contentWidth = padding * 2 + (maxRank + 1) * nodeWidth + maxRank * gapX;
      const positions = new Map();
      columns.forEach((column, columnIndex) => {
        const columnHeight = column.length * nodeHeight + Math.max(0, column.length - 1) * gapY;
        const startY = padding + Math.max(0, (contentHeight - padding * 2 - columnHeight) / 2);
        column.forEach((node, rowIndex) => {
          positions.set(node.id, {
            x: padding + columnIndex * (nodeWidth + gapX),
            y: startY + rowIndex * (nodeHeight + gapY),
          });
        });
      });
      return { nodeWidth, nodeHeight, contentWidth, contentHeight, positions, warnings, byID };
    }

    function renderProjectDAG(viewport, nodes, options) {
      viewport.innerHTML = '';
      if (!nodes.length) {
        const empty = document.createElement('div');
        empty.className = 'project-graph-empty';
        empty.textContent = (options && options.emptyText) || 'Nothing to show';
        viewport.appendChild(empty);
        return null;
      }
      const layout = buildProjectDAGLayout(nodes);
      const scale = Math.max(projectGraphMinScale, Math.min(projectGraphMaxScale, Number(options.scale || 1)));
      const stage = document.createElement('div');
      stage.className = 'project-graph-stage';
      stage.style.width = Math.ceil(layout.contentWidth * scale) + 'px';
      stage.style.height = Math.ceil(layout.contentHeight * scale) + 'px';
      stage.dataset.contentWidth = String(layout.contentWidth);
      stage.dataset.contentHeight = String(layout.contentHeight);

      const content = document.createElement('div');
      content.className = 'project-graph-content';
      content.style.width = layout.contentWidth + 'px';
      content.style.height = layout.contentHeight + 'px';
      content.style.transform = 'scale(' + scale + ')';

      const svgNS = 'http://www.w3.org/2000/svg';
      const svg = document.createElementNS(svgNS, 'svg');
      svg.classList.add('project-graph-edges');
      svg.setAttribute('width', String(layout.contentWidth));
      svg.setAttribute('height', String(layout.contentHeight));
      svg.setAttribute('aria-hidden', 'true');
      const markerID = 'projectGraphArrow' + Math.random().toString(36).slice(2);
      const defs = document.createElementNS(svgNS, 'defs');
      const marker = document.createElementNS(svgNS, 'marker');
      marker.setAttribute('id', markerID);
      marker.setAttribute('viewBox', '0 0 10 10');
      marker.setAttribute('refX', '9');
      marker.setAttribute('refY', '5');
      marker.setAttribute('markerWidth', '6');
      marker.setAttribute('markerHeight', '6');
      marker.setAttribute('orient', 'auto-start-reverse');
      const arrow = document.createElementNS(svgNS, 'path');
      arrow.setAttribute('d', 'M 0 0 L 10 5 L 0 10 z');
      arrow.setAttribute('fill', 'var(--graph-edge)');
      marker.appendChild(arrow);
      defs.appendChild(marker);
      svg.appendChild(defs);

      nodes.forEach(node => {
        const target = layout.positions.get(node.id);
        (node.dependsOn || []).forEach(depID => {
          const source = layout.positions.get(depID);
          if (!source || !target) return;
          const x1 = source.x + layout.nodeWidth;
          const y1 = source.y + layout.nodeHeight / 2;
          const x2 = target.x;
          const y2 = target.y + layout.nodeHeight / 2;
          const bend = Math.max(34, (x2 - x1) * 0.48);
          const path = document.createElementNS(svgNS, 'path');
          path.classList.add('project-graph-edge');
          path.setAttribute('d', 'M ' + x1 + ' ' + y1 + ' C ' + (x1 + bend) + ' ' + y1 + ', ' + (x2 - bend) + ' ' + y2 + ', ' + x2 + ' ' + y2);
          path.setAttribute('marker-end', 'url(#' + markerID + ')');
          svg.appendChild(path);
        });
      });
      content.appendChild(svg);

      nodes.forEach(node => {
        const position = layout.positions.get(node.id);
        const wrapper = document.createElement('div');
        const runnable = typeof options.onRun === 'function' && node.runnable !== false;
        const statusClass = String(node.status || '').trim().toLowerCase().replace(/[^a-z0-9_-]+/g, '-');
        wrapper.className = 'project-graph-node' + (node.id === options.selectedID ? ' selected' : '') + (runnable ? ' has-play' : '') + (statusClass ? (' graph-status-' + statusClass) : '');
        wrapper.dataset.nodeId = node.id;
        wrapper.style.left = position.x + 'px';
        wrapper.style.top = position.y + 'px';
        const button = document.createElement('button');
        button.type = 'button';
        button.className = 'project-graph-node-select';
        button.setAttribute('aria-pressed', node.id === options.selectedID ? 'true' : 'false');
        button.setAttribute('aria-label', node.label + '. ' + node.meta);
        const title = document.createElement('span');
        title.className = 'project-graph-node-title';
        title.textContent = node.label;
        title.setAttribute('data-ciwi-overflow-text', node.label);
        const meta = document.createElement('span');
        meta.className = 'project-graph-node-meta';
        meta.textContent = node.meta;
        button.appendChild(title);
        button.appendChild(meta);
        button.onclick = () => {
          if (ciwiElementContainsTextSelection(button)) return;
          options.onSelect(node.id);
        };
        wrapper.appendChild(button);
        if (runnable) {
          const play = document.createElement('button');
          play.type = 'button';
          play.className = 'project-graph-node-play';
          play.classList.add('ciwi-icon-only');
          play.appendChild(ciwiIconElement('player-play'));
          play.setAttribute('aria-label', node.runLabel || ('Run ' + node.label));
          play.title = node.runTitle || node.runLabel || ('Run ' + node.label);
          play.onclick = async event => {
            event.stopPropagation();
            play.disabled = true;
            try {
              await options.onRun(node.id, event, play);
            } finally {
              play.disabled = false;
            }
          };
          wrapper.appendChild(play);
        }
        content.appendChild(wrapper);
      });
      stage.appendChild(content);
      viewport.appendChild(stage);
      return layout;
    }

    function setProjectDAGSelectedNode(viewport, selectedID) {
      if (!viewport) return;
      viewport.querySelectorAll('.project-graph-node').forEach(node => {
        const selected = String(node.dataset.nodeId || '') === String(selectedID || '');
        node.classList.toggle('selected', selected);
        const button = node.querySelector('.project-graph-node-select');
        if (button) button.setAttribute('aria-pressed', selected ? 'true' : 'false');
      });
    }

    function applyProjectGraphScale(scale) {
      const viewport = document.getElementById('projectPipelineGraphViewport');
      if (!viewport) return;
      const stage = viewport.querySelector('.project-graph-stage');
      const content = viewport.querySelector('.project-graph-content');
      if (!stage || !content) return;
      const next = Math.max(projectGraphMinScale, Math.min(projectGraphMaxScale, scale));
      projectGraphState.scale = next;
      const width = Number(stage.dataset.contentWidth || 0);
      const height = Number(stage.dataset.contentHeight || 0);
      stage.style.width = Math.ceil(width * next) + 'px';
      stage.style.height = Math.ceil(height * next) + 'px';
      content.style.left = Math.max(0, Math.floor((viewport.clientWidth - width * next) / 2)) + 'px';
      content.style.transform = 'scale(' + next + ')';
      const label = document.getElementById('projectGraphZoomLabel');
      if (label) label.textContent = Math.round(next * 100) + '%';
    }

    function fitProjectGraph() {
      const viewport = document.getElementById('projectPipelineGraphViewport');
      if (!viewport) return;
      const stage = viewport.querySelector('.project-graph-stage');
      if (!stage) return;
      const width = Number(stage.dataset.contentWidth || 0);
      const height = Number(stage.dataset.contentHeight || 0);
      if (!width || !height) return;
      const widthScale = Math.max(projectGraphMinScale, Math.min(projectGraphMaxScale, (viewport.clientWidth - 18) / width));
      const fittedHeight = Math.min(520, Math.max(148, Math.ceil(height * widthScale) + 18));
      viewport.style.height = fittedHeight + 'px';
      const scale = Math.min(widthScale, (viewport.clientHeight - 18) / height);
      applyProjectGraphScale(scale);
      viewport.scrollLeft = 0;
      viewport.scrollTop = 0;
    }

    function setProjectStructureView(view) {
      const next = view === 'list' ? 'list' : 'graph';
      const graph = document.getElementById('structureGraph');
      const list = document.getElementById('structure');
      const graphBtn = document.getElementById('structureGraphBtn');
      const listBtn = document.getElementById('structureListBtn');
      if (!graph || !list) return;
      graph.hidden = next !== 'graph';
      list.hidden = next !== 'list';
      if (graphBtn) {
        graphBtn.classList.toggle('active', next === 'graph');
        graphBtn.setAttribute('aria-pressed', next === 'graph' ? 'true' : 'false');
      }
      if (listBtn) {
        listBtn.classList.toggle('active', next === 'list');
        listBtn.setAttribute('aria-pressed', next === 'list' ? 'true' : 'false');
      }
      writeProjectGraphStorage(structureViewStorageKey, next);
      if (next === 'graph' && projectGraphState.project) {
        renderProjectGraph();
        if (projectGraphState.fitOnRender) {
          projectGraphState.fitOnRender = false;
          requestAnimationFrame(fitProjectGraph);
        }
      }
    }

    function initializeProjectGraph(project) {
      projectGraphState.project = project;
      projectGraphState.scale = 1;
      projectGraphState.fitOnRender = true;
      const chains = Array.isArray(project.pipeline_chains) ? project.pipeline_chains : [];
      const storedChain = readProjectGraphStorage(projectGraphChainStorageKey(project.id), 'all');
      projectGraphState.chainID = chains.some(chain => String(chain.id) === storedChain) ? storedChain : 'all';
      projectGraphState.pipelineID = '';
      projectGraphState.jobID = '';
      projectGraphState.stepIndex = 0;
      const toggle = document.getElementById('structureViewToggle');
      if (toggle) toggle.hidden = false;
      const graphBtn = document.getElementById('structureGraphBtn');
      const listBtn = document.getElementById('structureListBtn');
      if (graphBtn) graphBtn.onclick = () => setProjectStructureView('graph');
      if (listBtn) listBtn.onclick = () => setProjectStructureView('list');
      const storedView = readProjectGraphStorage(structureViewStorageKey, 'graph');
      setProjectStructureView(storedView);
    }

    function filteredProjectGraphPipelines(project) {
      const pipelines = Array.isArray(project.pipelines) ? project.pipelines : [];
      if (projectGraphState.chainID === 'all') return pipelines;
      const chains = Array.isArray(project.pipeline_chains) ? project.pipeline_chains : [];
      const chain = chains.find(item => String(item.id) === projectGraphState.chainID);
      if (!chain) return pipelines;
      const included = new Set(Array.isArray(chain.pipelines) ? chain.pipelines : []);
      return pipelines.filter(pipeline => included.has(pipeline.pipeline_id));
    }

    function renderProjectGraph() {
      const host = document.getElementById('structureGraph');
      const project = projectGraphState.project;
      if (!host || !project) return;
      const previousViewport = document.getElementById('projectPipelineGraphViewport');
      const previousScrollLeft = previousViewport ? previousViewport.scrollLeft : 0;
      const previousScrollTop = previousViewport ? previousViewport.scrollTop : 0;
      destroyOverflowTooltips(host);
      host.innerHTML = '';
      const pipelines = filteredProjectGraphPipelines(project);
      const pipelineIDs = new Set(pipelines.map(pipeline => pipeline.pipeline_id));
      if (!pipelineIDs.has(projectGraphState.pipelineID)) {
        const root = pipelines.find(pipeline => !(pipeline.depends_on || []).some(dep => pipelineIDs.has(dep)));
        projectGraphState.pipelineID = root ? root.pipeline_id : (pipelines[0] ? pipelines[0].pipeline_id : '');
        projectGraphState.jobID = '';
        projectGraphState.stepIndex = 0;
      }

      const toolbar = document.createElement('div');
      toolbar.className = 'project-graph-toolbar';
      const filters = document.createElement('div');
      filters.className = 'project-graph-toolbar-group';
      const filterLabel = document.createElement('label');
      filterLabel.className = 'muted';
      filterLabel.setAttribute('for', 'projectGraphChainSelect');
      filterLabel.textContent = 'Show:';
      const select = document.createElement('select');
      select.id = 'projectGraphChainSelect';
      select.className = 'project-graph-select';
      const allOption = document.createElement('option');
      allOption.value = 'all';
      allOption.textContent = 'All Pipelines';
      select.appendChild(allOption);
      (project.pipeline_chains || []).forEach(chain => {
        const option = document.createElement('option');
        option.value = String(chain.id || '');
        option.textContent = pipelineChainDisplayName(chain);
        select.appendChild(option);
      });
      select.value = projectGraphState.chainID;
      select.onchange = () => {
        projectGraphState.chainID = String(select.value || 'all');
        projectGraphState.pipelineID = '';
        projectGraphState.jobID = '';
        projectGraphState.stepIndex = 0;
        projectGraphState.fitOnRender = true;
        writeProjectGraphStorage(projectGraphChainStorageKey(project.id), projectGraphState.chainID);
        renderProjectGraph();
        requestAnimationFrame(fitProjectGraph);
      };
      filters.appendChild(filterLabel);
      filters.appendChild(select);
      toolbar.appendChild(filters);

      const zoom = document.createElement('div');
      zoom.className = 'project-graph-toolbar-group';
      const fitBtn = document.createElement('button');
      fitBtn.type = 'button';
      fitBtn.className = 'secondary';
      fitBtn.textContent = 'Fit';
      fitBtn.onclick = fitProjectGraph;
      const resetBtn = document.createElement('button');
      resetBtn.type = 'button';
      resetBtn.className = 'secondary';
      resetBtn.textContent = 'Reset';
      resetBtn.onclick = () => {
        applyProjectGraphScale(1);
        const viewport = document.getElementById('projectPipelineGraphViewport');
        if (viewport) {
          viewport.scrollLeft = 0;
          viewport.scrollTop = 0;
        }
      };
      const outBtn = document.createElement('button');
      outBtn.type = 'button';
      outBtn.className = 'secondary';
      outBtn.classList.add('ciwi-icon-only');
      outBtn.appendChild(ciwiIconElement('zoom-out'));
      outBtn.setAttribute('aria-label', 'Zoom out');
      outBtn.title = 'Zoom out';
      outBtn.onclick = () => applyProjectGraphScale(projectGraphState.scale - 0.1);
      const label = document.createElement('span');
      label.id = 'projectGraphZoomLabel';
      label.className = 'project-graph-zoom-label';
      label.textContent = Math.round(projectGraphState.scale * 100) + '%';
      const inBtn = document.createElement('button');
      inBtn.type = 'button';
      inBtn.className = 'secondary';
      inBtn.classList.add('ciwi-icon-only');
      inBtn.appendChild(ciwiIconElement('zoom-in'));
      inBtn.setAttribute('aria-label', 'Zoom in');
      inBtn.title = 'Zoom in';
      inBtn.onclick = () => applyProjectGraphScale(projectGraphState.scale + 0.1);
      zoom.appendChild(fitBtn);
      zoom.appendChild(resetBtn);
      zoom.appendChild(outBtn);
      zoom.appendChild(label);
      zoom.appendChild(inBtn);
      toolbar.appendChild(zoom);
      host.appendChild(toolbar);

      const warningHost = document.createElement('div');
      host.appendChild(warningHost);
      const viewport = document.createElement('div');
      viewport.id = 'projectPipelineGraphViewport';
      viewport.className = 'project-graph-viewport';
      host.appendChild(viewport);
      const nodes = pipelines.map(pipeline => ({
        id: pipeline.pipeline_id,
        label: pipeline.pipeline_id,
        dependsOn: (pipeline.depends_on || []).filter(dep => pipelineIDs.has(dep)),
        meta: projectGraphNodeMeta((pipeline.jobs || []).length, (pipeline.depends_on || []).filter(dep => pipelineIDs.has(dep)).length, 'job'),
        runnable: true,
        runLabel: 'Run pipeline ' + pipeline.pipeline_id,
        runTitle: 'Run this pipeline. Shift-click to choose source ref and agent.',
      }));
      const layout = renderProjectDAG(viewport, nodes, {
        scale: projectGraphState.scale,
        selectedID: projectGraphState.pipelineID,
        emptyText: 'No pipelines in this chain',
        onSelect: id => {
          projectGraphState.pipelineID = id;
          projectGraphState.jobID = '';
          projectGraphState.stepIndex = 0;
          renderProjectGraph();
        },
        onRun: async (id, event, button) => {
          const pipeline = pipelines.find(item => item.pipeline_id === id);
          if (pipeline) await runProjectPipeline(event, pipeline, button);
        },
      });
      if (layout && layout.warnings.length) {
        layout.warnings.forEach(message => {
          const warning = document.createElement('div');
          warning.className = 'project-graph-warning';
          warning.textContent = message;
          warningHost.appendChild(warning);
        });
      }
      renderProjectGraphDetails(host, pipelines.find(pipeline => pipeline.pipeline_id === projectGraphState.pipelineID));
      viewport.scrollLeft = previousScrollLeft;
      viewport.scrollTop = previousScrollTop;
      bindOverflowTooltips(host);
    }

    function renderProjectGraphDetails(host, pipeline) {
      if (!pipeline) return;
      const details = document.createElement('section');
      details.className = 'project-graph-details';
      details.setAttribute('aria-label', 'Selected pipeline details');
      const head = document.createElement('div');
      head.className = 'project-graph-detail-head';
      const title = document.createElement('div');
      title.className = 'project-graph-detail-title';
      const deps = (pipeline.depends_on || []).join(', ');
      title.innerHTML = '<h3>Pipeline: <code>' + escapeHtml(pipeline.pipeline_id || '') + '</code></h3>' +
        '<div class="muted">' + (deps ? 'depends_on: ' + escapeHtml(deps) : 'No pipeline dependencies') + '</div>';
      const actions = document.createElement('div');
      actions.className = 'project-graph-detail-actions';
      appendPipelineActionControls(actions, pipeline);
      head.appendChild(title);
      head.appendChild(actions);
      details.appendChild(head);

      const jobs = Array.isArray(pipeline.jobs) ? pipeline.jobs : [];
      if (!jobs.length) {
        const empty = document.createElement('div');
        empty.className = 'muted';
        empty.textContent = 'This pipeline has no jobs.';
        details.appendChild(empty);
        host.appendChild(details);
        return;
      }
      const jobIDs = new Set(jobs.map(job => job.id));
      if (!jobIDs.has(projectGraphState.jobID)) {
        const root = jobs.find(job => !(job.needs || []).some(dep => jobIDs.has(dep)));
        projectGraphState.jobID = root ? root.id : jobs[0].id;
      }
      const layout = document.createElement('div');
      layout.className = 'project-job-graph-layout';
      const graph = document.createElement('div');
      graph.className = 'project-job-graph';
      const viewport = document.createElement('div');
      viewport.className = 'project-graph-viewport';
      graph.appendChild(viewport);
      const jobNodes = jobs.map(job => ({
        id: job.id,
        label: job.id,
        dependsOn: (job.needs || []).filter(dep => jobIDs.has(dep)),
        meta: projectGraphNodeMeta((job.steps || []).length, (job.needs || []).filter(dep => jobIDs.has(dep)).length, 'step'),
        runnable: true,
        runLabel: (job.matrix_includes || []).length ? ('Choose matrix entry for ' + job.id) : ('Run job ' + job.id),
        runTitle: (job.matrix_includes || []).length ? 'Choose a matrix entry to run.' : 'Run this job. Shift-click to choose source ref and agent.',
      }));
      const jobLayout = renderProjectDAG(viewport, jobNodes, {
        scale: 1,
        selectedID: projectGraphState.jobID,
        emptyText: 'No jobs',
        onSelect: id => {
          projectGraphState.jobID = id;
          projectGraphState.stepIndex = 0;
          renderProjectGraph();
        },
        onRun: async (id, event, button) => {
          const job = jobs.find(item => item.id === id);
          if (!job) return;
          if ((job.matrix_includes || []).length) {
            openProjectMatrixRunChooser(pipeline, job);
            return;
          }
          await runProjectJobSelection(event, pipeline, { pipeline_job_id: job.id }, (job.id || 'job'), 'Run selection failed', 'Run Job With Source Ref', 'Run Job', button);
        },
      });
      if (jobLayout) {
        requestAnimationFrame(() => {
          const stage = viewport.querySelector('.project-graph-stage');
          const content = viewport.querySelector('.project-graph-content');
          if (!stage || !content) return;
          const scale = Math.min(1, (viewport.clientWidth - 16) / jobLayout.contentWidth, (viewport.clientHeight - 16) / jobLayout.contentHeight);
          stage.style.width = Math.ceil(jobLayout.contentWidth * scale) + 'px';
          stage.style.height = Math.ceil(jobLayout.contentHeight * scale) + 'px';
          content.style.transform = 'scale(' + scale + ')';
        });
      }
      layout.appendChild(graph);
      const selectedJob = jobs.find(job => job.id === projectGraphState.jobID);
      layout.appendChild(buildProjectGraphJobDetail(pipeline, selectedJob));
      details.appendChild(layout);
      renderProjectStepSequence(details, selectedJob);
      host.appendChild(details);
    }

    function buildProjectGraphJobDetail(pipeline, job) {
      const panel = document.createElement('div');
      panel.className = 'project-job-detail';
      if (!job) return panel;
      const runsOn = Object.entries(job.runs_on || {}).map(kv => kv[0] + '=' + kv[1]).join(', ') || 'any eligible agent';
      const tools = Object.entries(job.requires_tools || {}).map(kv => kv[0] + '=' + (kv[1] || '*')).join(', ') || 'none';
      const needs = (job.needs || []).join(', ') || 'none';
      const heading = document.createElement('h4');
      heading.innerHTML = 'Job: <code>' + escapeHtml(job.id || '') + '</code>';
      panel.appendChild(heading);
      const meta = document.createElement('div');
      meta.className = 'project-job-detail-meta muted';
      meta.innerHTML = '<div>needs: ' + escapeHtml(needs) + '</div>' +
        '<div>runs_on: ' + escapeHtml(runsOn) + '</div>' +
        '<div>requires.tools: ' + escapeHtml(tools) + '</div>' +
        '<div>timeout: ' + Number(job.timeout_seconds || 0) + 's · ' + (job.steps || []).length + ' step(s)</div>';
      panel.appendChild(meta);
      appendJobActionControls(panel, pipeline, job);
      return panel;
    }

    function projectConfiguredStepName(step, index, jobID) {
      const configured = String((step && step.name) || '').trim();
      if (configured) return configured;
      if (String((step && step.type) || '') === 'test') {
        const testName = String((step && step.test_name) || '').trim();
        return testName ? ('test ' + testName) : ('test ' + String(jobID || 'job') + '-' + String(index + 1));
      }
      return 'step ' + String(index + 1);
    }

    function renderProjectStepSequence(host, job) {
      const steps = Array.isArray(job && job.steps) ? job.steps : [];
      if (!steps.length) return;
      if (projectGraphState.stepIndex < 0 || projectGraphState.stepIndex >= steps.length) projectGraphState.stepIndex = 0;
      const section = document.createElement('section');
      section.className = 'project-step-sequence';
      const heading = document.createElement('h4');
      heading.textContent = 'Configured steps';
      section.appendChild(heading);
      const track = document.createElement('div');
      track.className = 'project-step-track';
      track.setAttribute('aria-label', 'Configured step sequence');
      steps.forEach((step, index) => {
        if (index > 0) {
          const arrow = document.createElement('span');
          arrow.className = 'project-step-arrow';
          arrow.setAttribute('aria-hidden', 'true');
          arrow.textContent = '→';
          track.appendChild(arrow);
        }
        const node = document.createElement('button');
        node.type = 'button';
        node.className = 'project-step-node' + (index === projectGraphState.stepIndex ? ' selected' : '');
        const name = projectConfiguredStepName(step, index, job.id);
        const kind = String(step.type || 'run');
        const title = document.createElement('span');
        title.className = 'project-step-node-title';
        title.textContent = String(index + 1) + '. ' + name;
        title.setAttribute('data-ciwi-overflow-text', title.textContent);
        const meta = document.createElement('span');
        meta.className = 'project-step-node-meta';
        meta.textContent = kind + (step.skip_dry_run ? ' · skipped in dry run' : '');
        node.appendChild(title);
        node.appendChild(meta);
        node.onclick = () => {
          if (ciwiElementContainsTextSelection(node)) return;
          projectGraphState.stepIndex = index;
          renderProjectGraph();
          requestAnimationFrame(() => {
            const selected = document.querySelector('.project-step-node.selected');
            const selectedTrack = selected && selected.closest('.project-step-track');
            if (selected && selectedTrack) {
              selectedTrack.scrollLeft = Math.max(0, selected.offsetLeft - (selectedTrack.clientWidth - selected.offsetWidth) / 2);
            }
          });
        };
        track.appendChild(node);
      });
      section.appendChild(track);
      const selected = steps[projectGraphState.stepIndex] || {};
      const detail = document.createElement('div');
      detail.className = 'project-step-detail';
      const selectedName = projectConfiguredStepName(selected, projectGraphState.stepIndex, job.id);
      const command = String(selected.type || '') === 'test' ? String(selected.test_command || '') : String(selected.run || '');
      const envKeys = Object.keys(selected.env || {});
      detail.innerHTML = '<strong>' + escapeHtml(selectedName) + '</strong>' +
        '<div class="muted" style="margin-top:4px;">type: ' + escapeHtml(String(selected.type || 'run')) +
        (selected.skip_dry_run ? ' · skip_dry_run' : '') +
        (envKeys.length ? ' · env: ' + escapeHtml(envKeys.join(', ')) : '') + '</div>' +
        '<pre>' + escapeHtml(command || '(no command)') + '</pre>';
      section.appendChild(detail);
      host.appendChild(section);
    }
`

const uiProjectGraphJS = uiGraphJS
