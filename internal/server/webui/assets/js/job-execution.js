
    const structureViewStorageKey = 'ciwi.project.structure.view.v1';
    const projectGraphMinScale = 0.35;
    const projectGraphMaxScale = 1.75;
    const projectGraphState = {
      project: null,
      chainID: 'all-pipelines',
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
        projectGraphState.fitOnRender = false;
        requestAnimationFrame(() => requestAnimationFrame(fitProjectGraph));
      }
    }

    function initializeProjectGraph(project) {
      projectGraphState.project = project;
      projectGraphState.scale = 1;
      projectGraphState.fitOnRender = true;
      const chains = Array.isArray(project.pipeline_chains) ? project.pipeline_chains : [];
      let storedChain = readProjectGraphStorage(projectGraphChainStorageKey(project.id), 'all-pipelines');
      if (storedChain === 'all') storedChain = 'all-pipelines';
      projectGraphState.chainID = storedChain === 'all-pipelines' || storedChain === 'all-chains' || chains.some(chain => String(chain.id) === storedChain)
        ? storedChain : 'all-pipelines';
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
      if (projectGraphState.chainID === 'all-pipelines') return pipelines;
      const chains = Array.isArray(project.pipeline_chains) ? project.pipeline_chains : [];
      if (projectGraphState.chainID === 'all-chains') {
        const included = new Set(chains.flatMap(chain => Array.isArray(chain.pipelines) ? chain.pipelines : []));
        return pipelines.filter(pipeline => included.has(pipeline.pipeline_id));
      }
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
      const previousHeight = previousViewport ? previousViewport.style.height : '';
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
      select.className = 'ciwi-select project-graph-select';
      const allOption = document.createElement('option');
      allOption.value = 'all-pipelines';
      allOption.textContent = 'All Pipelines';
      select.appendChild(allOption);
      const allChainsOption = document.createElement('option');
      allChainsOption.value = 'all-chains';
      allChainsOption.textContent = 'All chains';
      select.appendChild(allChainsOption);
      (project.pipeline_chains || []).forEach(chain => {
        const option = document.createElement('option');
        option.value = String(chain.id || '');
        option.textContent = pipelineChainDisplayName(chain) + ' (chain)';
        select.appendChild(option);
      });
      select.value = projectGraphState.chainID;
      select.onchange = () => {
        projectGraphState.chainID = String(select.value || 'all-pipelines');
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
      if (previousHeight) viewport.style.height = previousHeight;
      host.appendChild(viewport);
      const nodes = pipelines.map(pipeline => ({
        id: pipeline.pipeline_id,
        label: pipeline.pipeline_id,
        dependsOn: (pipeline.depends_on || []).filter(dep => pipelineIDs.has(dep)),
        meta: projectGraphNodeMeta((pipeline.jobs || []).length, (pipeline.depends_on || []).filter(dep => pipelineIDs.has(dep)).length, 'job'),
        runnable: true,
        runLabel: 'Run pipeline ' + pipeline.pipeline_id,
        runTitle: ciwiIndependentExecutionTooltip('Run this pipeline.', { shiftSelect: true }),
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
        runTitle: (job.matrix_includes || []).length
          ? ciwiIndependentExecutionTooltip('Choose a matrix entry and run this job.')
          : ciwiIndependentExecutionTooltip('Run this job.', { shiftSelect: true }),
      }));
      renderProjectDAG(viewport, jobNodes, {
        scale: projectGraphState.scale,
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


    function renderModeValue(dryRun) {
      const label = dryRun ? 'Dry run' : 'Ordinary run';
      return '' +
        '<span class="mode-value">' +
          '<span>' + label + '</span>' +
          '<span class="mode-info" tabindex="0" aria-label="Run mode info" data-mode="' + (dryRun ? 'dry' : 'ordinary') + '">' +
            '<span aria-hidden="true">' + ciwiIconHTML('info-circle') + '</span>' +
          '</span>' +
        '</span>';
    }

    function renderCacheStats(stats) {
      const box = document.getElementById('cacheStatsBox');
      if (!box) return;
      const entries = Array.isArray(stats) ? stats : [];
      if (!entries.length) {
        box.className = 'cache-stats-empty';
        box.textContent = 'No cache statistics reported for this job.';
        return;
      }
      box.className = 'cache-stats-list';
      box.innerHTML = entries.map(s => {
        const id = String((s && s.id) || '').trim();
        const env = String((s && s.env) || '').trim();
        const typ = String((s && s.type) || '').trim();
        const source = String((s && s.source) || '').trim();
        const path = String((s && s.path) || '').trim();
        const files = Number((s && s.files) || 0);
        const dirs = Number((s && s.directories) || 0);
        const size = Number((s && s.size_bytes) || 0);
        const err = String((s && s.error) || '').trim();
        const metrics = (s && s.tool_metrics) || {};
        const metricRows = Object.keys(metrics).sort((a,b) => a.localeCompare(b)).slice(0, 10).map(k =>
          '<div><code>' + escapeHtml(k) + '</code>: ' + escapeHtml(String(metrics[k] || '')) + '</div>'
        ).join('');
        return '' +
          '<div class="cache-stat-item">' +
            '<div class="cache-stat-head">' +
              '<span class="cache-stat-title">' + escapeHtml(id || 'cache') + '</span>' +
              (env ? ('<span class="cache-stat-pill">' + escapeHtml(env) + '</span>') : '') +
              (typ ? ('<span class="cache-stat-pill">' + escapeHtml(typ) + '</span>') : '') +
              (source ? ('<span class="cache-stat-pill">source: ' + escapeHtml(source) + '</span>') : '') +
            '</div>' +
            (path ? ('<div class="cache-stat-row">Path: <code>' + escapeHtml(path) + '</code></div>') : '') +
            '<div class="cache-stat-row">Size: ' + escapeHtml(formatBytes(size)) + ' | Files: ' + escapeHtml(String(files)) + ' | Dirs: ' + escapeHtml(String(dirs)) + '</div>' +
            (err ? ('<div class="cache-stat-row" style="color:var(--bad);">Error: ' + escapeHtml(err) + '</div>') : '') +
            (metricRows ? ('<div class="cache-stat-metrics">' + metricRows + '</div>') : '') +
          '</div>';
      }).join('');
    }

    function normalizeVersionLike(v) {
      const raw = String(v || '').trim();
      if (!raw) return '';
      let out = raw.replace(/^go/i, '').replace(/^v/i, '');
      return out.trim();
    }

    function compareVersionLike(a, b) {
      const pa = normalizeVersionLike(a).split('.').map(s => Number.parseInt(s, 10)).filter(n => Number.isFinite(n));
      const pb = normalizeVersionLike(b).split('.').map(s => Number.parseInt(s, 10)).filter(n => Number.isFinite(n));
      if (!pa.length || !pb.length) return null;
      const n = Math.max(pa.length, pb.length);
      for (let i = 0; i < n; i += 1) {
        const av = i < pa.length ? pa[i] : 0;
        const bv = i < pb.length ? pb[i] : 0;
        if (av < bv) return -1;
        if (av > bv) return 1;
      }
      return 0;
    }

    function toolConstraintSatisfied(actual, constraint) {
      const av = String(actual || '').trim();
      const c = String(constraint || '').trim();
      if (!av) return false;
      if (!c || c === '*') return true;
      let op = '';
      let target = c;
      ['>=', '<=', '>', '<', '==', '='].forEach(candidate => {
        if (!op && c.startsWith(candidate)) {
          op = candidate;
          target = c.slice(candidate.length).trim();
        }
      });
      if (!target) return true;
      if (!op) return av === target;
      const cmp = compareVersionLike(av, target);
      if (cmp == null) {
        return (op === '=' || op === '==') ? av === target : false;
      }
      if (op === '>') return cmp > 0;
      if (op === '>=') return cmp >= 0;
      if (op === '<') return cmp < 0;
      if (op === '<=') return cmp <= 0;
      return cmp === 0;
    }

    function requirementRows(requiredCaps, prefix) {
      const out = [];
      Object.keys(requiredCaps || {}).forEach(k => {
        if (!k.startsWith(prefix)) return;
        const tool = k.slice(prefix.length).trim();
        if (!tool) return;
        out.push({ tool: tool, constraint: String(requiredCaps[k] || '').trim() });
      });
      out.sort((a, b) => a.tool.localeCompare(b.tool));
      return out;
    }

    function renderSchedulingDiagnosis(schedulingDiagnosis) {
      const card = document.getElementById('schedulingCard');
      const box = document.getElementById('schedulingBox');
      if (!card || !box) return;
      const diagnosis = schedulingDiagnosis || null;
      const summary = String((diagnosis && diagnosis.summary) || '').trim();
      if (!summary) {
        card.style.display = 'none';
        box.textContent = '';
        return;
      }
      card.style.display = '';
      const requirements = Array.isArray(diagnosis.requirements) ? diagnosis.requirements : [];
      const agents = Array.isArray(diagnosis.agents) ? diagnosis.agents : [];
      const compatibleAgents = agents.filter(agent => !!agent.capability_match);
      const incompatibleAgents = agents.filter(agent => !agent.capability_match);
      const displayedAgents = compatibleAgents.concat(incompatibleAgents.slice(0, 3));
      const agentRows = displayedAgents.map(agent => {
        const capabilityIssues = Array.isArray(agent.capability_issues) ? agent.capability_issues : [];
        const availabilityIssues = Array.isArray(agent.availability_issues) ? agent.availability_issues : [];
        const details = availabilityIssues.concat(capabilityIssues.map(issue => String((issue && issue.message) || ''))).filter(Boolean);
        const status = agent.available ? 'eligible' : (agent.capability_match ? 'unavailable' : 'does not match');
        return '<li><strong>' + escapeHtml(String(agent.agent_id || 'agent')) + '</strong>: ' + escapeHtml(status + (details.length ? ' — ' + details.join('; ') : '')) + '</li>';
      }).join('');
      const hidden = Math.max(0, incompatibleAgents.length - 3);
      box.className = diagnosis.state === 'ready' ? 'req-ok' : 'req-issues';
      box.innerHTML = '<strong>' + escapeHtml(summary) + '</strong>' +
        (requirements.length ? '<div style="margin-top:6px;">Required: ' + requirements.map(value => '<code>' + escapeHtml(String(value || '')) + '</code>').join(', ') + '</div>' : '') +
        (agentRows ? '<ul>' + agentRows + '</ul>' : '') +
        (hidden ? '<div>' + escapeHtml(String(hidden) + ' additional agent(s) do not match') + '</div>' : '');
    }

    function renderToolRequirements(requiredCaps, runtimeCaps, jobStatus) {
      const req = requiredCaps || {};
      const caps = runtimeCaps || {};
      const hostRows = requirementRows(req, 'requires.tool.');
      const containerRows = requirementRows(req, 'requires.container.tool.');
      const status = String(jobStatus || '').trim().toLowerCase();

      function renderInto(boxId, rows, prefix, emptyText) {
        const box = document.getElementById(boxId);
        if (!box) return;
        if (!rows.length) {
          box.className = 'req-empty';
          box.textContent = emptyText;
          return;
        }
        const hasObservedRuntimeData = rows.some(r => {
          const key = prefix + r.tool;
          return String(caps[key] || '').trim() !== '';
        });
        if (!hasObservedRuntimeData) {
          box.className = 'req-empty';
          if (isQueuedJobStatus(status)) {
            box.textContent = 'No agent has leased this job yet; runtime capability data is not available.';
          } else if (isRunningJobStatus(status)) {
            box.textContent = 'Waiting for the leased agent runtime capability report.';
          } else {
            box.textContent = 'Runtime capability report unavailable for this execution.';
          }
          return;
        }
        const issues = [];
        rows.forEach(r => {
          const actual = String(caps[prefix + r.tool] || '').trim();
          if (!toolConstraintSatisfied(actual, r.constraint)) {
            issues.push('<code>' + escapeHtml(r.tool) + '</code> expected <code>' + escapeHtml(r.constraint || '*') + '</code>, got <code>' + escapeHtml(actual || 'missing') + '</code>');
          }
        });
        if (!issues.length) {
          box.className = 'req-ok';
          box.innerHTML = '<strong>Requirements matched</strong>';
          return;
        }
        box.className = 'req-issues';
        box.innerHTML = '<strong>Requirements mismatch</strong><ul>' + issues.map(i => '<li>' + i + '</li>').join('') + '</ul>';
      }

      renderInto('hostToolReqBox', hostRows, 'host.tool.', 'No tool requirements declared for this job.');
      renderInto('containerToolReqBox', containerRows, 'container.tool.', 'No container tool requirements declared for this job.');
    }
    function buildArtifactTree(items) {
      const root = { dirs: {}, files: [] };
      items.forEach((a) => {
        const raw = String((a && a.path) || '').trim();
        if (!raw) return;
        const parts = raw.split('/').filter(Boolean);
        if (parts.length === 0) return;
        let node = root;
        for (let i = 0; i < parts.length - 1; i += 1) {
          const seg = parts[i];
          if (!node.dirs[seg]) node.dirs[seg] = { dirs: {}, files: [] };
          node = node.dirs[seg];
        }
        node.files.push({ name: parts[parts.length - 1], item: a });
      });
      return root;
    }

    function collectArtifactExpandedPaths(box) {
      const out = new Set();
      if (!box) return out;
      box.querySelectorAll('details[data-artifact-dir]').forEach(d => {
        const p = String(d.getAttribute('data-artifact-dir') || '').trim();
        if (d.open && p) out.add(p);
      });
      return out;
    }

    function renderArtifactTreeNode(node, parentPath, depth, expanded, jobId) {
      const dirNames = Object.keys(node.dirs).sort((a, b) => a.localeCompare(b));
      const files = (node.files || []).slice().sort((a, b) => a.name.localeCompare(b.name));
      let html = '<ul class="artifact-tree">';
      dirNames.forEach(name => {
        const path = parentPath ? (parentPath + '/' + name) : name;
        const open = expanded.has(path);
        const zipHref = '/api/v1/jobs/' + encodeURIComponent(jobId) + '/artifacts/download?prefix=' + encodeURIComponent(path);
        html += '<li><details data-artifact-dir="' + escapeHtml(path) + '"' + (open ? ' open' : '') + '><summary>' + escapeHtml(name) + ' <a class="artifact-dir-download" href="' + zipHref + '" onclick="event.stopPropagation()">Download .zip</a></summary>' + renderArtifactTreeNode(node.dirs[name], path, depth + 1, expanded, jobId) + '</details></li>';
      });
      files.forEach(entry => {
        const a = entry.item || {};
        html += '' +
          '<li class="artifact-leaf">' +
            '<div class="artifact-row">' +
              '<span class="artifact-path">' + escapeHtml(entry.name) + '</span>' +
              '<span>(' + formatBytes(a.size_bytes) + ')</span>' +
              '<a href=\"' + a.url + '\" target=\"_blank\" rel=\"noopener\">Download</a>' +
            '</div>' +
          '</li>';
      });
      html += '</ul>';
      return html;
    }

    function renderArtifacts(box, jobId, items) {
      const downloadAllBtn = document.getElementById('artifactsDownloadAllBtn');
      const signature = JSON.stringify(items.map(a => [String(a.path || ''), Number(a.size_bytes || 0), String(a.url || '')]));
      if (signature === lastArtifactsSignature) {
        return;
      }
      const previousExpanded = collectArtifactExpandedPaths(box);
      if (previousExpanded.size > 0) {
        artifactExpandedPaths = previousExpanded;
      }
      if (items.length === 0) {
        if (downloadAllBtn) {
          downloadAllBtn.style.display = 'none';
          downloadAllBtn.setAttribute('href', '#');
        }
        box.textContent = 'No artifacts';
        lastArtifactsSignature = signature;
        return;
      }
      if (downloadAllBtn) {
        downloadAllBtn.style.display = '';
        downloadAllBtn.setAttribute('href', '/api/v1/jobs/' + encodeURIComponent(jobId) + '/artifacts/download-all');
      }
      const tree = buildArtifactTree(items);
      const expanded = (artifactExpandedPaths && artifactExpandedPaths.size > 0)
        ? new Set(artifactExpandedPaths)
        : new Set();
      if (expanded.size === 0) {
        // Default expansion is one directory level from root.
        Object.keys(tree.dirs || {}).forEach(name => expanded.add(name));
      }
      box.innerHTML = renderArtifactTreeNode(tree, '', 0, expanded, jobId);
      box.querySelectorAll('details[data-artifact-dir]').forEach(d => {
        d.addEventListener('toggle', () => {
          const path = String(d.getAttribute('data-artifact-dir') || '').trim();
          if (!path) return;
          if (artifactExpandedPaths == null) artifactExpandedPaths = new Set();
          if (d.open) artifactExpandedPaths.add(path);
          else artifactExpandedPaths.delete(path);
        });
      });
      artifactExpandedPaths = collectArtifactExpandedPaths(box);
      lastArtifactsSignature = signature;
    }

    function coverageTotals(c) {
      const total = Number(c.total_statements || c.total_lines || 0);
      const covered = Number(c.covered_statements || c.covered_lines || 0);
      return { total: total, covered: covered };
    }

    function coverageFileTotals(f) {
      const total = Number(f.total_statements || f.total_lines || 0);
      const covered = Number(f.covered_statements || f.covered_lines || 0);
      return { total: total, covered: covered };
    }

    function pct(covered, total) {
      if (!total) return 0;
      return (100 * covered) / total;
    }

    function renderCoverageReport(coverage) {
      const box = document.getElementById('coverageReportBox');
      if (!box) return;
      const openState = {};
      box.querySelectorAll('details[data-cov-key]').forEach(d => {
        const key = String(d.getAttribute('data-cov-key') || '');
        if (key) openState[key] = !!d.open;
      });
      if (!coverage) {
        box.textContent = 'No parsed coverage report';
        return;
      }
      const files = Array.isArray(coverage.files) ? coverage.files.slice() : [];
      const overall = coverageTotals(coverage);
      const overallPct = Number(coverage.percent || pct(overall.covered, overall.total) || 0);

      const modules = new Map();
      files.forEach(f => {
        const path = String(f.path || '').trim();
        const slash = path.lastIndexOf('/');
        const moduleName = slash > 0 ? path.slice(0, slash) : '.';
        const t = coverageFileTotals(f);
        const prev = modules.get(moduleName) || { total: 0, covered: 0, files: 0 };
        prev.total += t.total;
        prev.covered += t.covered;
        prev.files += 1;
        modules.set(moduleName, prev);
      });
      const moduleRows = Array.from(modules.entries())
        .sort((a, b) => pct(a[1].covered, a[1].total) - pct(b[1].covered, b[1].total))
        .map(([name, m]) =>
          '<tr>' +
          '<td style="padding:4px 6px;border-bottom:1px solid var(--code-line);"><code>' + escapeHtml(name) + '</code></td>' +
          '<td style="padding:4px 6px;border-bottom:1px solid var(--code-line);text-align:right;">' + m.files + '</td>' +
          '<td style="padding:4px 6px;border-bottom:1px solid var(--code-line);text-align:right;">' + m.covered + '/' + m.total + '</td>' +
          '<td style="padding:4px 6px;border-bottom:1px solid var(--code-line);text-align:right;"><strong>' + pct(m.covered, m.total).toFixed(2) + '%</strong></td>' +
          '</tr>'
        ).join('');

      const fileRows = files
        .slice()
        .sort((a, b) => pct(coverageFileTotals(a).covered, coverageFileTotals(a).total) - pct(coverageFileTotals(b).covered, coverageFileTotals(b).total))
        .map(f => {
          const t = coverageFileTotals(f);
          return '<tr>' +
            '<td style="padding:4px 6px;border-bottom:1px solid var(--code-line);"><code>' + escapeHtml(String(f.path || '')) + '</code></td>' +
            '<td style="padding:4px 6px;border-bottom:1px solid var(--code-line);text-align:right;">' + t.covered + '/' + t.total + '</td>' +
            '<td style="padding:4px 6px;border-bottom:1px solid var(--code-line);text-align:right;"><strong>' + pct(t.covered, t.total).toFixed(2) + '%</strong></td>' +
            '</tr>';
        }).join('');

      const root = { name: '/', children: new Map(), total: 0, covered: 0, isFile: false };
      files.forEach(f => {
        const path = String(f.path || '').trim();
        if (!path) return;
        const t = coverageFileTotals(f);
        const parts = path.split('/').filter(Boolean);
        let node = root;
        node.total += t.total;
        node.covered += t.covered;
        parts.forEach((part, idx) => {
          const key = idx === parts.length - 1 ? 'f:' + part : 'd:' + part;
          if (!node.children.has(key)) {
            node.children.set(key, { name: part, children: new Map(), total: 0, covered: 0, isFile: idx === parts.length - 1 });
          }
          node = node.children.get(key);
          node.total += t.total;
          node.covered += t.covered;
        });
      });

      function nodeHtml(node, prefix) {
        const nodeKey = prefix ? (prefix + '/' + node.name) : node.name;
        const children = Array.from(node.children.values())
          .sort((a, b) => {
            if (a.isFile !== b.isFile) return a.isFile ? 1 : -1;
            return a.name.localeCompare(b.name);
          })
          .map(ch => nodeHtml(ch, nodeKey))
          .join('');
        const label = escapeHtml(node.name) + ' - ' + node.covered + '/' + node.total + ' (' + pct(node.covered, node.total).toFixed(2) + '%)';
        if (!children) {
          return '<li><code>' + label + '</code></li>';
        }
        const isOpen = Object.prototype.hasOwnProperty.call(openState, 'tree:' + nodeKey) ? !!openState['tree:' + nodeKey] : false;
        return '<li><details data-cov-key="tree:' + escapeHtml(nodeKey) + '"' + (isOpen ? ' open' : '') + '><summary><code>' + label + '</code></summary><ul style="margin:6px 0 0 18px;padding:0 0 0 12px;">' + children + '</ul></details></li>';
      }
      const tree = '<ul style="margin:6px 0 0 0;padding:0 0 0 12px;">' + Array.from(root.children.values()).map(ch => nodeHtml(ch, '')).join('') + '</ul>';
      const openModules = Object.prototype.hasOwnProperty.call(openState, 'modules') ? !!openState.modules : true;
      const openFiles = Object.prototype.hasOwnProperty.call(openState, 'files') ? !!openState.files : false;
      const openTree = Object.prototype.hasOwnProperty.call(openState, 'tree') ? !!openState.tree : false;

      box.innerHTML =
        '<div style="margin:0 0 10px;padding:8px;border:1px solid var(--line);border-radius:6px;background:var(--surface-soft);">' +
          '<div><strong>Format:</strong> ' + escapeHtml(String(coverage.format || '')) + '</div>' +
          '<div><strong>Overall:</strong> ' + overallPct.toFixed(2) + '% (' + overall.covered + '/' + overall.total + ')</div>' +
          '<div><strong>Files:</strong> ' + files.length + '</div>' +
        '</div>' +
        '<details data-cov-key="modules"' + (openModules ? ' open' : '') + '><summary><strong>By Module</strong></summary>' +
          '<table style="width:100%;border-collapse:collapse;margin-top:6px;font-size:12px;">' +
          '<thead><tr><th style="text-align:left;border-bottom:1px solid var(--line);">Module</th><th style="text-align:right;border-bottom:1px solid var(--line);">Files</th><th style="text-align:right;border-bottom:1px solid var(--line);">Covered/Total</th><th style="text-align:right;border-bottom:1px solid var(--line);">Coverage</th></tr></thead>' +
          '<tbody>' + moduleRows + '</tbody></table>' +
        '</details>' +
        '<details data-cov-key="files"' + (openFiles ? ' open' : '') + '><summary><strong>By File</strong></summary>' +
          '<table style="width:100%;border-collapse:collapse;margin-top:6px;font-size:12px;">' +
          '<thead><tr><th style="text-align:left;border-bottom:1px solid var(--line);">File</th><th style="text-align:right;border-bottom:1px solid var(--line);">Covered/Total</th><th style="text-align:right;border-bottom:1px solid var(--line);">Coverage</th></tr></thead>' +
          '<tbody>' + fileRows + '</tbody></table>' +
        '</details>' +
        '<details data-cov-key="tree"' + (openTree ? ' open' : '') + '><summary><strong>Tree View</strong></summary>' + tree + '</details>';
    }

    function parseRepoContext(repoURL) {
      const raw = String(repoURL || '').trim();
      if (!raw) return { host: '', repoPath: '' };
      let host = '';
      let repoPath = '';
      let m = raw.match(/^https?:\/\/([^/]+)\/(.+)$/i);
      if (m) {
        host = String(m[1] || '').toLowerCase();
        repoPath = String(m[2] || '');
      } else {
        m = raw.match(/^git@([^:]+):(.+)$/i);
        if (m) {
          host = String(m[1] || '').toLowerCase();
          repoPath = String(m[2] || '');
        } else {
          m = raw.match(/^([^/]+\.[^/]+)\/(.+)$/i);
          if (m) {
            host = String(m[1] || '').toLowerCase();
            repoPath = String(m[2] || '');
          }
        }
      }
      repoPath = repoPath.replace(/\.git$/i, '').replace(/^\/+/, '').replace(/\/+$/, '');
      return { host: host, repoPath: repoPath };
    }

    function deriveTestSourceContext(job) {
      const j = job || {};
      const meta = (j.metadata || {});
      const src = (j.source || {});
      const repo = String(meta.pipeline_source_repo || src.repo || '').trim();
      const ref = String(meta.pipeline_source_ref_resolved || src.ref || '').trim();
      const parsed = parseRepoContext(repo);
      return { host: parsed.host, repoPath: parsed.repoPath, ref: ref };
    }

    function testCaseMatchesFilter(c, filter) {
      if (filter === 'all') return true;
      return String((c && c.status) || '').toLowerCase() === filter;
    }

    function testCaseStatusRank(c) {
      const st = String((c && c.status) || '').toLowerCase();
      if (st === 'fail') return 0;
      if (st === 'skip') return 1;
      if (st === 'pass') return 2;
      return 3;
    }

    function normalizeTestPath(path) {
      let p = String(path || '').trim();
      if (!p) return '';
      p = p.replace(/\\/g, '/');
      while (p.indexOf('./') === 0) p = p.slice(2);
      return p;
    }

    function testPackageRelativePath(pkg, sourceCtx) {
      const sc = sourceCtx || {};
      const host = String(sc.host || '').trim().toLowerCase();
      const repoPath = String(sc.repoPath || '').trim();
      const p = normalizeTestPath(pkg);
      if (!p || !repoPath) return '';
      const fullPrefix = (host ? (host + '/') : '') + repoPath + '/';
      if (p.indexOf(fullPrefix) === 0) return p.slice(fullPrefix.length);
      const ghPrefix = 'github.com/' + repoPath + '/';
      if (p.indexOf(ghPrefix) === 0) return p.slice(ghPrefix.length);
      const glPrefix = 'gitlab.com/' + repoPath + '/';
      if (p.indexOf(glPrefix) === 0) return p.slice(glPrefix.length);
      return '';
    }

    function resolveTestCaseSourcePath(testCase, sourceCtx) {
      const c = testCase || {};
      const sc = sourceCtx || {};
      const repoPath = String(sc.repoPath || '').trim();
      const host = String(sc.host || '').trim().toLowerCase();
      const relPkg = testPackageRelativePath(c.package, sourceCtx);
      let file = normalizeTestPath(c.file);
      if (!file) return '';
      const fullPrefix = (host ? (host + '/') : '') + repoPath + '/';
      if (repoPath && file.indexOf(fullPrefix) === 0) file = file.slice(fullPrefix.length);
      if (repoPath && file.indexOf(repoPath + '/') === 0) file = file.slice(repoPath.length + 1);
      if (repoPath && file.indexOf('github.com/' + repoPath + '/') === 0) file = file.slice(('github.com/' + repoPath + '/').length);
      if (repoPath && file.indexOf('gitlab.com/' + repoPath + '/') === 0) file = file.slice(('gitlab.com/' + repoPath + '/').length);
      if (relPkg && file.indexOf('/') < 0) file = relPkg + '/' + file;
      return normalizeTestPath(file);
    }

    function parseExternalRepoPath(rawPath) {
      const p = normalizeTestPath(rawPath);
      if (!p) return null;
      const segs = p.split('/').filter(Boolean);
      if (segs.length < 4) return null;
      const host = String(segs[0] || '').toLowerCase();
      if (host !== 'github.com' && host !== 'gitlab.com') return null;
      const owner = String(segs[1] || '').trim();
      const repo = String(segs[2] || '').trim();
      if (!owner || !repo) return null;
      const repoPath = owner + '/' + repo;
      const subPath = normalizeTestPath(segs.slice(3).join('/'));
      return { host: host, repoPath: repoPath, subPath: subPath };
    }

    function inferExternalTestSource(testCase, sourceCtx) {
      const c = testCase || {};
      const sc = sourceCtx || {};
      const parsedFile = parseExternalRepoPath(c.file);
      if (parsedFile && parsedFile.subPath) {
        const sameRepo = String(sc.host || '').toLowerCase() === parsedFile.host && String(sc.repoPath || '') === parsedFile.repoPath;
        const ref = sameRepo && String(sc.ref || '').trim() ? String(sc.ref || '').trim() : 'HEAD';
        return {
          sourceCtx: { host: parsedFile.host, repoPath: parsedFile.repoPath, ref: ref },
          relPath: parsedFile.subPath,
          packageRelPath: '',
        };
      }
      const parsedPackage = parseExternalRepoPath(c.package);
      const file = normalizeTestPath(c.file);
      if (!parsedPackage) return null;
      let relPath = parsedPackage.subPath;
      if (file) {
        if (file.indexOf('/') >= 0) relPath = file;
        else if (relPath) relPath = relPath + '/' + file;
        else relPath = file;
      }
      relPath = normalizeTestPath(relPath);
      if (!relPath) return null;
      const sameRepo = String(sc.host || '').toLowerCase() === parsedPackage.host && String(sc.repoPath || '') === parsedPackage.repoPath;
      const ref = sameRepo && String(sc.ref || '').trim() ? String(sc.ref || '').trim() : 'HEAD';
      return {
        sourceCtx: { host: parsedPackage.host, repoPath: parsedPackage.repoPath, ref: ref },
        relPath: relPath,
        packageRelPath: parsedPackage.subPath,
      };
    }

    function buildBlobURL(sourceCtx, relPath, line) {
      const sc = sourceCtx || {};
      const host = String(sc.host || '').trim().toLowerCase();
      const repoPath = String(sc.repoPath || '').trim();
      const ref = String(sc.ref || '').trim();
      const path = normalizeTestPath(relPath);
      if (!host || !repoPath || !ref || !path) return '';
      if (host === 'github.com') {
        return 'https://github.com/' + repoPath + '/blob/' + encodeURIComponent(ref) + '/' + encodeURI(path) + (line > 0 ? ('#L' + line) : '');
      }
      if (host === 'gitlab.com') {
        return 'https://gitlab.com/' + repoPath + '/-/blob/' + encodeURIComponent(ref) + '/' + encodeURI(path) + (line > 0 ? ('#L' + line) : '');
      }
      return '';
    }

    function buildCodeSearchURL(sourceCtx, name, pkgRelPath) {
      const sc = sourceCtx || {};
      const host = String(sc.host || '').trim().toLowerCase();
      const repoPath = String(sc.repoPath || '').trim();
      const ref = String(sc.ref || '').trim();
      const testName = String(name || '').trim();
      if (!host || !repoPath || !testName) return '';
      const terms = ['"' + testName + '"'];
      if (pkgRelPath) terms.push('path:' + pkgRelPath);
      const query = encodeURIComponent(terms.join(' '));
      if (host === 'github.com') {
        let url = 'https://github.com/' + repoPath + '/search?q=' + query + '&type=code';
        if (ref) url += '&ref=' + encodeURIComponent(ref);
        return url;
      }
      if (host === 'gitlab.com') {
        return 'https://gitlab.com/' + repoPath + '/-/search?search=' + query + '&scope=blobs';
      }
      return '';
    }

    function buildTestCaseSourceURL(testCase, sourceCtx) {
      const c = testCase || {};
      let activeSourceCtx = sourceCtx || {};
      let relPath = resolveTestCaseSourcePath(c, sourceCtx);
      if (!relPath) {
        const inferred = inferExternalTestSource(c, sourceCtx);
        if (inferred && inferred.sourceCtx && inferred.relPath) {
          activeSourceCtx = inferred.sourceCtx;
          relPath = inferred.relPath;
        }
      }
      const line = Number(c.line || 0);
      const blobURL = buildBlobURL(activeSourceCtx, relPath, line);
      if (blobURL) return blobURL;

      const name = String(c.name || '').trim();
      if (!name) return '';
      let pkgRel = testPackageRelativePath(c.package, sourceCtx);
      const inferredForSearch = inferExternalTestSource(c, sourceCtx);
      if (inferredForSearch && inferredForSearch.sourceCtx) {
        activeSourceCtx = inferredForSearch.sourceCtx;
        if (inferredForSearch.packageRelPath) pkgRel = inferredForSearch.packageRelPath;
      }
      return buildCodeSearchURL(activeSourceCtx, name, pkgRel);
    }

    function renderTestReport(report, job) {
      const box = document.getElementById('testReportBox');
      if (!box) return;
      const suites = report && Array.isArray(report.suites) ? report.suites : [];
      if (!suites.length) {
        box.textContent = 'No parsed test report';
        return;
      }
      const sourceCtx = deriveTestSourceContext(job);
      if (window.__ciwiTestFilter == null) {
        window.__ciwiTestFilter = (Number(report.failed || 0) > 0) ? 'fail' : 'all';
      }
      const activeFilter = String(window.__ciwiTestFilter || 'all');
      const header = '' +
        '<div class="test-summary-row">' +
          '<span class="test-pill">Total: ' + (report.total || 0) + '</span>' +
          '<span class="test-pill test-pill-pass">Passed: ' + (report.passed || 0) + '</span>' +
          '<span class="test-pill test-pill-fail">Failed: ' + (report.failed || 0) + '</span>' +
          '<span class="test-pill test-pill-skip">Skipped: ' + (report.skipped || 0) + '</span>' +
        '</div>' +
        '<div class="test-filter-row">' +
          '<button type="button" class="test-filter-btn' + (activeFilter === 'all' ? ' active' : '') + '" data-test-filter="all">All</button>' +
          '<button type="button" class="test-filter-btn' + (activeFilter === 'fail' ? ' active' : '') + '" data-test-filter="fail">Failed</button>' +
          '<button type="button" class="test-filter-btn' + (activeFilter === 'skip' ? ' active' : '') + '" data-test-filter="skip">Skipped</button>' +
          '<button type="button" class="test-filter-btn' + (activeFilter === 'pass' ? ' active' : '') + '" data-test-filter="pass">Passed</button>' +
        '</div>';

      const suiteHtml = suites.map((s, suiteIdx) => {
        const cases = Array.isArray(s.cases) ? s.cases : [];
        const modules = new Map();
        cases.forEach(c => {
          const mod = String(c.package || '').trim() || '(root)';
          if (!modules.has(mod)) modules.set(mod, []);
          modules.get(mod).push(c);
        });
        const moduleHtml = Array.from(modules.entries())
          .sort((a, b) => a[0].localeCompare(b[0]))
          .map(([mod, moduleCases], modIdx) => {
            const visibleCases = moduleCases
              .filter(c => testCaseMatchesFilter(c, activeFilter))
              .slice()
              .sort((a, b) => {
                const byStatus = testCaseStatusRank(a) - testCaseStatusRank(b);
                if (byStatus !== 0) return byStatus;
                return String(a.name || '').localeCompare(String(b.name || ''));
              });
            if (!visibleCases.length) return '';
            let mPass = 0;
            let mFail = 0;
            let mSkip = 0;
            visibleCases.forEach(c => {
              const st = String(c.status || '').toLowerCase();
              if (st === 'pass') mPass++;
              else if (st === 'fail') mFail++;
              else if (st === 'skip') mSkip++;
            });
            const rows = visibleCases.map(c => {
              const testName = escapeHtml(c.name || '');
              const sourceURL = buildTestCaseSourceURL(c, sourceCtx);
              const nameCell = sourceURL
                ? ('<a href="' + sourceURL + '" target="_blank" rel="noopener noreferrer">' + testName + '</a>')
                : testName;
              return '<tr>' +
              '<td>' + nameCell + '</td>' +
              '<td>' + escapeHtml(c.status || '') + '</td>' +
              '<td>' + (c.duration_seconds || 0).toFixed(3) + 's</td>' +
              '</tr>';
            }).join('');
            return '<details data-test-key="suite:' + suiteIdx + ':mod:' + modIdx + '">' +
              '<summary><code>' + escapeHtml(mod) + '</code> - total=' + visibleCases.length + ', passed=' + mPass + ', failed=' + mFail + ', skipped=' + mSkip + '</summary>' +
              '<table style="width:100%;border-collapse:collapse;margin-top:6px;font-size:12px;">' +
              '<thead><tr><th style="text-align:left;border-bottom:1px solid var(--line);">Test</th><th style="text-align:left;border-bottom:1px solid var(--line);">Status</th><th style="text-align:left;border-bottom:1px solid var(--line);">Duration</th></tr></thead>' +
              '<tbody>' + rows + '</tbody></table>' +
              '</details>';
          }).filter(Boolean).join('');
        if (!moduleHtml) return '';
        return '<div style="margin-top:10px;">' +
          '<div><strong>' + escapeHtml(s.name || 'suite') + '</strong> (' + escapeHtml(s.format || '') + ')</div>' +
          '<div class="muted">total=' + (s.total || 0) + ', passed=' + (s.passed || 0) + ', failed=' + (s.failed || 0) + ', skipped=' + (s.skipped || 0) + '</div>' +
          '<div style="margin-top:6px;display:flex;flex-direction:column;gap:6px;">' + moduleHtml + '</div>' +
          '</div>';
      }).filter(Boolean).join('');
      const emptyMsg = suiteHtml ? '' : '<div class="muted">No tests for selected filter.</div>';
      box.innerHTML = header + emptyMsg + suiteHtml;
      box.querySelectorAll('[data-test-filter]').forEach(btn => {
        btn.addEventListener('click', () => {
          window.__ciwiTestFilter = String(btn.getAttribute('data-test-filter') || 'all');
          renderTestReport(report, job);
        });
      });
    }
    function classifyLine(rawLine) {
      if (/^\[meta\]/.test(rawLine)) return 'phase-meta';
      if (/^\[checkout\]/.test(rawLine)) return 'phase-checkout';
      if (/^\[run\]/.test(rawLine)) return 'phase-run';
      if (/^[+]{1,2}\s/.test(rawLine)) return 'shell-trace';
      if (/^[+]{1,2}\s*(git push|gh release create|gh release upload)\b/.test(rawLine)) return 'shell-trace risky-cmd';
      return '';
    }

    function highlightTextTokens(rawText) {
      let out = escapeHtml(rawText);
      out = out.replace(/\b(v\d+\.\d+\.\d+)\b/g, '<span class="tok-version">$1</span>');
      out = out.replace(/\b([0-9a-fA-F]{7,40})\b/g, '<span class="tok-sha">$1</span>');
      out = out.replace(/\bduration=([0-9]+(?:\.[0-9]+)?s)\b/g, 'duration=<span class="tok-duration">$1</span>');
      return out;
    }

    function highlightInline(rawLine) {
      const src = String(rawLine || '');
      const urlRE = /https:\/\/[^\s"']+/g;
      let out = '';
      let last = 0;
      let match;
      while ((match = urlRE.exec(src)) !== null) {
        out += highlightTextTokens(src.slice(last, match.index));
        out += '<span class="tok-url">' + escapeHtml(match[0]) + '</span>';
        last = match.index + match[0].length;
      }
      out += highlightTextTokens(src.slice(last));
      return out;
    }

    function renderDryRunSkippedBlock(lines) {
      const cleaned = lines.filter(l => String(l || '').trim() !== '');
      if (!cleaned.length) return '';
      const head = '<div class="log-dryskip-head">[dry-run] skipped step</div>';
      const body = '<div class="log-dryskip-body">' + cleaned.map(highlightInline).join('\n') + '</div>';
      return '<div class="log-dryskip">' + head + body + '</div>';
    }

    function renderDetachedHeadFold(lines) {
      const text = lines.join('\n');
      return '<details class="log-fold"><summary>git detached HEAD advice (collapsed)</summary><pre>' + escapeHtml(text) + '</pre></details>';
    }

    function stepEventDisplayName(step) {
      step = step || {};
      let name = String(step.name || '').trim();
      if (name.indexOf('\n') >= 0) {
        name = name.split('\n').map(s => s.trim()).filter(Boolean)[0] || name;
      }
      name = name.replace(/\s+/g, ' ').trim();
      if (!name) return '';
      return name.replace(/_/g, ' ');
    }

    function stepEventTitle(step) {
      step = step || {};
      const idx = Number(step.index || 0);
      const total = Number(step.total || 0);
      const title = idx > 0 && total > 0 ? ('Job step ' + idx + '/' + total) : (idx > 0 ? ('Job step ' + idx) : 'Job step');
      const name = stepEventDisplayName(step);
      if (idx > 0 && name.toLowerCase() === ('step ' + idx)) return title;
      return name ? (title + ': ' + name) : title;
    }

    function executionTimelineMaps(job) {
      const byKey = Object.create(null);
      (Array.isArray(job && job.execution_timeline) ? job.execution_timeline : []).forEach(item => {
        if (!item) return;
        const key = item.kind === 'phase' ? ('phase:' + String(item.id || '')) : ('step:' + String(Number(item.step_index || 0)));
        byKey[key] = item;
      });
      return byKey;
    }

    function executionEventKey(ev) {
      if (ev && ev.phase) return 'phase:' + String((ev.phase || {}).id || '');
      if (ev && ev.step) return 'step:' + String(Number((ev.step || {}).index || 0));
      return '';
    }

    function structuredExecutionGroups(job, events) {
      const groups = [];
      const byKey = Object.create(null);
      const timeline = executionTimelineMaps(job);
      const stepPlanByIndex = Object.create(null);
      (Array.isArray(job && job.step_plan) ? job.step_plan : []).forEach(step => {
        const index = Number((step && step.index) || 0);
        if (index > 0) stepPlanByIndex[index] = step;
      });
      (Array.isArray(job && job.execution_timeline) ? job.execution_timeline : []).forEach(item => {
        if (!item) return;
        const isPhase = String(item.kind || '') === 'phase';
        const key = isPhase
          ? ('phase:' + String(item.id || ''))
          : ('step:' + String(Number(item.step_index || 0)));
        if (!key || byKey[key]) return;
        const step = isPhase ? null : (stepPlanByIndex[Number(item.step_index || 0)] || null);
        const phase = isPhase
          ? { id: item.id, name: item.name, description: item.description, index: item.index, total: item.total }
          : null;
        const group = { key: key, item: item, step: step, phase: phase, reached: false, started: '', output: '', finish: null };
        byKey[key] = group;
        groups.push(group);
      });
      (Array.isArray(events) ? events : []).forEach(ev => {
        const key = executionEventKey(ev);
        if (!key) return;
        let group = byKey[key];
        if (!group) {
          const fallback = ev.phase
            ? { id: ev.phase.id, kind: 'phase', name: ev.phase.name, description: ev.phase.description, index: ev.phase.index, total: ev.phase.total }
            : { id: key, kind: 'step', name: (ev.step || {}).name, step_index: (ev.step || {}).index, index: (ev.step || {}).index, total: (ev.step || {}).total };
          group = { key: key, item: timeline[key] || fallback, step: ev.step || null, phase: ev.phase || null, reached: false, started: '', output: '', finish: null };
          byKey[key] = group;
          groups.push(group);
        }
        group.reached = true;
        if (ev.step && (!group.step || !String(group.step.name || '').trim())) group.step = ev.step;
        if (ev.phase && !group.phase) group.phase = ev.phase;
        if (ev.type === 'step.started' || ev.type === 'phase.started') group.started = String(ev.timestamp_utc || '');
        if (ev.type === 'step.output' || ev.type === 'phase.output') group.output += String(ev.output || '');
        if (ev.type === 'step.finished' || ev.type === 'phase.finished') group.finish = ev;
      });
      groups.sort((a, b) => Number((a.item || {}).index || 0) - Number((b.item || {}).index || 0));
      const phases = groups.filter(group => String((group.item || {}).kind || '') === 'phase');
      const jobSteps = groups.filter(group => String((group.item || {}).kind || '') !== 'phase');
      phases.forEach((group, index) => {
        group.categoryIndex = index + 1;
        group.categoryTotal = phases.length;
      });
      jobSteps.forEach((group, index) => {
        group.categoryIndex = index + 1;
        group.categoryTotal = jobSteps.length;
      });
      return groups;
    }

    function executionGroupTitle(group) {
      const item = (group && group.item) || {};
      const isPhase = String(item.kind || '') === 'phase';
      const idx = Number((group && group.categoryIndex) || 0);
      const total = Number((group && group.categoryTotal) || 0);
      const category = isPhase ? 'Ciwi phase' : 'Job step';
      const prefix = idx > 0 && total > 0 ? (category + ' ' + idx + '/' + total) : (idx > 0 ? (category + ' ' + idx) : category);
      const name = stepEventDisplayName({ name: item.name || ((group.step || {}).name) || ((group.phase || {}).name) });
      return name ? (prefix + ': ' + name) : prefix;
    }

    function hasStructuredLogEvents(events) {
      return (Array.isArray(events) ? events : []).some(ev => ev && (
        ev.type === 'system.message' ||
        (ev.step && (ev.type === 'step.started' || ev.type === 'step.output' || ev.type === 'step.finished')) ||
        (ev.phase && (ev.type === 'phase.started' || ev.type === 'phase.output' || ev.type === 'phase.finished'))
      ));
    }

    function stepEventCommandSummary(step) {
      const text = String((step && step.script) || '').trim();
      if (!text) return '';
      return text.replace(/\s+/g, ' ');
    }

    function renderStructuredOutputLog(job, events) {
      const groups = structuredExecutionGroups(job, events);
      const byKey = Object.create(null);
      groups.forEach(group => { byKey[group.key] = group; });
      const renderedSteps = Object.create(null);
      if (!hasStructuredLogEvents(events) && !groups.length) return '<span class="log-empty">&lt;no structured output&gt;</span>';
      function renderGroup(group) {
        if (!group) return '';
        const key = String(group.key || '');
        if (!key || renderedSteps[key]) return '';
        renderedSteps[key] = true;
        const finish = group.finish || null;
        const reached = !!group.reached;
        const remembered = (typeof logStepOpenState !== 'undefined') ? logStepOpenState[group.key] : undefined;
        const open = (remembered === true || remembered === false) ? remembered : false;
        const meta = [];
        if (!reached) meta.push('Status: Not reached');
        if (group.started) meta.push('Started: ' + escapeHtml(formatTimestamp(group.started)));
        if (finish && Number(finish.duration_ms || 0) > 0) meta.push('Duration: ' + escapeHtml(formatDurationMs(Number(finish.duration_ms || 0))));
        if (finish && finish.exit_code !== null && finish.exit_code !== undefined) meta.push('Exit code: ' + escapeHtml(String(finish.exit_code)));
        if (finish && String(finish.error || '').trim()) meta.push('Error: ' + escapeHtml(String(finish.error || '').trim()));
        const isPhase = String((group.item || {}).kind || '') === 'phase';
        const script = String((group.step && group.step.script) || '');
        const yamlLiteral = String((group.step && group.step.yaml_literal) || '');
        const output = String(group.output || '');
        const commandSummary = isPhase ? '' : stepEventCommandSummary(group.step);
        const detailsBlock = isPhase
          ? ('<div class="log-step-label">Details</div><pre>' + escapeHtml(String((group.item || {}).description || (group.phase || {}).description || '(none)')) + '</pre>')
          : ('<div class="log-step-label">YAML literal</div><pre>' + escapeHtml(yamlLiteral || '(none)') + '</pre>' +
             '<div class="log-step-label">Expanded command</div><pre>' + escapeHtml(script || '(none)') + '</pre>');
        return '' +
          '<details class="log-step' + (reached ? '' : ' log-step-unreached') + '" data-step-key="' + escapeHtml(group.key) + '"' + (open ? ' open' : '') + '>' +
            '<summary><span class="log-step-summary-title">' + escapeHtml(executionGroupTitle(group)) + '</span>' + (commandSummary ? '<span class="log-step-summary-command" data-ciwi-overflow-text="' + escapeHtml(commandSummary) + '">' + escapeHtml(commandSummary) + '</span>' : '') + (!reached ? '<span class="log-step-status">Not reached</span>' : '') + '</summary>' +
            '<button class="copy-btn log-step-collapse-btn" type="button" title="Collapse this step" hidden>Collapse ' + ciwiIconHTML('arrow-up') + '</button>' +
            (meta.length ? ('<div class="log-step-meta">' + meta.map(m => '<span>' + m + '</span>').join('') + '</div>') : '') +
            detailsBlock +
            '<div class="log-step-label">Output</div>' +
            '<div>' + renderOutputLog(output || (reached ? '(no output)' : '(step was not reached)')) + '</div>' +
          '</details>';
      }
      const html = (Array.isArray(events) ? events : []).map(ev => {
        if (ev && ev.type === 'system.message') {
          const message = String(ev.message || '');
          return message ? ('<div class="log-system-message">' + renderOutputLog(message) + '</div>') : '';
        }
        return renderGroup(byKey[executionEventKey(ev)]);
      });
      groups.forEach(group => {
        if (!renderedSteps[group.key]) html.push(renderGroup(group));
      });
      return html.join('');
    }

    function bindStructuredStepProgress(job, events) {
      const groups = structuredExecutionGroups(job, events);
      const byKey = Object.create(null);
      groups.forEach(group => { byKey[group.key] = group; });
      const activeIdx = activeTimelineIndex(job);
      const expectedByStep = (job && job.step_expected_duration_ms) || {};
      const expectedByPhase = (job && job.phase_expected_duration_ms) || {};
      document.querySelectorAll('#logBox details.log-step[data-step-key]').forEach(details => {
        const key = String(details.getAttribute('data-step-key') || '');
        const group = byKey[key];
        const summary = details.querySelector(':scope > summary');
        if (!group || !summary) return;
        const index = Number((group.item || {}).index || 0);
        const finish = group.finish || null;
        const running = activeIdx === index - 1 && isRunningJobStatus((job && job.status) || '');
        if (!finish && !running) return;
        const isPhase = String((group.item || {}).kind || '') === 'phase';
        const expectedDurationMS = isPhase
          ? Number(expectedByPhase[String((group.item || {}).id || '')] || 0)
          : Number(expectedByStep[String((group.item || {}).step_index || '')] || 0);
        bindCiwiProgress(summary, {
          status: finish ? (String(finish.error || '').trim() || finish.exit_code !== null && finish.exit_code !== undefined ? 'failed' : 'succeeded') : 'running',
          started_utc: group.started || '',
          finished_utc: finish ? String(finish.timestamp_utc || '') : '',
          leased_by_agent_id: String((job && job.leased_by_agent_id) || ''),
          expected_duration_ms: expectedDurationMS,
        });
      });
    }

    function plainTextFromStructuredEvents(job, events) {
      const groups = structuredExecutionGroups(job, events);
      const byKey = Object.create(null);
      groups.forEach(group => { byKey[group.key] = group; });
      const renderedSteps = Object.create(null);
      const lines = [];
      lines.push('ciwi job log');
      lines.push('Job execution ID: ' + String((job && job.id) || ''));
      lines.push('Status: ' + String((job && job.status) || ''));
      lines.push('');
      (Array.isArray(events) ? events : []).forEach(ev => {
        if (ev && ev.type === 'system.message') {
          const message = String(ev.message || '').trimEnd();
          if (message) {
            lines.push(message);
            lines.push('');
          }
          return;
        }
        const key = executionEventKey(ev);
        if (!key) return;
        const group = byKey[key];
        if (!group || renderedSteps[key]) return;
        renderedSteps[key] = true;
        lines.push('--------------------------------------------------------------------------------');
        lines.push(executionGroupTitle(group));
        lines.push('--------------------------------------------------------------------------------');
        if (group.started) lines.push('Start time: ' + formatTimestamp(group.started));
        if (group.finish && Number(group.finish.duration_ms || 0) > 0) {
          const durationLabel = String((group.item || {}).kind || '') === 'phase' ? 'Ciwi phase duration' : 'Job step duration';
          lines.push(durationLabel + ': ' + formatDurationMs(Number(group.finish.duration_ms || 0)));
        }
        if (group.finish && group.finish.exit_code !== null && group.finish.exit_code !== undefined) lines.push('Exit code: ' + String(group.finish.exit_code));
        if (group.finish && String(group.finish.error || '').trim()) lines.push('Error: ' + String(group.finish.error || '').trim());
        lines.push('');
        if (String((group.item || {}).kind || '') === 'phase') {
          lines.push('Details:');
          lines.push(String((group.item || {}).description || (group.phase || {}).description || ''));
        } else {
          lines.push('YAML literal:');
          lines.push("'''");
          lines.push(String((group.step && (group.step.yaml_literal || group.step.script)) || ''));
          lines.push("'''");
          lines.push('');
          lines.push('Expanded command:');
          lines.push("'''");
          lines.push(String((group.step && group.step.script) || ''));
          lines.push("'''");
        }
        lines.push('');
        lines.push('Output:');
        lines.push("'''");
        lines.push(String(group.output || ''));
        lines.push("'''");
        lines.push('');
      });
      return lines.join('\n');
    }

    function renderOutputLog(raw) {
      const text = String(raw || '');
      if (!text) return '<span class="log-empty">&lt;no output yet&gt;</span>';
      const lines = text.replace(/\r\n/g, '\n').replace(/\r/g, '\n').split('\n');
      const html = [];
      for (let i = 0; i < lines.length; i++) {
        const line = lines[i];
        if (/^\[dry-run\]\s+skipped step:/.test(line)) {
          const skipped = [line.replace(/^\[dry-run\]\s+skipped step:\s*/, '')];
          for (let j = i + 1; j < lines.length; j++) {
            const next = lines[j];
            if (/^\[(meta|checkout|run|dry-run)\]/.test(next) || /^[+]{1,2}\s/.test(next)) {
              i = j - 1;
              break;
            }
            if (j === lines.length - 1) i = j;
            if (next.trim() === '') {
              i = j;
              break;
            }
            skipped.push(next);
          }
          html.push(renderDryRunSkippedBlock(skipped));
          continue;
        }

        if (line.indexOf("You are in 'detached HEAD' state.") === 0) {
          const folded = [line];
          for (let j = i + 1; j < lines.length; j++) {
            const next = lines[j];
            folded.push(next);
            if (next.indexOf("Turn off this advice by setting config variable advice.detachedHead to false") === 0) {
              i = j;
              break;
            }
            if (j === lines.length - 1) i = j;
          }
          html.push(renderDetachedHeadFold(folded));
          continue;
        }

        const cls = classifyLine(line);
        const classAttr = cls ? ' class="log-line ' + cls + '"' : ' class="log-line"';
        html.push('<div' + classAttr + '>' + highlightInline(line) + '</div>');
      }
      return html.join('');
    }
    function renderReleaseSummary(job) {
      const card = document.getElementById('releaseSummaryCard');
      const box = document.getElementById('releaseSummaryBox');
      if (!card || !box) return;

      const m = (job && job.metadata) || {};
      const isReleasePipeline = (m.pipeline_id || '') === 'release';
      if (!isReleasePipeline) {
        card.style.display = 'none';
        box.innerHTML = '';
        return;
      }

      const dryRun = (m.dry_run || '') === '1';
      const versionLabel = String(m.version || m.pipeline_version_raw || '').trim();
      const tagLabel = String(m.tag || m.pipeline_version || '').trim();
      const lines = [];
      lines.push('<div><strong>Mode:</strong> ' + (dryRun ? 'dry-run' : 'live') + '</div>');
      if (versionLabel) lines.push('<div><strong>Version:</strong> ' + escapeHtml(versionLabel) + '</div>');
      if (tagLabel) lines.push('<div><strong>Tag:</strong> ' + escapeHtml(tagLabel) + '</div>');
      if (m.artifacts) lines.push('<div><strong>Assets:</strong> ' + escapeHtml(m.artifacts) + '</div>');
      if (m.next_version) lines.push('<div><strong>Next version:</strong> ' + escapeHtml(m.next_version) + '</div>');
      if (m.auto_bump_branch) lines.push('<div><strong>Auto bump branch:</strong> ' + escapeHtml(m.auto_bump_branch) + '</div>');
      if (lines.length === 1) lines.push('<div class="label">No release metadata reported yet.</div>');

      box.innerHTML = lines.join('');
      card.style.display = '';
    }



    const runContextCollapsedStorageKey = 'ciwi.jobExecution.runContext.collapsed.v1';
    const jobExecutionGraphState = {
      context: null,
      selectedPipelineID: '',
      selectedJobID: '',
      terminalLoaded: false,
      contextSignature: '',
    };

    function readRunContextCollapsed() {
      try { return localStorage.getItem(runContextCollapsedStorageKey) === '1'; } catch (_) { return false; }
    }

    function writeRunContextCollapsed(collapsed) {
      try { localStorage.setItem(runContextCollapsedStorageKey, collapsed ? '1' : '0'); } catch (_) {}
    }

    function initializeJobRunContextCard() {
      const card = document.getElementById('runContextCard');
      const button = document.getElementById('runContextToggleBtn');
      if (!card || !button) return;
      const apply = collapsed => {
        card.classList.toggle('collapsed', collapsed);
        button.textContent = collapsed ? 'Expand' : 'Collapse';
        button.setAttribute('aria-expanded', collapsed ? 'false' : 'true');
      };
      apply(readRunContextCollapsed());
      button.onclick = () => {
        const collapsed = !card.classList.contains('collapsed');
        apply(collapsed);
        writeRunContextCollapsed(collapsed);
      };
    }

    function fitExecutionGraphViewport(viewport, layout) {
      if (!viewport || !layout) return;
      const viewportHeight = Math.min(360, Math.max(148, layout.contentHeight + 16));
      viewport.style.height = viewportHeight + 'px';
      requestAnimationFrame(() => {
        const stage = viewport.querySelector('.project-graph-stage');
        const content = viewport.querySelector('.project-graph-content');
        if (!stage || !content) return;
        const scale = Math.max(projectGraphMinScale, Math.min(1, (viewport.clientWidth - 16) / layout.contentWidth, (viewport.clientHeight - 16) / layout.contentHeight));
        stage.style.width = Math.ceil(layout.contentWidth * scale) + 'px';
        stage.style.height = Math.ceil(layout.contentHeight * scale) + 'px';
        content.style.transform = 'scale(' + scale + ')';
      });
    }

    async function runCurrentDefinitionPipeline(event, pipeline, button) {
      const pipelineDBID = Number((pipeline && pipeline.pipeline_db_id) || 0);
      if (!pipelineDBID) return;
      if (button) button.disabled = true;
      try {
        const result = await runWithOptionalSourceRef(event, {
          runPath: '/api/v1/pipelines/' + pipelineDBID + '/run-selection',
          sourceRefsPath: '/api/v1/pipelines/' + pipelineDBID + '/source-refs',
          eligibleAgentsPath: '/api/v1/pipelines/' + pipelineDBID + '/eligible-agents',
          payload: {},
          title: 'Run Current Pipeline Definition',
          subtitle: String(pipeline.pipeline_id || ''),
          runLabel: 'Run Pipeline',
        });
        if (result.cancelled) return;
        showQueuedJobsSnackbar('Current ' + String(pipeline.pipeline_id || 'pipeline') + ' definition started');
      } catch (error) {
        await showAlertDialog({ title: 'Run failed', message: 'Run failed: ' + String(error && error.message || error) });
      } finally {
        if (button) button.disabled = false;
      }
    }

    async function rerunGraphExecution(execution, button) {
      const id = String((execution && execution.id) || '').trim();
      if (!id) return false;
      if (button) button.disabled = true;
      try {
        const data = await apiActionJSON('rerun-execution', { jobExecutionId: id }, button,
          '/api/v1/jobs/' + encodeURIComponent(id) + '/rerun', {
          method: 'POST',
          body: '{}',
        });
        const queuedID = String((((data || {}).job_execution || {}).id) || '').trim();
        showJobStartedSnackbar('Job rerun started', queuedID);
        jobExecutionGraphState.terminalLoaded = false;
        return true;
      } catch (error) {
        await showAlertDialog({ title: 'Run again failed', message: 'Run again failed: ' + String(error && error.message || error) });
        return false;
      } finally {
        if (button) button.disabled = false;
      }
    }

    function latestGraphExecutions(jobNode) {
      return (Array.isArray(jobNode && jobNode.executions) ? jobNode.executions : []).filter(execution => !!execution.latest_attempt);
    }

    function graphExecutionLabel(execution) {
      return String((execution && execution.matrix_name) || '').trim() ||
        (String((execution && execution.matrix_index) || '').trim() ? ('index-' + String(execution.matrix_index)) : 'job');
    }

    function renderJobRunContext() {
      const card = document.getElementById('runContextCard');
      const body = document.getElementById('runContextBody');
      const context = jobExecutionGraphState.context;
      if (!card || !body || !context) return;
      card.style.display = '';
      const pipelines = Array.isArray(context.pipelines) ? context.pipelines : [];
      if (!pipelines.some(pipeline => pipeline.pipeline_id === jobExecutionGraphState.selectedPipelineID)) {
        jobExecutionGraphState.selectedPipelineID = String(context.current_pipeline_id || (pipelines[0] && pipelines[0].pipeline_id) || '');
        jobExecutionGraphState.selectedJobID = '';
      }
      const selectedPipeline = pipelines.find(pipeline => pipeline.pipeline_id === jobExecutionGraphState.selectedPipelineID) || pipelines[0];
      const oldPipelineViewport = document.getElementById('runContextPipelineViewport');
      const oldJobViewport = document.getElementById('runContextJobViewport');
      const oldPipelineScroll = oldPipelineViewport ? { left: oldPipelineViewport.scrollLeft, top: oldPipelineViewport.scrollTop } : null;
      const oldJobScroll = oldJobViewport ? { left: oldJobViewport.scrollLeft, top: oldJobViewport.scrollTop } : null;
      destroyOverflowTooltips(body);
      body.innerHTML = '';
      const graph = document.createElement('div');
      graph.className = 'run-context-graph';
      const viewport = document.createElement('div');
      viewport.id = 'runContextPipelineViewport';
      viewport.className = 'project-graph-viewport';
      graph.appendChild(viewport);
      const pipelineIDs = new Set(pipelines.map(pipeline => pipeline.pipeline_id));
      const nodes = pipelines.map(pipeline => ({
        id: pipeline.pipeline_id,
        label: pipeline.pipeline_id,
        dependsOn: (pipeline.depends_on || []).filter(dep => pipelineIDs.has(dep)),
        meta: String(pipeline.status || 'unknown') + ' · ' + (pipeline.jobs || []).length + ' job(s)',
        status: pipeline.status,
        runnable: Number(pipeline.pipeline_db_id || 0) > 0,
        runLabel: 'Run current definition of pipeline ' + pipeline.pipeline_id,
        runTitle: ciwiIndependentExecutionTooltip('Start a fresh run from the current project definition.', { shiftSelect: true }),
      }));
      const pipelineLayout = renderProjectDAG(viewport, nodes, {
        scale: 1,
        selectedID: selectedPipeline ? selectedPipeline.pipeline_id : '',
        emptyText: 'No run context available',
        onSelect: id => {
          jobExecutionGraphState.selectedPipelineID = id;
          jobExecutionGraphState.selectedJobID = '';
          setProjectDAGSelectedNode(viewport, id);
          const previousDetails = body.querySelector('.run-context-pipeline-details');
          if (previousDetails) {
            destroyOverflowTooltips(previousDetails);
            previousDetails.remove();
          }
          const pipeline = pipelines.find(item => item.pipeline_id === id);
          const nextDetails = pipeline ? renderJobRunContextPipeline(body, pipeline, context, null) : null;
          if (nextDetails) bindOverflowTooltips(nextDetails);
        },
        onRun: async (id, event, button) => {
          const pipeline = pipelines.find(item => item.pipeline_id === id);
          if (pipeline) await runCurrentDefinitionPipeline(event, pipeline, button);
        },
      });
      body.appendChild(graph);
      fitExecutionGraphViewport(viewport, pipelineLayout);
      if (oldPipelineScroll) {
        viewport.scrollLeft = oldPipelineScroll.left;
        viewport.scrollTop = oldPipelineScroll.top;
      }
      if (selectedPipeline) renderJobRunContextPipeline(body, selectedPipeline, context, oldJobScroll);
      bindOverflowTooltips(body);
    }

    function renderJobRunContextPipeline(body, pipeline, context, oldScroll) {
      const section = document.createElement('section');
      section.className = 'run-context-pipeline-details';
      const heading = document.createElement('h4');
      heading.innerHTML = 'Jobs in <code>' + escapeHtml(pipeline.pipeline_id || '') + '</code>';
      section.appendChild(heading);
      const jobs = Array.isArray(pipeline.jobs) ? pipeline.jobs : [];
      const jobIDs = new Set(jobs.map(job => job.pipeline_job_id));
      if (!jobIDs.has(jobExecutionGraphState.selectedJobID)) {
        jobExecutionGraphState.selectedJobID = pipeline.pipeline_id === context.current_pipeline_id && jobIDs.has(context.current_pipeline_job_id)
          ? context.current_pipeline_job_id
          : String((jobs[0] && jobs[0].pipeline_job_id) || '');
      }
      const selectedJob = jobs.find(job => job.pipeline_job_id === jobExecutionGraphState.selectedJobID) || jobs[0];
      const layout = document.createElement('div');
      layout.className = 'run-context-job-layout';
      const graph = document.createElement('div');
      graph.className = 'run-context-graph';
      const viewport = document.createElement('div');
      viewport.id = 'runContextJobViewport';
      viewport.className = 'project-graph-viewport';
      graph.appendChild(viewport);
      const nodes = jobs.map(job => ({
        id: job.pipeline_job_id,
        label: job.pipeline_job_id,
        dependsOn: (job.needs || []).filter(need => jobIDs.has(need)),
        meta: String(job.status || 'unknown') + ' · ' + latestGraphExecutions(job).length + ' execution(s)',
        status: job.status,
        runnable: latestGraphExecutions(job).length > 0,
        runLabel: 'Run job ' + job.pipeline_job_id + ' again',
        runTitle: latestGraphExecutions(job).length > 1
          ? ciwiIndependentExecutionTooltip('Choose a matrix execution to rerun.')
          : ciwiIndependentExecutionTooltip('Rerun the latest stored job execution.'),
      }));
      const selectJob = id => {
        jobExecutionGraphState.selectedJobID = id;
        setProjectDAGSelectedNode(viewport, id);
        const job = jobs.find(item => item.pipeline_job_id === id);
        const previousDetail = layout.querySelector('.run-context-job-detail');
        if (previousDetail) previousDetail.replaceWith(buildRunContextJobDetail(job, context));
      };
      const jobLayout = renderProjectDAG(viewport, nodes, {
        scale: 1,
        selectedID: selectedJob ? selectedJob.pipeline_job_id : '',
        emptyText: 'No jobs in this pipeline run',
        onSelect: selectJob,
        onRun: async (id, event, button) => {
          const job = jobs.find(item => item.pipeline_job_id === id);
          if (!job) return;
          const latest = latestGraphExecutions(job);
          if (latest.length === 1) {
            await rerunGraphExecution(latest[0], button);
          } else {
            selectJob(id);
          }
        },
      });
      layout.appendChild(graph);
      fitExecutionGraphViewport(viewport, jobLayout);
      if (oldScroll) {
        viewport.scrollLeft = oldScroll.left;
        viewport.scrollTop = oldScroll.top;
      }
      layout.appendChild(buildRunContextJobDetail(selectedJob, context));
      section.appendChild(layout);
      body.appendChild(section);
      return section;
    }

    function buildRunContextJobDetail(job, context) {
      const panel = document.createElement('div');
      panel.className = 'run-context-job-detail';
      if (!job) return panel;
      const heading = document.createElement('h4');
      heading.innerHTML = 'Job: <code>' + escapeHtml(job.pipeline_job_id || '') + '</code> <span class="run-context-status status-' + escapeHtml(job.status || 'unknown') + '">' + escapeHtml(job.status || 'unknown') + '</span>';
      panel.appendChild(heading);
      const executions = Array.isArray(job.executions) ? job.executions.slice().reverse() : [];
      executions.forEach(execution => {
        const row = document.createElement('div');
        row.className = 'run-context-execution-row';
        const info = document.createElement('div');
        info.className = 'run-context-execution-meta';
        const label = graphExecutionLabel(execution);
        info.innerHTML = '<a href="/jobs/' + encodeURIComponent(execution.id) + '?back=' + encodeURIComponent(window.location.pathname + window.location.search) + '">' + escapeHtml(label) + '</a>' +
          ' <span class="run-context-status status-' + escapeHtml(execution.status || 'unknown') + '">' + escapeHtml(execution.status || 'unknown') + '</span>' +
          (execution.id === context.current_execution_id ? ' <strong>(viewing)</strong>' : '') +
          (!execution.latest_attempt ? ' <span class="muted">older attempt</span>' : '');
        const actions = document.createElement('div');
        actions.className = 'run-context-execution-actions';
        if (execution.latest_attempt) {
          const rerun = document.createElement('button');
          rerun.type = 'button';
          rerun.className = 'secondary';
          rerun.classList.add('ciwi-icon-only');
          rerun.appendChild(ciwiIconElement('player-play'));
          rerun.setAttribute('aria-label', 'Run ' + label + ' again');
          rerun.title = 'Rerun this stored job execution';
          rerun.onclick = () => rerunGraphExecution(execution, rerun);
          actions.appendChild(rerun);
        }
        row.appendChild(info);
        row.appendChild(actions);
        panel.appendChild(row);
      });
      return panel;
    }

    async function refreshJobRunContext(job, active) {
      if (!active && jobExecutionGraphState.terminalLoaded && jobExecutionGraphState.context) return;
      const id = String((job && job.id) || jobExecutionIdFromPath()).trim();
      if (!id) return;
      try {
        const response = await fetch('/api/v1/jobs/' + encodeURIComponent(id) + '/graph-context', { cache: 'no-store' });
        if (!response.ok) throw new Error(await response.text());
        const nextContext = await response.json();
        const nextSignature = JSON.stringify(nextContext);
        jobExecutionGraphState.context = nextContext;
        jobExecutionGraphState.terminalLoaded = !active;
        if (nextSignature === jobExecutionGraphState.contextSignature) return;
        jobExecutionGraphState.contextSignature = nextSignature;
        renderJobRunContext();
      } catch (_) {
        const card = document.getElementById('runContextCard');
        if (card && !jobExecutionGraphState.context) card.style.display = 'none';
      }
    }

    function executionNavigatorStatus(group, job) {
      const item = (group && group.item) || {};
      if (group && group.step && String(group.step.kind || '') === 'dryrun_skip') return 'skipped';
      const finish = group && group.finish;
      if (finish) {
        const exitCode = finish.exit_code;
        return String(finish.error || '').trim() || (exitCode !== null && exitCode !== undefined && Number(exitCode) !== 0) ? 'failed' : 'succeeded';
      }
      const activeIndex = activeTimelineIndex(job);
      if (isRunningJobStatus((job && job.status) || '') && Number(item.index || 0) === activeIndex + 1) return 'running';
      return group && group.reached ? 'running' : 'not-reached';
    }

    function navigateToExecutionStep(key) {
      const logBox = document.getElementById('logBox');
      if (!logBox) return;
      const details = Array.from(logBox.querySelectorAll('details.log-step[data-step-key]')).find(node => String(node.getAttribute('data-step-key') || '') === key);
      if (!details) return;
      setTailingEnabled(false);
      details.open = true;
      logStepOpenState[key] = true;
      saveLogStepOpenState();
      suppressLogScrollEvent = true;
      requestAnimationFrame(() => {
        if (!details.isConnected) {
          suppressLogScrollEvent = false;
          return;
        }
        const target = details.getBoundingClientRect().top - logBox.getBoundingClientRect().top + logBox.scrollTop;
        logBox.scrollTop = Math.max(0, target - 8);
        updateLogStepCollapseButtons();
        requestAnimationFrame(() => { suppressLogScrollEvent = false; });
      });
    }

    function executionStepNavigatorSignature(groups) {
      return JSON.stringify((groups || []).map(group => [
        String(group.key || ''),
        executionGroupTitle(group),
        String(((group && group.item) || {}).kind || ''),
      ]));
    }

    function updateExecutionStepNavigatorNodes(track, groups, job) {
      const nodes = track ? track.querySelectorAll('.execution-step-node') : [];
      groups.forEach((group, index) => {
        const node = nodes[index];
        if (!node) return;
        const status = executionNavigatorStatus(group, job);
        const item = group.item || {};
        node.className = 'execution-step-node status-' + status;
        const meta = node.querySelector('.execution-step-node-meta');
        if (meta) meta.textContent = (String(item.kind || '') === 'phase' ? 'Ciwi phase' : 'Job step') + ' · ' + status.replace('-', ' ');
      });
    }

    function centerExecutionStepNavigatorOnActive(host, track, groups, job) {
      if (!tailingEnabled || !host || !track) return;
      const activeIndex = activeTimelineIndex(job);
      const active = activeIndex >= 0 ? track.querySelectorAll('.execution-step-node')[activeIndex] : null;
      if (!active) return;
      const group = groups[activeIndex] || {};
      const activeKey = String(group.key || activeIndex);
      if (host.__ciwiExecutionStepActiveKey === activeKey) return;
      host.__ciwiExecutionStepActiveKey = activeKey;
      requestAnimationFrame(() => {
        const target = Math.max(0, active.offsetLeft - (track.clientWidth - active.offsetWidth) / 2);
        if (Math.abs(track.scrollLeft - target) > 1) track.scrollLeft = target;
      });
    }

    function renderExecutionStepNavigator(job, events) {
      const host = document.getElementById('executionStepNavigator');
      if (!host) return;
      const previousScroll = host.querySelector('.execution-step-track');
      const previousScrollLeft = previousScroll ? previousScroll.scrollLeft : 0;
      const groups = structuredExecutionGroups(job, events);
      if (!groups.length) {
        destroyOverflowTooltips(host);
        host.innerHTML = '';
        host.__ciwiExecutionStepSignature = '';
        host.__ciwiExecutionStepActiveKey = '';
        host.style.display = 'none';
        return;
      }
      host.style.display = '';
      const signature = executionStepNavigatorSignature(groups);
      if (previousScroll && host.__ciwiExecutionStepSignature === signature) {
        updateExecutionStepNavigatorNodes(previousScroll, groups, job);
        centerExecutionStepNavigatorOnActive(host, previousScroll, groups, job);
        return;
      }
      destroyOverflowTooltips(host);
      host.innerHTML = '';
      const title = document.createElement('div');
      title.className = 'execution-step-navigator-title';
      title.textContent = 'Execution path';
      host.appendChild(title);
      const track = document.createElement('div');
      track.className = 'execution-step-track';
      groups.forEach((group, index) => {
        if (index > 0) {
          const arrow = document.createElement('span');
          arrow.className = 'execution-step-arrow';
          arrow.setAttribute('aria-hidden', 'true');
          arrow.textContent = '→';
          track.appendChild(arrow);
        }
        const status = executionNavigatorStatus(group, job);
        const item = group.item || {};
        const node = document.createElement('button');
        node.type = 'button';
        node.className = 'execution-step-node status-' + status;
        const nodeTitle = document.createElement('span');
        nodeTitle.className = 'execution-step-node-title';
        nodeTitle.textContent = executionGroupTitle(group);
        nodeTitle.setAttribute('data-ciwi-overflow-text', nodeTitle.textContent);
        const meta = document.createElement('span');
        meta.className = 'execution-step-node-meta';
        meta.textContent = (String(item.kind || '') === 'phase' ? 'Ciwi phase' : 'Job step') + ' · ' + status.replace('-', ' ');
        node.appendChild(nodeTitle);
        node.appendChild(meta);
        node.onclick = () => {
          if (ciwiElementContainsTextSelection(node)) return;
          navigateToExecutionStep(group.key);
        };
        track.appendChild(node);
      });
      host.appendChild(track);
      host.__ciwiExecutionStepSignature = signature;
      track.scrollLeft = previousScrollLeft;
      bindOverflowTooltips(host);
      centerExecutionStepNavigatorOnActive(host, track, groups, job);
    }

    async function renderJobExecutionGraphs(job, events, active) {
      renderExecutionStepNavigator(job, events);
      await refreshJobRunContext(job, active);
    }


    let refreshInFlight = false;
    let lastRenderedOutput = null;
    let lastOutputRaw = '';
    let lastStructuredEvents = [];
    let lastEventID = 0;
    let supplementalLoaded = false;
    let continuePolling = true;
    let pollTimer = null;
    let terminalSyncPasses = 0;
    let logStepOpenState = Object.create(null);
    const LOG_STEP_OPEN_STATE_STORAGE_PREFIX = 'ciwi.jobExecution.stepOpen.v1.';
    let tailingEnabled = true;
    let suppressLogScrollEvent = false;
    let lastCoverageSignature = null;
    let lastTestReportSignature = '';
    let lastArtifactsSignature = '';
    let artifactExpandedPaths = null;
    let logSearchController = null;
    const refreshGuard = createRefreshGuard(5000);

    function jobExecutionIdFromPath() {
      const parts = window.location.pathname.split('/').filter(Boolean);
      return parts.length >= 2 ? decodeURIComponent(parts[1]) : '';
    }

    function logStepOpenStateStorageKey() {
      const jobID = jobExecutionIdFromPath();
      return jobID ? (LOG_STEP_OPEN_STATE_STORAGE_PREFIX + jobID) : '';
    }

    function loadLogStepOpenState() {
      const state = Object.create(null);
      const storageKey = logStepOpenStateStorageKey();
      if (!storageKey) return state;
      try {
        const parsed = JSON.parse(localStorage.getItem(storageKey) || '{}');
        if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) return state;
        Object.keys(parsed).forEach(key => {
          if (parsed[key] === true || parsed[key] === false) state[key] = parsed[key];
        });
      } catch (_) {}
      return state;
    }

    function saveLogStepOpenState() {
      const storageKey = logStepOpenStateStorageKey();
      if (!storageKey) return;
      try {
        localStorage.setItem(storageKey, JSON.stringify(logStepOpenState));
      } catch (_) {}
    }

    logStepOpenState = loadLogStepOpenState();

    function parseOptionalTimestamp(ts) {
      const raw = String(ts || '').trim();
      if (!raw) return null;
      const parsed = new Date(raw);
      if (Number.isNaN(parsed.getTime())) return null;
      return parsed;
    }

    function computeJobExecutionDuration(startTs, finishTs, status) {
      const start = parseOptionalTimestamp(startTs);
      if (!start) return null;
      const running = isRunningJobStatus(status);
      const finish = parseOptionalTimestamp(finishTs);
      const end = (running || !finish) ? new Date() : finish;
      if (Number.isNaN(end.getTime())) return null;
      let ms = end.getTime() - start.getTime();
      if (ms < 0) ms = 0;
      return {
        ms: ms,
        isRunningWithoutFinish: running && !finish,
      };
    }

    function formatJobExecutionDuration(startTs, finishTs, status) {
      const duration = computeJobExecutionDuration(startTs, finishTs, status);
      if (!duration) return '';
      const core = formatDurationMs(duration.ms);
      if (!core) return '';
      if (duration.isRunningWithoutFinish) return core + ' (running)';
      return core;
    }

    function setBackLink() {
      const link = document.getElementById('backLink');
      if (!link) return;
      const params = new URLSearchParams(window.location.search || '');
      const back = params.get('back') || '';
      if (back && back.startsWith('/')) {
        link.href = back;
        link.innerHTML = '<span class="nav-emoji" aria-hidden="true">' + ciwiIconHTML('arrow-left') + '</span> ' + (back.startsWith('/projects/') ? 'Back to Project' : 'Back to Job Executions');
        return;
      }
      link.href = '/';
      link.innerHTML = '<span class="nav-emoji" aria-hidden="true">' + ciwiIconHTML('arrow-left') + '</span> Back to Job Executions';
    }

    function activeTimelineIndex(job) {
      const text = String((job && job.current_step) || '').trim();
      if (!text) return -1;
      const timeline = Array.isArray(job && job.execution_timeline) ? job.execution_timeline : [];
      let m = text.match(/^Job step\s+(\d+)(?:\/\d+)?(?:\s*:|$)/i);
      if (m) {
        const stepIndex = Number.parseInt(String(m[1] || '').trim(), 10);
        const item = timeline.find(entry =>
          String((entry && entry.kind) || '') === 'step' &&
          Number((entry && entry.step_index) || 0) === stepIndex
        );
        if (item) return Number(item.index || 0) - 1;
      }
      m = text.match(/^Ciwi phase\s+(\d+)(?:\/\d+)?(?:\s*:|$)/i);
      if (m) {
        const phaseIndex = Number.parseInt(String(m[1] || '').trim(), 10);
        const phases = timeline.filter(entry => String((entry && entry.kind) || '') === 'phase');
        const item = phaseIndex > 0 ? phases[phaseIndex - 1] : null;
        if (item) return Number(item.index || 0) - 1;
      }
      return -1;
    }

    function subtitleStepDetail(job) {
      const stepPlan = Array.isArray(job && job.step_plan) ? job.step_plan : [];
      const idx = activeTimelineIndex(job);
      if (idx < 0) return '';
      const timeline = Array.isArray(job && job.execution_timeline) ? job.execution_timeline : [];
      const activeItem = timeline.find(item => Number((item && item.index) || 0) === idx + 1);
      if (activeItem && String(activeItem.kind || '') !== 'step') return '';
      const stepIndex = activeItem ? Number(activeItem.step_index || 0) : idx + 1;
      const step = stepPlan.find(item => Number((item && item.index) || 0) === stepIndex) || {};
      const script = String(step.script || '').trim();
      if (script) return script.replace(/\s+/g, ' ');
      const kind = String(step.kind || '').trim();
      const testName = String(step.test_name || '').trim();
      if (kind === 'test' && testName) return 'test ' + testName;
      if (kind === 'dryrun_skip') return 'skipped during dry run';
      return '';
    }

    function renderProjectIcon(projectID) {
      const icon = document.getElementById('jobProjectIcon');
      if (!icon) return;
      const id = String(projectID || '').trim();
      if (!id) {
        icon.style.display = 'none';
        return;
      }
      icon.src = '/api/v1/projects/' + encodeURIComponent(id) + '/icon';
      icon.onload = () => { icon.style.display = 'inline-block'; };
      icon.onerror = () => { icon.style.display = 'none'; };
    }
    function logUnreachedBoundary(el) {
      if (!el) return null;
      const firstUnreached = el.querySelector('details.log-step-unreached');
      if (!firstUnreached) return null;
      return firstUnreached.getBoundingClientRect().top - el.getBoundingClientRect().top + el.scrollTop;
    }

    function isNearLogBottom() {
      const el = document.getElementById('logBox');
      if (!el) return true;
      const leewayPx = 48;
      const viewportBottom = el.scrollTop + el.clientHeight;
      const unreachedBoundary = logUnreachedBoundary(el);
      if (unreachedBoundary !== null) {
        // Scrolling beyond live output means the user is browsing the planned,
        // unreached portion of the timeline. Polling must not pull them back.
        if (viewportBottom > unreachedBoundary + 4) return false;
        return viewportBottom >= unreachedBoundary - leewayPx;
      }
      return viewportBottom >= (el.scrollHeight - leewayPx);
    }

    function scrollLogToBottom() {
      const el = document.getElementById('logBox');
      if (!el) return;
      suppressLogScrollEvent = true;
      const unreachedBoundary = logUnreachedBoundary(el);
      if (unreachedBoundary !== null) {
        el.scrollTop = Math.max(0, unreachedBoundary - el.clientHeight);
      } else {
        el.scrollTop = el.scrollHeight;
      }
      setTimeout(() => { suppressLogScrollEvent = false; }, 0);
    }

    function setTailingEnabled(enabled) {
      const wasTailing = tailingEnabled;
      tailingEnabled = !!enabled;
      if (!wasTailing && tailingEnabled) {
        const navigator = document.getElementById('executionStepNavigator');
        if (navigator) navigator.__ciwiExecutionStepActiveKey = '';
      }
      const btn = document.getElementById('tailToggleBtn');
      if (!btn) return;
      btn.textContent = tailingEnabled ? 'Tailing: On' : 'Tailing: Off';
      btn.classList.toggle('tail-on', tailingEnabled);
      btn.classList.toggle('tail-off', !tailingEnabled);
    }

    function wireLogControls() {
      const logBox = document.getElementById('logBox');
      if (logBox && !logBox.__ciwiTailingBound) {
        logBox.__ciwiTailingBound = true;
        logBox.addEventListener('scroll', () => {
          if (suppressLogScrollEvent) return;
          if (isNearLogBottom()) {
            setTailingEnabled(true);
          } else {
            setTailingEnabled(false);
          }
        });
      }

      const tailBtn = document.getElementById('tailToggleBtn');
      if (tailBtn && !tailBtn.__ciwiBound) {
        tailBtn.__ciwiBound = true;
        tailBtn.addEventListener('click', () => {
          setTailingEnabled(!tailingEnabled);
          if (tailingEnabled) {
            scrollLogToBottom();
          }
        });
      }

      const copyBtn = document.getElementById('copyOutputBtn');
      if (copyBtn && !copyBtn.__ciwiBound) {
        copyBtn.__ciwiBound = true;
        copyBtn.addEventListener('click', async () => {
          const text = String(lastOutputRaw || '');
          const old = copyBtn.textContent;
          try {
            if (navigator.clipboard && typeof navigator.clipboard.writeText === 'function') {
              await navigator.clipboard.writeText(text);
            } else {
              throw new Error('navigator.clipboard unavailable');
            }
            copyBtn.textContent = 'Copied';
          } catch (primaryErr) {
            try {
              const ta = document.createElement('textarea');
              ta.value = text;
              ta.setAttribute('readonly', '');
              ta.style.position = 'fixed';
              ta.style.left = '-9999px';
              ta.style.top = '0';
              document.body.appendChild(ta);
              ta.focus();
              ta.select();
              const ok = document.execCommand && document.execCommand('copy');
              document.body.removeChild(ta);
              if (!ok) throw new Error('execCommand copy returned false');
              copyBtn.textContent = 'Copied';
            } catch (fallbackErr) {
              console.warn('Copy output failed', primaryErr, fallbackErr);
              copyBtn.textContent = 'Copy failed';
            }
          }
          setTimeout(() => { copyBtn.textContent = old; }, 1200);
        });
      }
      if (!logSearchController) {
        logSearchController = createTextSearchController({
          scopeEl: document.getElementById('logBox'),
          inputEl: document.getElementById('logSearchInput'),
          prevBtn: document.getElementById('logSearchPrevBtn'),
          nextBtn: document.getElementById('logSearchNextBtn'),
          countEl: document.getElementById('logSearchCount'),
        });
      }
      bindLogInfoTooltips();
      setTailingEnabled(tailingEnabled);
    }

    function bindLogInfoTooltips() {
      document.querySelectorAll('.log-info[data-log-info]').forEach(el => {
        if (el.__ciwiHoverTooltip) return;
        const kind = String(el.getAttribute('data-log-info') || '').trim();
        const tooltipHTML = kind === 'raw'
          ? '<strong>Raw log</strong><br />Downloads the redacted structured event stream with ANSI escape sequences preserved.'
          : '<strong>Clean log</strong><br />Downloads an editor-friendly plain text log generated from structured events. ANSI escape sequences and terminal control characters are stripped.';
        createHoverTooltip(el, { html: tooltipHTML, lingerMs: 2000, owner: 'log-info-' + kind });
      });
    }

    function bindLogStepToggles() {
      const logBox = document.getElementById('logBox');
      if (!logBox) return;
      logBox.querySelectorAll('details.log-step[data-step-key]').forEach(d => {
        const key = String(d.getAttribute('data-step-key') || '').trim();
        if (!key) return;
        if (!d.__ciwiStepToggleBound) {
          d.__ciwiStepToggleBound = true;
          d.addEventListener('toggle', () => {
            logStepOpenState[key] = !!d.open;
            saveLogStepOpenState();
            if (d.classList.contains('log-step-unreached')) {
              setTailingEnabled(false);
            }
            requestAnimationFrame(updateLogStepCollapseButtons);
          });
        }
        const collapseBtn = d.querySelector(':scope > .log-step-collapse-btn');
        if (collapseBtn && !collapseBtn.__ciwiBound) {
          collapseBtn.__ciwiBound = true;
          collapseBtn.addEventListener('click', () => {
            const headerTop = d.getBoundingClientRect().top - logBox.getBoundingClientRect().top + logBox.scrollTop;
            d.open = false;
            requestAnimationFrame(() => {
              suppressLogScrollEvent = true;
              logBox.scrollTop = Math.max(0, headerTop - 8);
              setTimeout(() => { suppressLogScrollEvent = false; }, 0);
            });
          });
        }
      });
      if (!logBox.__ciwiCollapseResizeBound) {
        logBox.__ciwiCollapseResizeBound = true;
        window.addEventListener('resize', updateLogStepCollapseButtons);
      }
      requestAnimationFrame(updateLogStepCollapseButtons);
    }

    function updateLogStepCollapseButtons() {
      const logBox = document.getElementById('logBox');
      if (!logBox) return;
      const largeStepThreshold = Math.max(480, logBox.clientHeight);
      logBox.querySelectorAll('details.log-step[data-step-key]').forEach(d => {
        const collapseBtn = d.querySelector(':scope > .log-step-collapse-btn');
        if (!collapseBtn) return;
        const summary = d.querySelector(':scope > summary');
        const contentHeight = d.open ? Math.max(0, d.scrollHeight - Number((summary && summary.offsetHeight) || 0)) : 0;
        collapseBtn.hidden = !d.open || contentHeight <= largeStepThreshold;
      });
    }

    async function loadJobExecution(force) {
      if (refreshInFlight || (!force && refreshGuard.shouldPause())) {
        scheduleJobExecutionRefresh(1000);
        return;
      }
      refreshInFlight = true;
      const jobId = jobExecutionIdFromPath();
      if (!jobId) {
        document.getElementById('subtitle').textContent = 'Missing job id';
        refreshInFlight = false;
        return;
      }

      try {
        const res = await fetch('/api/v1/jobs/' + encodeURIComponent(jobId), { cache: 'no-store' });
        if (!res.ok) {
          document.getElementById('subtitle').textContent = 'Failed to load job';
          return;
        }
        const data = await res.json();
        const job = data.job_execution || {};
        bindCiwiProgress(document.getElementById('jobHeaderCard'), job);
        let newEvents = [];
        let eventRefreshSucceeded = false;
        try {
          const eventsURL = '/api/v1/jobs/' + encodeURIComponent(jobId) + '/events?after_id=' + encodeURIComponent(String(lastEventID));
          const evRes = await fetch(eventsURL, { cache: 'no-store' });
          if (evRes.ok) {
            const evData = await evRes.json();
            newEvents = Array.isArray(evData.events) ? evData.events : [];
            if (newEvents.length) {
              lastStructuredEvents = lastStructuredEvents.concat(newEvents);
            }
            const nextEventID = Number(evData.next_event_id || 0);
            if (Number.isFinite(nextEventID) && nextEventID >= lastEventID) {
              lastEventID = nextEventID;
            }
            eventRefreshSucceeded = true;
          }
        } catch (_) {}
        const events = lastStructuredEvents;
        const cleanBtn = document.getElementById('downloadCleanLogBtn');
        const rawBtn = document.getElementById('downloadRawLogBtn');
        if (cleanBtn) cleanBtn.href = '/api/v1/jobs/' + encodeURIComponent(jobId) + '/log?format=clean';
        if (rawBtn) rawBtn.href = '/api/v1/jobs/' + encodeURIComponent(jobId) + '/log?format=raw';

        const metaSource = (job && job.metadata) || {};
        const projectName = String(metaSource.project || '').trim();
        const projectID = String(metaSource.project_id || '').trim();
        const pipelineJobID = String(metaSource.pipeline_job_id || '').trim();
        document.getElementById('jobTitle').textContent = jobExecutionTitle(job);
        renderProjectIcon(projectID);

        const pipeline = String(metaSource.pipeline_id || '').trim();
        const dryRun = String(metaSource.dry_run || '').trim() === '1';
        const buildVersion = buildVersionLabel(job);
        const rows = [
          { label: 'Job Execution ID', value: escapeHtml(job.id || '') },
          { label: 'Project', value: escapeHtml(projectName) },
          { label: 'Job ID', value: escapeHtml(pipelineJobID) },
          { label: 'Pipeline', value: escapeHtml(pipeline) },
          { label: 'Mode', valueHTML: renderModeValue(dryRun) },
          { label: 'Build', value: escapeHtml(buildVersion) },
          { label: 'Agent', value: escapeHtml(job.leased_by_agent_id || '') },
          { label: 'Created', value: escapeHtml(formatTimestamp(job.created_utc)) },
          { label: 'Started', value: escapeHtml(formatTimestamp(job.started_utc)) },
          { label: 'Duration', value: escapeHtml(formatJobExecutionDuration(job.started_utc, job.finished_utc, job.status)) },
          { label: 'Exit Code', value: (job.exit_code === null || job.exit_code === undefined) ? '' : String(job.exit_code) },
        ];
        renderCacheStats(job.cache_stats);
        renderSchedulingDiagnosis(job.scheduling_diagnosis);
        renderToolRequirements(job.required_capabilities, job.runtime_capabilities, job.status);

        const meta = document.getElementById('metaGrid');
        const metaHTML = rows.map(r =>
          '<div class="label">' + r.label + '</div><div' + (r.valueId ? ' id="' + r.valueId + '"' : '') + '>' + (r.valueHTML || r.value || '') + '</div>'
        ).join('');
        if (meta.__ciwiHTML !== metaHTML) {
          meta.querySelectorAll('.mode-info').forEach(el => {
            if (el.__ciwiHoverTooltip && typeof el.__ciwiHoverTooltip.destroy === 'function') {
              el.__ciwiHoverTooltip.destroy();
            }
          });
          meta.innerHTML = metaHTML;
          meta.__ciwiHTML = metaHTML;
          const modeInfo = meta.querySelector('.mode-info');
          if (modeInfo) {
            const mode = String(modeInfo.getAttribute('data-mode') || '').trim();
            const tooltipHTML = mode === 'dry'
              ? 'Dry run executes the job plan but skips steps marked <code>skip_dry_run</code>. This is useful to avoid pushing tags, commits, and artifacts to repositories. Ciwi does not automagically detect such writes, so make sure your ciwi YAML files use <code>skip_dry_run</code> where needed. See <a href="https://github.com/izzyreal/ciwi/blob/main/ciwi-project.yaml" target="_blank" rel="noopener">ciwi\'s own YAML</a> for example usage.'
              : 'Ordinary run executes all configured steps, including side-effecting steps such as publish/release.';
            createHoverTooltip(modeInfo, { html: tooltipHTML, lingerMs: 2000, owner: 'mode-info' });
          }
        }

        const eventSignature = JSON.stringify(events.map(ev => [ev.id, ev.type, ev.timestamp_utc, ev.step && ev.step.index, ev.message, ev.output, ev.error, ev.duration_ms]));
        const hasStructured = hasStructuredLogEvents(events);
        const renderSignature = 'structured:' + eventSignature + ':' + String(job.current_step || '') + ':' + String(job.status || '');
        lastOutputRaw = plainTextFromStructuredEvents(job, events);
        if (renderSignature !== lastRenderedOutput) {
          const logBox = document.getElementById('logBox');
          if (typeof destroyOverflowTooltips === 'function') {
            destroyOverflowTooltips(logBox);
          }
          logBox.innerHTML = renderStructuredOutputLog(job, events);
          bindLogStepToggles();
          if (hasStructured) bindStructuredStepProgress(job, events);
          if (typeof bindOverflowTooltips === 'function') {
            bindOverflowTooltips(logBox, { ownerPrefix: 'log-step-command' });
          }
          lastRenderedOutput = renderSignature;
          if (logSearchController && typeof logSearchController.refresh === 'function') {
            logSearchController.refresh();
          }
          if (tailingEnabled) {
            requestAnimationFrame(scrollLogToBottom);
          }
        }
        const stepDescription = String(job.current_step || '').trim();
        let subtitle = 'Status: <span class="' + statusClassForJob(job) + '">' + escapeHtml(formatJobStatus(job)) + '</span>';
        const waitingReason = jobWaitingReason(job);
        if (waitingReason) {
          subtitle += '<div class="job-subtitle-detail">' + escapeHtml(waitingReason) + '</div>';
        }
        if (stepDescription) {
          subtitle += ' <span class="label"> - ' + escapeHtml(stepDescription) + '</span>';
        }
        const stepDetail = subtitleStepDetail(job);
        if (stepDetail) {
          subtitle += '<div class="job-subtitle-detail">Command: <code data-ciwi-overflow-text="' + escapeHtml(stepDetail) + '">' + escapeHtml(stepDetail) + '</code></div>';
        }
        const subtitleElement = document.getElementById('subtitle');
        if (subtitleElement.__ciwiHTML !== subtitle) {
          if (typeof destroyOverflowTooltips === 'function') {
            destroyOverflowTooltips(subtitleElement);
          }
          subtitleElement.innerHTML = subtitle;
          subtitleElement.__ciwiHTML = subtitle;
          if (typeof bindOverflowTooltips === 'function') {
            bindOverflowTooltips(subtitleElement, { ownerPrefix: 'job-subtitle-command' });
          }
        }

      const forceBtn = document.getElementById('forceFailBtn');
      const active = isActiveJobStatus(job.status);
      if (active) terminalSyncPasses = 0;
      await renderJobExecutionGraphs(job, events, active);
      if (active) {
        forceBtn.style.display = 'inline-block';
        forceBtn.disabled = false;
        forceBtn.onclick = async () => {
          const confirmed = await showConfirmDialog({
            title: 'Cancel Job',
            message: 'Cancel this active job?',
            okLabel: 'Cancel job',
          });
          if (!confirmed) return;
          forceBtn.disabled = true;
          try {
            await apiActionJSON('cancel-execution', { jobExecutionId: jobId }, forceBtn,
              '/api/v1/jobs/' + encodeURIComponent(jobId) + '/cancel', {
              method: 'POST',
              body: '{}'
            });
            await loadJobExecution(true);
          } catch (e) {
            await showAlertDialog({ title: 'Cancel failed', message: 'Cancel failed: ' + e.message });
          } finally {
            forceBtn.disabled = false;
          }
        };
      } else {
        forceBtn.style.display = 'none';
      }

      const rerunBtn = document.getElementById('rerunBtn');
      const rerunBlockedLink = document.getElementById('rerunBlockedLink');
      const hasStarted = !!String(job.started_utc || '').trim();
      rerunBtn.disabled = !hasStarted;
      rerunBtn.title = hasStarted ? '' : 'Job must have started at least once';
      if (rerunBlockedLink) {
        rerunBlockedLink.style.display = 'none';
        rerunBlockedLink.removeAttribute('href');
        rerunBlockedLink.textContent = 'Open failed dependency';
      }
      if (!hasStarted && isDependencyBlockedJob(job) && rerunBlockedLink) {
		rerunBtn.disabled = false;
		rerunBtn.title = '';
        try {
          const bres = await fetch('/api/v1/jobs/' + encodeURIComponent(jobId) + '/blocked-by', { cache: 'no-store' });
          if (bres.ok) {
            const bdata = await bres.json();
            const dep = (bdata && bdata.dependency) || null;
            const depID = String((dep && dep.job_execution_id) || '').trim();
            if (depID) {
              const backTo = encodeURIComponent(window.location.pathname + window.location.search);
              rerunBlockedLink.href = '/jobs/' + encodeURIComponent(depID) + '?back=' + backTo;
              rerunBlockedLink.style.display = 'inline';
              const depJob = String((dep && dep.pipeline_job_id) || '').trim();
              const depMatrix = String((dep && dep.matrix_name) || '').trim();
              let label = depJob;
              if (depMatrix) {
                label = depJob ? (depJob + ' / ' + depMatrix) : depMatrix;
              }
              if (label) {
                rerunBlockedLink.textContent = 'Open failed dependency: ' + label;
              }
            }
          }
        } catch (_) {}
      }
      const rerunInfo = document.getElementById('rerunInfo');
      if (rerunInfo && !rerunInfo.__ciwiHoverTooltip) {
        const tooltipHTML = '' +
          '<strong>What Run Job Again does</strong><br />' +
          'It enqueues a new attempt with the same script, requirements, source repo/ref, and step plan as this run. Pipeline and chain jobs remain part of their original run and refresh their upstream artifact bindings, allowing failed runs to be repaired in place.<br /><br />' +
          '<strong>Ongoing executions</strong><br />' +
          'The new attempt does not cancel, pause, replace, or otherwise change queued or running executions. It may run concurrently with them.<br /><br />' +
          '<strong>Source checkout behavior</strong><br />' +
          'Rerun keeps the same pinned source commit as the original queued job.<br /><br />' +
          '<strong>Artifacts and logs</strong><br />' +
          'Rerun creates a fresh job execution ID with fresh logs and artifact records. Previous job artifacts are kept; they are not replaced.<br /><br />' +
          '<strong>When this is useful</strong><br />' +
          'Use it to quickly retry flaky failures, rerun after agent/tool fixes, or rerun a one-off job without re-enqueueing an entire pipeline.';
        createHoverTooltip(rerunInfo, { html: tooltipHTML, lingerMs: 2000, owner: 'rerun-info' });
      }
      rerunBtn.onclick = async () => {
        if (rerunBtn.disabled) return;
        rerunBtn.disabled = true;
        const old = rerunBtn.textContent;
        rerunBtn.textContent = 'Queueing...';
        try {
          const data = await apiActionJSON('rerun-execution', { jobExecutionId: jobId }, rerunBtn,
            '/api/v1/jobs/' + encodeURIComponent(jobId) + '/rerun', {
            method: 'POST',
            body: '{}',
          });
          const enqueuedID = String((((data || {}).job_execution || {}).id) || '').trim();
          const metaSource = job.metadata || {};
          const projectName = String(metaSource.project || '').trim() || 'Project';
          const matrixName = String(metaSource.matrix_name || '').trim();
          const pipelineName = String(metaSource.pipeline_id || '').trim();
          const shortName = matrixName || pipelineName || 'job';
          showJobStartedSnackbar(projectName + ' ' + shortName + ' started', enqueuedID);
        } catch (e) {
          await showAlertDialog({ title: 'Run again failed', message: 'Run again failed: ' + String(e.message || e) });
        } finally {
          rerunBtn.textContent = old;
          rerunBtn.disabled = !hasStarted;
        }
      };

        renderReleaseSummary(job);

      const shouldLoadSupplemental = !supplementalLoaded || !active;
      let supplementalSucceeded = true;
      if (shouldLoadSupplemental) try {
        const ares = await fetch('/api/v1/jobs/' + encodeURIComponent(jobId) + '/artifacts', { cache: 'no-store' });
        if (!ares.ok) {
          throw new Error('artifact request failed');
        }
        const adata = await ares.json();
        const box = document.getElementById('artifactsBox');
        const items = adata.artifacts || [];
        renderArtifacts(box, jobId, items);
      } catch (_) {
        supplementalSucceeded = false;
        document.getElementById('artifactsBox').textContent = 'Could not load artifacts';
      }

      if (shouldLoadSupplemental) try {
        const tres = await fetch('/api/v1/jobs/' + encodeURIComponent(jobId) + '/tests', { cache: 'no-store' });
        if (!tres.ok) throw new Error('test report request failed');
        const tdata = await tres.json();
        const report = tdata.report || {};
        const coverage = report.coverage || null;
        const coverageSignature = coverage ? JSON.stringify(coverage) : '';
        if (coverageSignature !== lastCoverageSignature) {
          renderCoverageReport(coverage);
          lastCoverageSignature = coverageSignature;
        }
        const testSignature = JSON.stringify(report);
        if (testSignature !== lastTestReportSignature) {
          renderTestReport(report, job);
          lastTestReportSignature = testSignature;
        }
      } catch (_) {
        supplementalSucceeded = false;
        document.getElementById('testReportBox').textContent = 'Could not load test report';
        document.getElementById('coverageReportBox').textContent = 'Could not load coverage report';
        lastCoverageSignature = null;
        lastTestReportSignature = '';
      }
      if (shouldLoadSupplemental && supplementalSucceeded) supplementalLoaded = true;
      if (active) {
        continuePolling = true;
      } else if (eventRefreshSucceeded && supplementalSucceeded) {
        terminalSyncPasses += 1;
        continuePolling = terminalSyncPasses < 2;
      } else {
        continuePolling = true;
      }
      } finally {
        refreshInFlight = false;
        if (continuePolling) scheduleJobExecutionRefresh(1000);
      }
    }

    function scheduleJobExecutionRefresh(delayMs) {
      if (!continuePolling || pollTimer !== null) return;
      pollTimer = setTimeout(() => {
        pollTimer = null;
        loadJobExecution(false);
      }, delayMs);
    }
    setBackLink();
    initializeJobRunContextCard();
    wireLogControls();
    refreshGuard.bindSelectionListener();
    loadJobExecution(true);
