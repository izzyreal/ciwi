
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
        projectGraphState.fitOnRender = false;
        requestAnimationFrame(() => requestAnimationFrame(fitProjectGraph));
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

    let refreshInFlight = false;
    const refreshGuard = createRefreshGuard(5000);
    let inspectModalState = null;

    function projectIdFromPath() {
      const parts = window.location.pathname.split('/').filter(Boolean);
      return parts.length >= 2 ? parts[1] : '';
    }
    function setBackLink() {
      const link = document.getElementById('backLink');
      if (!link) return;
      const params = new URLSearchParams(window.location.search || '');
      const back = String(params.get('back') || '').trim();
      if (back && back.startsWith('/')) {
        link.href = back;
        link.innerHTML = '<span class="nav-emoji" aria-hidden="true">' + ciwiIconHTML('arrow-left') + '</span> ' + (back === '/settings' ? 'Back to Global Settings' : 'Back to Projects');
        return;
      }
      link.href = '/';
      link.innerHTML = '<span class="nav-emoji" aria-hidden="true">' + ciwiIconHTML('arrow-left') + '</span> Back to Projects';
    }
    let currentProjectName = '';
    let currentProjectID = 0;

    function ensureProjectInspectModal() {
      let overlay = document.getElementById('projectInspectOverlay');
      if (overlay) return overlay;
      ensureModalBaseStyles();
      overlay = document.createElement('div');
      overlay.id = 'projectInspectOverlay';
      overlay.className = 'ciwi-modal-overlay';
      overlay.setAttribute('aria-hidden', 'true');
      overlay.innerHTML = '' +
        '<div class="ciwi-modal" role="dialog" aria-modal="true" aria-label="Inspect pipeline or job">' +
          '<div class="ciwi-modal-head">' +
            '<div>' +
              '<div class="ciwi-modal-title" id="projectInspectTitle">Inspect</div>' +
              '<div class="ciwi-modal-subtitle" id="projectInspectSubtitle"></div>' +
            '</div>' +
            '<button type="button" class="secondary" id="projectInspectCloseBtn">Close</button>' +
          '</div>' +
          '<div class="ciwi-modal-body">' +
            '<div class="inspect-toolbar">' +
              '<label for="projectInspectView" class="muted">View:</label>' +
              '<select id="projectInspectView" class="inspect-select">' +
                '<option value="raw_yaml">Raw YAML</option>' +
                '<option value="executor_script">Executor script</option>' +
                '<option value="secret_mappings">Secret mappings</option>' +
              '</select>' +
              '<label class="inspect-checkbox">' +
                '<input id="projectInspectDryRun" type="checkbox" />' +
                '<span>Dry run</span>' +
              '</label>' +
              '<label class="inspect-checkbox">' +
                '<input id="projectInspectTestSecrets" type="checkbox" />' +
                '<span>Test secrets against Vault</span>' +
              '</label>' +
            '</div>' +
            '<pre id="projectInspectContent" class="inspect-content">Loading...</pre>' +
          '</div>' +
        '</div>';
      document.body.appendChild(overlay);
      wireModalCloseBehavior(overlay, closeProjectInspectModal);
      const closeBtn = document.getElementById('projectInspectCloseBtn');
      if (closeBtn) closeBtn.onclick = closeProjectInspectModal;
      const viewSelect = document.getElementById('projectInspectView');
      if (viewSelect) {
        viewSelect.onchange = () => {
          if (!inspectModalState) return;
          inspectModalState.view = String(viewSelect.value || 'raw_yaml').trim() || 'raw_yaml';
          syncProjectInspectControls();
          loadProjectInspectContent();
        };
      }
      const dryRunInput = document.getElementById('projectInspectDryRun');
      const testSecretsInput = document.getElementById('projectInspectTestSecrets');
      if (dryRunInput) {
        dryRunInput.onchange = () => {
          if (!inspectModalState) return;
          inspectModalState.dryRun = !!dryRunInput.checked;
          loadProjectInspectContent();
        };
      }
      if (testSecretsInput) {
        testSecretsInput.onchange = () => {
          if (!inspectModalState) return;
          inspectModalState.testSecrets = !!testSecretsInput.checked;
          loadProjectInspectContent();
        };
      }
      return overlay;
    }

    function closeProjectInspectModal() {
      inspectModalState = null;
      const overlay = document.getElementById('projectInspectOverlay');
      closeModalOverlay(overlay);
    }

    function openProjectInspectModal(req, title, subtitle) {
      inspectModalState = {
        req: req || {},
        view: 'raw_yaml',
        dryRun: false,
        testSecrets: false,
        title: String(title || 'Inspect').trim(),
        subtitle: String(subtitle || '').trim(),
      };
      const overlay = ensureProjectInspectModal();
      const titleEl = document.getElementById('projectInspectTitle');
      const subtitleEl = document.getElementById('projectInspectSubtitle');
      const viewSelect = document.getElementById('projectInspectView');
      const dryRunInput = document.getElementById('projectInspectDryRun');
      const testSecretsInput = document.getElementById('projectInspectTestSecrets');
      if (titleEl) titleEl.textContent = inspectModalState.title;
      if (subtitleEl) subtitleEl.textContent = inspectModalState.subtitle;
      if (viewSelect) viewSelect.value = inspectModalState.view;
      if (dryRunInput) dryRunInput.checked = inspectModalState.dryRun;
      if (testSecretsInput) testSecretsInput.checked = inspectModalState.testSecrets;
      syncProjectInspectControls();
      openModalOverlay(overlay, '900px', '78vh');
      loadProjectInspectContent();
    }

    function syncProjectInspectControls() {
      const dryRunInput = document.getElementById('projectInspectDryRun');
      const testSecretsInput = document.getElementById('projectInspectTestSecrets');
      if (!inspectModalState) return;
      const currentView = String(inspectModalState.view || '').trim();
      if (dryRunInput) {
        const isScript = currentView === 'executor_script';
        dryRunInput.disabled = !isScript;
      }
      if (testSecretsInput) {
        const isMappings = currentView === 'secret_mappings';
        testSecretsInput.disabled = !isMappings;
      }
    }

    async function loadProjectInspectContent() {
      if (!inspectModalState) return;
      const contentEl = document.getElementById('projectInspectContent');
      if (!contentEl) return;
      contentEl.textContent = 'Loading...';
      const req = {
        ...(inspectModalState.req || {}),
        view: inspectModalState.view || 'raw_yaml',
        dry_run: !!inspectModalState.dryRun,
        test_secrets: !!inspectModalState.testSecrets,
      };
      try {
        const data = await apiJSON('/api/v1/projects/' + encodeURIComponent(String(currentProjectID || '')) + '/inspect', {
          method: 'POST',
          body: JSON.stringify(req),
        });
        const payload = data || {};
        const title = String(payload.title || '').trim();
        const content = String(payload.content || '').trim();
        if (title) {
          const titleEl = document.getElementById('projectInspectTitle');
          if (titleEl) titleEl.textContent = title;
        }
        contentEl.textContent = content || '(empty)';
      } catch (e) {
        contentEl.textContent = 'Failed to load: ' + String(e && e.message || e);
      }
    }

    function pipelineSupportsDryRun(pipeline) {
      return (pipeline.jobs || []).some(job =>
        (job.steps || []).some(step => !!step.skip_dry_run)
      );
    }

    async function runProjectPipeline(event, pipeline, button) {
      if (button) button.disabled = true;
      try {
        const runResult = await runWithOptionalSourceRef(event, {
          runPath: '/api/v1/pipelines/' + pipeline.id + '/run-selection',
          sourceRefsPath: '/api/v1/pipelines/' + pipeline.id + '/source-refs',
          eligibleAgentsPath: '/api/v1/pipelines/' + pipeline.id + '/eligible-agents',
          payload: {},
          title: 'Run Pipeline With Source Ref',
          subtitle: String(pipeline.pipeline_id || ''),
          runLabel: 'Run',
        });
        if (runResult.cancelled) return false;
        showQueuedJobsSnackbar((currentProjectName || 'Project') + ' ' + (pipeline.pipeline_id || 'pipeline') + ' started');
        await loadHistory();
      } catch (e) {
        await showAlertDialog({ title: 'Run failed', message: 'Run failed: ' + e.message });
      } finally {
        if (button) button.disabled = false;
      }
    }

    function appendPipelineActionControls(container, pipeline) {
      const runBtn = document.createElement('button');
      runBtn.textContent = 'Run Pipeline';
      runBtn.className = 'secondary';
      runBtn.onclick = ev => runProjectPipeline(ev, pipeline, runBtn);
      container.appendChild(runBtn);

      if (pipelineSupportsDryRun(pipeline)) {
        const dryBtn = document.createElement('button');
        dryBtn.textContent = 'Dry Run Pipeline';
        dryBtn.className = 'secondary';
        dryBtn.onclick = async (ev) => {
          dryBtn.disabled = true;
          try {
            const runResult = await runWithOptionalSourceRef(ev, {
              runPath: '/api/v1/pipelines/' + pipeline.id + '/run-selection',
              sourceRefsPath: '/api/v1/pipelines/' + pipeline.id + '/source-refs',
              eligibleAgentsPath: '/api/v1/pipelines/' + pipeline.id + '/eligible-agents',
              payload: { dry_run: true },
              title: 'Dry Run Pipeline With Source Ref',
              subtitle: String(pipeline.pipeline_id || ''),
              runLabel: 'Dry Run',
            });
            if (runResult.cancelled) return;
            showQueuedJobsSnackbar((currentProjectName || 'Project') + ' ' + (pipeline.pipeline_id || 'pipeline') + ' started');
            await loadHistory();
          } catch (e) {
            await showAlertDialog({ title: 'Dry run failed', message: 'Dry run failed: ' + e.message });
          } finally {
            dryBtn.disabled = false;
          }
        };
        container.appendChild(dryBtn);
      }

      const previewBtn = document.createElement('button');
      previewBtn.textContent = 'Execution Plan';
      previewBtn.className = 'secondary';
      previewBtn.onclick = () => {
        openDryRunPreviewModal({
          title: 'Execution Plan',
          subtitle: String(pipeline.pipeline_id || ''),
          previewPath: '/api/v1/pipelines/' + pipeline.id + '/dry-run-preview',
          runPath: '/api/v1/pipelines/' + pipeline.id + '/run-selection',
          sourceRefsPath: '/api/v1/pipelines/' + pipeline.id + '/source-refs',
          eligibleAgentsPath: '/api/v1/pipelines/' + pipeline.id + '/eligible-agents',
          payload: { dry_run: true },
        });
      };
      container.appendChild(previewBtn);

      const resolveBtn = document.createElement('button');
      resolveBtn.textContent = 'Resolve Upcoming Build Version';
      resolveBtn.className = 'secondary';
      resolveBtn.onclick = () => openVersionResolveModal(pipeline.id, pipeline.pipeline_id);
      container.appendChild(resolveBtn);

      const inspectBtn = document.createElement('button');
      inspectBtn.textContent = 'Inspect Pipeline';
      inspectBtn.className = 'secondary';
      inspectBtn.onclick = () => openProjectInspectModal(
        { pipeline_db_id: pipeline.id },
        'Pipeline ' + (pipeline.pipeline_id || ''),
        'Preview raw YAML or rendered executor scripts'
      );
      container.appendChild(inspectBtn);
    }

    async function runProjectJobSelection(event, pipeline, payload, successName, errorPrefix, modalTitle, label, button) {
      if (button) button.disabled = true;
      try {
        const runResult = await runWithOptionalSourceRef(event, {
          runPath: '/api/v1/pipelines/' + pipeline.id + '/run-selection',
          sourceRefsPath: '/api/v1/pipelines/' + pipeline.id + '/source-refs',
          eligibleAgentsPath: '/api/v1/pipelines/' + pipeline.id + '/eligible-agents',
          payload: payload,
          title: modalTitle,
          subtitle: String(pipeline.pipeline_id || ''),
          runLabel: label,
        });
        if (runResult.cancelled) return false;
        const response = runResult.response || {};
        const ids = Array.isArray(response.job_execution_ids) ? response.job_execution_ids : [];
        if (ids.length === 1) {
          showJobStartedSnackbar((currentProjectName || 'Project') + ' ' + successName + ' started', ids[0]);
        } else {
          showQueuedJobsSnackbar((currentProjectName || 'Project') + ' ' + successName + ' started');
        }
        await loadHistory();
        return true;
      } catch (e) {
        await showAlertDialog({ title: errorPrefix, message: errorPrefix + ': ' + e.message });
        return false;
      } finally {
        if (button) button.disabled = false;
      }
    }

    function createJobRunActionButton(pipeline, job, label, payload, successName, errorPrefix, modalTitle) {
      const btn = document.createElement('button');
      btn.textContent = label;
      btn.className = 'secondary';
      btn.onclick = ev => runProjectJobSelection(ev, pipeline, payload, successName, errorPrefix, modalTitle, label, btn);
      return btn;
    }

    function openProjectMatrixRunChooser(pipeline, job) {
      ensureModalBaseStyles();
      let overlay = document.getElementById('projectMatrixRunOverlay');
      if (!overlay) {
        overlay = document.createElement('div');
        overlay.id = 'projectMatrixRunOverlay';
        overlay.className = 'ciwi-modal-overlay';
        overlay.setAttribute('aria-hidden', 'true');
        overlay.innerHTML = '' +
          '<div class="ciwi-modal" role="dialog" aria-modal="true" aria-label="Choose matrix entry">' +
            '<div class="ciwi-modal-head"><div><div class="ciwi-modal-title" id="projectMatrixRunTitle">Run matrix job</div><div class="ciwi-modal-subtitle">Choose one matrix entry</div></div><button type="button" class="secondary" id="projectMatrixRunClose">Close</button></div>' +
            '<div class="ciwi-modal-body"><div id="projectMatrixRunChoices" class="project-matrix-chooser"></div></div>' +
          '</div>';
        document.body.appendChild(overlay);
        wireModalCloseBehavior(overlay, () => closeModalOverlay(overlay));
        document.getElementById('projectMatrixRunClose').onclick = () => closeModalOverlay(overlay);
      }
      document.getElementById('projectMatrixRunTitle').textContent = 'Run ' + String(job.id || 'matrix job');
      const choices = document.getElementById('projectMatrixRunChoices');
      choices.innerHTML = '';
      (job.matrix_includes || []).forEach(include => {
        const item = document.createElement('div');
        item.className = 'matrix-item';
        const name = String(include.name || '').trim() || ('index-' + include.index);
        const vars = Object.entries(include.vars || {}).map(kv => kv[0] + '=' + kv[1]).join(', ');
        const info = document.createElement('div');
        info.className = 'matrix-info';
        info.innerHTML = '<div><code>' + escapeHtml(name) + '</code></div><div class="muted">' + escapeHtml(vars) + '</div>';
        const run = document.createElement('button');
        run.type = 'button';
        run.className = 'secondary';
        run.innerHTML = ciwiIconHTML('player-play') + '<span>Run</span>';
        run.onclick = async event => {
          const queued = await runProjectJobSelection(event, pipeline, { pipeline_job_id: job.id, matrix_index: include.index }, name, 'Run selection failed', 'Run Matrix Entry With Source Ref', 'Run', run);
          if (queued) closeModalOverlay(overlay);
        };
        item.appendChild(info);
        item.appendChild(run);
        choices.appendChild(item);
      });
      openModalOverlay(overlay, 'min(620px,94vw)', 'auto');
    }

    function appendJobActionControls(container, pipeline, job, actionsOnly) {
      const supportsDryRun = (job.steps || []).some(step => !!step.skip_dry_run);
      const includes = Array.isArray(job.matrix_includes) ? job.matrix_includes : [];
      if (includes.length > 0) {
        const matrixList = document.createElement('div');
        matrixList.className = 'matrix-list';
        includes.forEach(include => {
          const item = document.createElement('div');
          item.className = 'matrix-item';
          const name = (include.name || '').trim() || ('index-' + include.index);
          const vars = Object.entries(include.vars || {}).map(kv => kv[0] + '=' + kv[1]).join(', ');
          const info = document.createElement('div');
          info.className = 'matrix-info';
          info.innerHTML = '<div><code>' + escapeHtml(name) + '</code></div><div class="muted">' + escapeHtml(vars) + '</div>';
          const actions = document.createElement('div');
          actions.className = 'matrix-actions';
          actions.appendChild(createJobRunActionButton(pipeline, job, 'Run', { pipeline_job_id: job.id, matrix_index: include.index }, name, 'Run selection failed', 'Run Matrix Entry With Source Ref'));
          if (supportsDryRun) {
            actions.appendChild(createJobRunActionButton(pipeline, job, 'Dry Run', { pipeline_job_id: job.id, matrix_index: include.index, dry_run: true }, name, 'Dry run selection failed', 'Dry Run Matrix Entry With Source Ref'));
          }
          const previewBtn = document.createElement('button');
          previewBtn.textContent = 'Execution Plan';
          previewBtn.className = 'secondary';
          previewBtn.onclick = () => openDryRunPreviewModal({
            title: 'Execution Plan',
            subtitle: String(pipeline.pipeline_id || '') + ' / ' + String(job.id || '') + ' / ' + name,
            previewPath: '/api/v1/pipelines/' + pipeline.id + '/dry-run-preview',
            runPath: '/api/v1/pipelines/' + pipeline.id + '/run-selection',
            sourceRefsPath: '/api/v1/pipelines/' + pipeline.id + '/source-refs',
            eligibleAgentsPath: '/api/v1/pipelines/' + pipeline.id + '/eligible-agents',
            payload: { dry_run: true, pipeline_job_id: job.id, matrix_index: include.index },
          });
          actions.appendChild(previewBtn);
          const inspectBtn = document.createElement('button');
          inspectBtn.textContent = 'Inspect';
          inspectBtn.className = 'secondary';
          inspectBtn.onclick = () => openProjectInspectModal(
            { pipeline_db_id: pipeline.id, pipeline_job_id: job.id, matrix_index: include.index },
            'Job ' + (job.id || ''),
            'Matrix ' + name
          );
          actions.appendChild(inspectBtn);
          item.appendChild(info);
          item.appendChild(actions);
          matrixList.appendChild(item);
        });
        container.appendChild(matrixList);
        return;
      }

      const actions = actionsOnly ? container : document.createElement('div');
      if (!actionsOnly) actions.className = 'project-job-detail-actions';
      actions.appendChild(createJobRunActionButton(pipeline, job, 'Run Job', { pipeline_job_id: job.id }, (job.id || 'job'), 'Run selection failed', 'Run Job With Source Ref'));
      if (supportsDryRun) {
        actions.appendChild(createJobRunActionButton(pipeline, job, 'Dry Run Job', { pipeline_job_id: job.id, dry_run: true }, (job.id || 'job'), 'Dry run selection failed', 'Dry Run Job With Source Ref'));
      }
      const previewBtn = document.createElement('button');
      previewBtn.textContent = 'Execution Plan';
      previewBtn.className = 'secondary';
      previewBtn.onclick = () => openDryRunPreviewModal({
        title: 'Execution Plan',
        subtitle: String(pipeline.pipeline_id || '') + ' / ' + String(job.id || ''),
        previewPath: '/api/v1/pipelines/' + pipeline.id + '/dry-run-preview',
        runPath: '/api/v1/pipelines/' + pipeline.id + '/run-selection',
        sourceRefsPath: '/api/v1/pipelines/' + pipeline.id + '/source-refs',
        eligibleAgentsPath: '/api/v1/pipelines/' + pipeline.id + '/eligible-agents',
        payload: { dry_run: true, pipeline_job_id: job.id },
      });
      actions.appendChild(previewBtn);
      const inspectBtn = document.createElement('button');
      inspectBtn.textContent = 'Inspect Job';
      inspectBtn.className = 'secondary';
      inspectBtn.onclick = () => openProjectInspectModal(
        { pipeline_db_id: pipeline.id, pipeline_job_id: job.id },
        'Job ' + (job.id || ''),
        'Preview raw YAML or rendered executor script'
      );
      actions.appendChild(inspectBtn);
      if (!actionsOnly) container.appendChild(actions);
    }

    async function loadProject() {
      const id = projectIdFromPath();
      if (!id) return;
      const data = await apiJSON('/api/v1/projects/' + encodeURIComponent(id));
      const p = data.project;
      currentProjectID = p.id;
      currentProjectName = p.name || '';
      document.getElementById('title').textContent = p.name || 'Project';
      document.getElementById('subtitle').innerHTML = projectSourceMetadataHTML(p);
      const icon = document.getElementById('projectIcon');
      icon.src = '/api/v1/projects/' + encodeURIComponent(String(p.id || '')) + '/icon';
      icon.onload = () => { icon.style.display = 'inline-block'; };
      icon.onerror = () => { icon.style.display = 'none'; };

      const structure = document.getElementById('structure');
      const pipelines = Array.isArray(p.pipelines) ? p.pipelines : [];
      const chains = Array.isArray(p.pipeline_chains) ? p.pipeline_chains : [];
      if (pipelines.length === 0 && chains.length === 0) {
        const toggle = document.getElementById('structureViewToggle');
        const graph = document.getElementById('structureGraph');
        if (toggle) toggle.hidden = true;
        if (graph) graph.hidden = true;
        structure.hidden = false;
        structure.innerHTML = '<div class="muted">No pipelines</div>';
        return;
      }

      structure.innerHTML = '';
      if (chains.length > 0) {
        const chainBlock = document.createElement('div');
        chainBlock.className = 'pipeline';
        const chainTitle = document.createElement('div');
        chainTitle.className = 'muted';
        chainTitle.style.marginBottom = '8px';
        chainTitle.innerHTML = '<strong>Pipeline Chains</strong>';
        chainBlock.appendChild(chainTitle);
        chains.forEach(ch => {
          const chainRow = document.createElement('div');
          chainRow.className = 'jobbox';
          const chainHead = document.createElement('div');
          chainHead.className = 'job-head';
          const chainDesc = document.createElement('div');
          chainDesc.className = 'job-desc';
          const chainActions = document.createElement('div');
          chainActions.className = 'job-actions';
          const chainPipes = pipelineChainSequence(ch);
          const chainName = pipelineChainDisplayName(ch);
          chainDesc.innerHTML =
            '<div><strong>Chain: ' + pipelineChainDisplayHTML(ch) + '</strong></div>' +
            (chainName !== chainPipes ? ('<div class="muted">' + pipelineChainSequenceHTML(ch) + '</div>') : '');
          chainHead.appendChild(chainDesc);
          chainHead.appendChild(chainActions);
          chainRow.appendChild(chainHead);

          const runBtn = document.createElement('button');
          runBtn.textContent = 'Run Chain';
          runBtn.className = 'secondary';
          runBtn.onclick = async (ev) => {
            runBtn.disabled = true;
            try {
              const runResult = await runWithOptionalSourceRef(ev, {
                runPath: pipelineChainAPIPath(p.id, ch.id, 'run'),
                sourceRefsPath: pipelineChainAPIPath(p.id, ch.id, 'source-refs'),
                eligibleAgentsPath: pipelineChainAPIPath(p.id, ch.id, 'eligible-agents'),
                payload: {},
                title: 'Run Chain With Source Ref',
                subtitle: chainName,
                runLabel: 'Run Chain',
              });
              if (runResult.cancelled) return;
              showQueuedJobsSnackbar((currentProjectName || 'Project') + ' ' + chainName + ' started');
              await loadHistory();
            } catch (e) {
              await showAlertDialog({ title: 'Run failed', message: 'Run failed: ' + e.message });
            } finally {
              runBtn.disabled = false;
            }
          };
          chainActions.appendChild(runBtn);

          if (ch.supports_dry_run) {
            const dryBtn = document.createElement('button');
            dryBtn.textContent = 'Dry Run Chain';
            dryBtn.className = 'secondary';
            dryBtn.onclick = async (ev) => {
              dryBtn.disabled = true;
              try {
                const runResult = await runWithOptionalSourceRef(ev, {
                  runPath: pipelineChainAPIPath(p.id, ch.id, 'run'),
                  sourceRefsPath: pipelineChainAPIPath(p.id, ch.id, 'source-refs'),
                  eligibleAgentsPath: pipelineChainAPIPath(p.id, ch.id, 'eligible-agents'),
                  payload: { dry_run: true },
                  title: 'Dry Run Chain With Source Ref',
                  subtitle: chainName,
                  runLabel: 'Dry Run Chain',
                });
                if (runResult.cancelled) return;
                showQueuedJobsSnackbar((currentProjectName || 'Project') + ' ' + chainName + ' started');
                await loadHistory();
              } catch (e) {
                await showAlertDialog({ title: 'Dry run failed', message: 'Dry run failed: ' + e.message });
              } finally {
                dryBtn.disabled = false;
              }
            };
            chainActions.appendChild(dryBtn);
          }

          const previewBtn = document.createElement('button');
          previewBtn.textContent = 'Execution Plan';
          previewBtn.className = 'secondary';
          previewBtn.onclick = () => {
            openDryRunPreviewModal({
              title: 'Execution Plan',
              subtitle: chainName,
              previewPath: pipelineChainAPIPath(p.id, ch.id, 'dry-run-preview'),
              runPath: pipelineChainAPIPath(p.id, ch.id, 'run'),
              sourceRefsPath: pipelineChainAPIPath(p.id, ch.id, 'source-refs'),
              eligibleAgentsPath: pipelineChainAPIPath(p.id, ch.id, 'eligible-agents'),
              payload: { dry_run: true },
            });
          };
          chainActions.appendChild(previewBtn);

          chainBlock.appendChild(chainRow);
        });
        structure.appendChild(chainBlock);
      }

      pipelines.forEach(pl => {
        const container = document.createElement('div');
        container.className = 'pipeline';
        const head = document.createElement('div');
        head.className = 'pipeline-head';
        const deps = (pl.depends_on || []).join(', ');
        const versioning = pl.versioning || {};
        const vparts = [];
        if (versioning.file) vparts.push('file=' + versioning.file);
        if (versioning.tag_prefix) vparts.push('tag_prefix=' + versioning.tag_prefix);
        if (versioning.auto_bump) vparts.push('auto_bump=' + versioning.auto_bump);
        const headMeta = document.createElement('div');
        headMeta.className = 'pipeline-meta';
        headMeta.innerHTML = '<strong>Pipeline: <code>' + escapeHtml(pl.pipeline_id) + '</code></strong>' +
          (deps ? ('<span class="muted">depends_on: ' + escapeHtml(deps) + '</span>') : '') +
          (vparts.length > 0 ? ('<span class="muted">versioning: ' + escapeHtml(vparts.join(', ')) + '</span>') : '');
        head.appendChild(headMeta);
        const headControls = document.createElement('div');
        headControls.className = 'pipeline-controls';
        appendPipelineActionControls(headControls, pl);
        const toggleBtn = document.createElement('button');
        toggleBtn.textContent = 'Collapse';
        toggleBtn.className = 'secondary';
        toggleBtn.onclick = () => {
          const collapsed = container.classList.toggle('collapsed');
          toggleBtn.textContent = collapsed ? 'Expand' : 'Collapse';
        };
        headControls.appendChild(toggleBtn);
        head.appendChild(headControls);
        container.appendChild(head);
        const pipelineBody = document.createElement('div');
        pipelineBody.className = 'pipeline-body';

        (pl.jobs || []).forEach(j => {
          const jb = document.createElement('div');
          jb.className = 'jobbox';
          const jobHead = document.createElement('div');
          jobHead.className = 'job-head';
          const jobDesc = document.createElement('div');
          jobDesc.className = 'job-desc';
          const jobActions = document.createElement('div');
          jobActions.className = 'job-actions';
          const runsOn = Object.entries(j.runs_on || {}).map(kv => kv[0] + '=' + kv[1]).join(', ');
          const requiresTools = Object.entries(j.requires_tools || {}).map(kv => kv[0] + '=' + (kv[1] || '*')).join(', ');
          jobDesc.innerHTML =
            '<div><strong>Job: ' + escapeHtml(j.id || '') + '</strong> <span class="muted">timeout=' + (j.timeout_seconds || 0) + 's</span></div>' +
            '<div class="muted">runs_on: ' + escapeHtml(runsOn) + '</div>' +
            '<div class="muted">requires.tools: ' + escapeHtml(requiresTools) + '</div>';
          jobHead.appendChild(jobDesc);
          jobHead.appendChild(jobActions);
          jb.appendChild(jobHead);
          if (Array.isArray(j.matrix_includes) && j.matrix_includes.length > 0) {
            appendJobActionControls(jb, pl, j);
          } else {
            appendJobActionControls(jobActions, pl, j, true);
          }
          pipelineBody.appendChild(jb);
        });
        container.appendChild(pipelineBody);
        structure.appendChild(container);
      });
      initializeProjectGraph(p);
    }

    async function loadHistory(force) {
      if (refreshInFlight || (!force && refreshGuard.shouldPause())) {
        return;
      }
      refreshInFlight = true;
      try {
        const data = await apiJSON('/api/v1/jobs');
        const body = document.getElementById('historyBody');
        body.innerHTML = '';
        const projectID = String(currentProjectID || '').trim();
        const rows = (data.job_executions || []).filter(j => {
          const metadata = (j && j.metadata) || {};
          return String(metadata.project_id || '').trim() === projectID;
        }).slice(0, 120);
        rows.forEach(job => {
          const tr = buildJobExecutionRow(job, {
            includeActions: false,
            includeDuration: true,
            backPath: window.location.pathname || '/'
          });
          body.appendChild(tr);
        });
      } finally {
        refreshInFlight = false;
      }
    }

    async function tick() {
      try {
        await refreshRuntimeStateBanner('runtimeStateBanner');
        await loadProject();
        await loadHistory(true);
      } catch (e) {
        document.getElementById('subtitle').textContent = 'Failed to load project: ' + e.message;
      }
    }

    refreshGuard.bindSelectionListener();
    setBackLink();
    tick();
    setInterval(() => {
      refreshRuntimeStateBanner('runtimeStateBanner');
      loadHistory(false);
    }, 4000);
