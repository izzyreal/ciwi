(function () {
  'use strict';

  window.ciwiCreateBrowserViewBindings = function ({getCurrentData}) {
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
	const suppliedProject = view && view.project && typeof view.project === 'object' ? view.project : {};
	const project = Object.assign({
	  id: Number(view && view.project_id || 0), name: 'Project', project_icon: '',
	  repo_url: '', repo_ref: '', config_file: '', pipeline_chains: [], has_pipeline_chains: false,
	}, suppliedProject);
	view.project = project;
	const previousProject = getCurrentData() && getCurrentData().projectDetails && getCurrentData().projectDetails.project;
	const previousFilter = previousProject && String(previousProject.id) === String(project.id)
	  ? String(getCurrentData().projectDetails.structure_filter || 'all-pipelines')
	  : 'all-pipelines';
	project.project_icon = Number(project.id || 0) > 0 ? '/api/v1/projects/' + encodeURIComponent(project.id) + '/icon' : '';
	view.loading = false;
	view.ready = true;
	view.load_error = '';
	applyProjectStructureFilter(view, previousFilter);
    }

    function projectDetailsLoadingBinding(projectID) {
	const id = String(projectID || '');
	const currentProject = getCurrentData() && getCurrentData().projectDetails && getCurrentData().projectDetails.project;
	const frontProjects = getCurrentData() && getCurrentData().frontPage && Array.isArray(getCurrentData().frontPage.projects)
	  ? getCurrentData().frontPage.projects : [];
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

    return {
      decorateFrontPageProjects,
      decorateProjectDetails,
      browserLoadingBinding,
      browserClientBinding,
      markBrowserViewReady,
      decorateJobDetails,
      applyProjectStructureFilter,
      managedYAMLBinding,
      agentScriptBinding,
      vaultBinding,
      declarativeExecutionTimestamp,
    };
  };
})();
