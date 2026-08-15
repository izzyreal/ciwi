//go:build darwin || ios || linux || windows

package gio

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/izzyreal/ciwi/internal/presentation"
	"github.com/izzyreal/ciwi/internal/protocol"
	cnpv1 "github.com/izzyreal/ciwi/pkg/cnp/v1"
	"github.com/izzyreal/ciwi/pkg/cnpclient"
	"github.com/izzyreal/ciwi/pkg/uidsl"
	sharedUI "github.com/izzyreal/ciwi/ui"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

func loadScreenData(ctx context.Context, client *cnpclient.Client, navigation navigationState, themeName string) (map[string]any, error) {
	switch navigation.screen {
	case "front-page":
		requestCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
		defer cancel()
		view, err := client.GetFrontPageView(requestCtx)
		if err != nil {
			return nil, err
		}
		return frontPageBindingData(view)
	case "project-details":
		requestCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
		defer cancel()
		view, err := client.GetProjectDetails(requestCtx, navigation.projectID)
		if err != nil {
			return nil, err
		}
		return projectDetailsBindingData(view)
	case "job-details":
		requestCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
		defer cancel()
		view, err := client.GetJobDetails(requestCtx, navigation.jobID)
		if err != nil {
			return nil, err
		}
		return jobDetailsBindingData(view)
	case "settings":
		return loadSettingsData(ctx, client, themeName)
	case "managed-yaml":
		return loadManagedYAMLData(ctx, client, navigation.projectID)
	case "run-options":
		return loadRunOptions(ctx, client, navigation)
	case "agents":
		requestCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
		defer cancel()
		view, err := client.GetAgentsView(requestCtx)
		if err != nil {
			return nil, err
		}
		return protobufBindingData("agents", "agents", view)
	case "agent-details":
		requestCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
		defer cancel()
		view, err := client.GetAgentDetails(requestCtx, navigation.agentDetailsID)
		if err != nil {
			return nil, err
		}
		return protobufBindingData("agentDetails", "agent details", view)
	case "agent-script":
		return loadAgentScriptData(ctx, client, navigation)
	case "vault":
		return loadVaultData(ctx, client)
	case "connection":
		return map[string]any{}, nil
	default:
		return nil, fmt.Errorf("screen %q is unsupported", navigation.screen)
	}
}

func loadManagedYAMLData(ctx context.Context, client *cnpclient.Client, projectID int64) (map[string]any, error) {
	if projectID <= 0 {
		return managedYAMLBindingData(nil), nil
	}
	requestCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	definition, err := client.GetManagedYAML(requestCtx, projectID)
	if err != nil {
		return nil, err
	}
	return managedYAMLBindingData(definition), nil
}

func managedYAMLBindingData(definition *cnpv1.ManagedYAMLDefinition) map[string]any {
	projectID, name, raw, revision := int64(0), "New managed project", "", ""
	editing := definition != nil && definition.ProjectId > 0
	if definition != nil {
		projectID, name, raw, revision = definition.ProjectId, definition.ProjectName, definition.Yaml, definition.Revision
	}
	title := "Add Managed YAML"
	if editing {
		title = "Edit Managed YAML"
	}
	return map[string]any{"managedYAML": map[string]any{
		"title": title, "project_id": projectID, "project_name": name, "yaml": raw, "revision": revision, "editing": editing,
		"result": "", "result_tone": "muted", "loading": false, "ready": true, "load_error": "",
	}}
}

func loadSettingsData(ctx context.Context, client *cnpclient.Client, themeName string) (map[string]any, error) {
	requestCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	server, err := client.GetServerInfo(requestCtx)
	if err != nil {
		return nil, err
	}
	projects, err := client.ListProjects(requestCtx)
	if err != nil {
		return nil, err
	}
	updateStatus, updateStatusErr := client.GetServerUpdateStatus(requestCtx)
	themes, err := sharedUI.LoadThemes()
	if err != nil {
		return nil, err
	}
	data, err := settingsBindingData(server, themes, themeName)
	if err != nil {
		return nil, err
	}
	settings, ok := data["settings"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("settings binding is malformed")
	}
	projectData, err := protobufBindingData("projects", "settings projects", projects)
	if err != nil {
		return nil, err
	}
	projectRoot, ok := projectData["projects"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("settings projects binding is malformed")
	}
	projectItems, _ := projectRoot["projects"].([]any)
	decorateSettingsProjects(projectItems)
	settings["projects"] = projectItems
	decorateSettingsUpdate(settings, updateStatus, updateStatusErr)
	return data, nil
}

func loadAgentScriptData(ctx context.Context, client *cnpclient.Client, navigation navigationState) (map[string]any, error) {
	requestCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	view, err := client.GetAgentDetails(requestCtx, navigation.agentScriptID)
	if err != nil {
		return nil, err
	}
	shells := make([]any, 0, len(view.GetAgent().GetScriptShells()))
	for _, shell := range view.GetAgent().GetScriptShells() {
		shells = append(shells, map[string]any{
			"value": shell.GetValue(), "label": shell.GetLabel(), "example_script": shell.GetExampleScript(),
		})
	}
	selectedShell := strings.TrimSpace(navigation.scriptShell)
	script := navigation.script
	if selectedShell == "" && len(view.GetAgent().GetScriptShells()) > 0 {
		selectedShell = view.GetAgent().GetScriptShells()[0].GetValue()
	}
	if script == "" {
		script = presentation.ExampleAgentScript(selectedShell)
	}
	return map[string]any{"agentScript": map[string]any{
		"agent_id": navigation.agentScriptID, "agent_label": view.GetAgent().GetHostname(),
		"shells": shells, "selected_shell": selectedShell, "script": script,
		"can_run": view.GetAgent().GetCanRunScript() && selectedShell != "", "result": "", "result_tone": "muted",
		"loading": false, "ready": true, "load_error": "",
	}}, nil
}

func loadVaultData(ctx context.Context, client *cnpclient.Client) (map[string]any, error) {
	requestCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	connections, err := client.ListVaultConnections(requestCtx)
	if err != nil {
		return nil, err
	}
	return vaultBindingData(connections)
}

func vaultBindingData(connections *cnpv1.VaultConnectionList) (map[string]any, error) {
	data, err := protobufBindingData("vault", "Vault connections", connections)
	if err != nil {
		return nil, err
	}
	root, ok := data["vault"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("Vault binding is malformed")
	}
	items, _ := root["connections"].([]any)
	root["connections_empty"] = len(items) == 0
	root["name"] = "home-vault"
	root["url"] = ""
	root["role_id"] = ""
	root["approle_mount"] = "approle"
	root["secret_id_env"] = "CIWI_VAULT_SECRET_ID"
	root["result"] = ""
	root["result_tone"] = "muted"
	return data, nil
}

func loadRunOptions(ctx context.Context, client *cnpclient.Client, navigation navigationState) (map[string]any, error) {
	requestCtx, cancel := context.WithTimeout(ctx, 70*time.Second)
	defer cancel()
	view, err := client.GetRunOptions(requestCtx, &cnpv1.GetRunOptionsRequest{
		PipelineDbId: navigation.pipelineDBID, ProjectId: navigation.projectID, ChainId: navigation.chainID,
		Selection: &cnpv1.RunPipelineSelection{SourceRef: navigation.sourceRef, AgentId: navigation.agentID},
	})
	if err != nil {
		return nil, err
	}
	data, err := protobufBindingData("runOptions", "run-options", view)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func runOptionsLoadingData(navigation navigationState) map[string]any {
	return map[string]any{"runOptions": map[string]any{
		"target_kind": "loading", "target_label": "Loading…", "pipeline_db_id": navigation.pipelineDBID,
		"project_id": navigation.projectID, "chain_id": navigation.chainID, "supports_dry_run": false,
		"source_repo": "Fetching source branches and eligible agents…", "default_source_ref": "",
		"selected_source_ref": navigation.sourceRef, "selected_agent_id": navigation.agentID,
		"source_refs": []any{}, "eligible_agents": []any{}, "pending_jobs": float64(0),
	}}
}

func screenLoadingData(navigation navigationState, clientVersion, themeName, mode, endpoint string, sshSettings sshConnectionSettings) (map[string]any, error) {
	data, err := screenLoadingBindingData(navigation, clientVersion, themeName, mode, endpoint, sshSettings)
	if err != nil {
		return nil, err
	}
	setScreenLifecycle(data, navigation.screen, true, false, "")
	return data, nil
}

func screenLoadingBindingData(navigation navigationState, clientVersion, themeName, mode, endpoint string, sshSettings sshConnectionSettings) (map[string]any, error) {
	switch navigation.screen {
	case "front-page":
		return offlineFrontPageBindingData()
	case "project-details":
		data, err := projectDetailsBindingData(&cnpv1.ProjectDetailsView{
			Project: &cnpv1.ProjectSummary{Id: navigation.projectID},
		})
		if err != nil {
			return nil, err
		}
		root := data["projectDetails"].(map[string]any)
		root["loading"] = true
		root["ready"] = false
		root["load_error"] = ""
		root["structure_filter"] = "all-pipelines"
		root["visible_pipelines"] = []any{}
		root["structure_root"] = map[string]any{
			"id": fmt.Sprintf("project:%d:loading", navigation.projectID), "label": "Project", "meta": "",
			"runnable": false, "project_id": float64(navigation.projectID), "chain_id": "",
		}
		root["show_chain_structure"] = false
		root["show_pipeline_structure"] = false
		if project, ok := root["project"].(map[string]any); ok {
			project["name"] = "Project"
		}
		return data, nil
	case "job-details":
		return jobDetailsBindingData(&cnpv1.JobDetailsView{})
	case "settings":
		return offlineSettingsBindingData(clientVersion, themeName, mode, endpoint, sshSettings)
	case "managed-yaml":
		return managedYAMLBindingData(nil), nil
	case "run-options":
		return runOptionsLoadingData(navigation), nil
	case "agents":
		return protobufBindingData("agents", "agents", &cnpv1.AgentsView{})
	case "agent-details":
		return protobufBindingData("agentDetails", "agent details", &cnpv1.AgentDetailsView{
			Agent: &cnpv1.AgentSummary{Id: navigation.agentDetailsID},
		})
	case "agent-script":
		return map[string]any{"agentScript": map[string]any{
			"agent_id": navigation.agentScriptID, "agent_label": navigation.agentScriptID,
			"shells": []any{}, "selected_shell": "", "script": "", "can_run": false, "result": "", "result_tone": "muted",
		}}, nil
	case "vault":
		return vaultBindingData(&cnpv1.VaultConnectionList{})
	default:
		return nil, fmt.Errorf("screen %q is unavailable", navigation.screen)
	}
}

func seedProjectDetailsLoadingData(data, frontPage map[string]any, projectID int64) {
	root, _ := data["projectDetails"].(map[string]any)
	project, _ := root["project"].(map[string]any)
	frontRoot, _ := frontPage["frontPage"].(map[string]any)
	projects, _ := frontRoot["projects"].([]any)
	for _, raw := range projects {
		candidate, ok := raw.(map[string]any)
		if !ok || int64(numberValue(candidate["id"])) != projectID {
			continue
		}
		for _, key := range []string{
			"id", "name", "project_icon", "project_icon_content_type", "repo_url", "repo_ref", "config_file",
		} {
			if value, exists := candidate[key]; exists {
				project[key] = cloneBindingValue(value)
			}
		}
		return
	}
}

func decorateSettingsUpdate(settings map[string]any, status *cnpv1.ServerUpdateStatus, statusErr error) {
	settings["update_versions"] = versionOptions(nil, "Check for updates")
	settings["selected_update_version"] = ""
	settings["rollback_versions"] = versionOptions(nil, "Refresh versions")
	settings["selected_rollback_version"] = ""
	settings["update_result"] = ""
	settings["update_result_tone"] = "muted"
	settings["rollback_result"] = ""
	settings["rollback_result_tone"] = "muted"
	if statusErr != nil || status == nil {
		settings["update_supported"] = false
		settings["update_capability_notice"] = "Update status unavailable"
		settings["update_status_label"] = ""
		return
	}
	settings["update_supported"] = status.SelfUpdateSupported
	settings["update_current_version"] = status.CurrentVersion
	settings["update_last_apply_status"] = status.LastApplyStatus
	notice := strings.TrimSpace(status.SelfUpdateReason)
	if status.ServerMode == "dev" {
		notice = "Running in dev mode. Updates disabled."
	} else if !status.SelfUpdateSupported && notice == "" {
		notice = "Server self-updates are unavailable in this installation."
	}
	settings["update_capability_notice"] = notice
	parts := []string{}
	if status.CurrentVersion != "" {
		parts = append(parts, "Current: "+status.CurrentVersion)
	}
	if status.LatestVersion != "" {
		parts = append(parts, "Latest: "+status.LatestVersion)
	}
	if status.UpdateAvailable && status.CurrentVersion != status.LatestVersion {
		parts = append(parts, "Update available")
	}
	if status.LastApplyStatus != "" {
		parts = append(parts, "Apply: "+status.LastApplyStatus)
	}
	if status.Message != "" {
		parts = append(parts, "Message: "+status.Message)
	}
	settings["update_status_label"] = strings.Join(parts, " · ")
	settings["blocked_agent_notice"] = ""
	if len(status.BlockedAgentIds) > 0 {
		settings["blocked_agent_notice"] = "Agents requiring manual update: " + strings.Join(status.BlockedAgentIds, ", ")
	}
}

func versionOptions(versions []string, emptyLabel string) []any {
	if len(versions) == 0 {
		return []any{map[string]any{"value": "", "label": emptyLabel}}
	}
	result := make([]any, 0, len(versions))
	for _, version := range versions {
		if version = strings.TrimSpace(version); version != "" {
			result = append(result, map[string]any{"value": version, "label": version})
		}
	}
	if len(result) == 0 {
		return []any{map[string]any{"value": "", "label": emptyLabel}}
	}
	return result
}

func decorateSettingsProjects(projects []any) {
	for _, item := range projects {
		project, ok := item.(map[string]any)
		if !ok {
			continue
		}
		project["action_status"] = ""
		project["action_tone"] = "muted"
		project["updated_label"] = formatLoadedProjectTime(project["updated_unix_ms"])
	}
}

func formatLoadedProjectTime(value any) string {
	var milliseconds int64
	switch typed := value.(type) {
	case float64:
		milliseconds = int64(typed)
	case float32:
		milliseconds = int64(typed)
	case int64:
		milliseconds = typed
	case int:
		milliseconds = int64(typed)
	default:
		milliseconds, _ = strconv.ParseInt(strings.TrimSpace(fmt.Sprint(value)), 10, 64)
	}
	if milliseconds <= 0 {
		return "Unknown"
	}
	return time.UnixMilli(milliseconds).Local().Format("Mon 02 Jan, 15:04:05")
}

func frontPageBindingData(view *cnpv1.FrontPageView) (map[string]any, error) {
	data, err := protobufBindingData("frontPage", "front-page", view)
	if err != nil {
		return nil, err
	}
	root, ok := data["frontPage"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("front-page binding is malformed")
	}
	ensureExecutionCardBindings(root["queued_executions"])
	ensureExecutionCardBindings(root["history_executions"])
	queued, _ := root["queued_executions"].([]any)
	history, _ := root["history_executions"].([]any)
	root["queued_empty"] = len(queued) == 0
	root["history_empty"] = len(history) == 0
	root["loading"] = false
	return data, nil
}

func ensureExecutionCardBindings(value any) {
	cards, ok := value.([]any)
	if !ok {
		return
	}
	for _, card := range cards {
		entry, entryOK := card.(map[string]any)
		if !entryOK {
			continue
		}
		if sections, ok := entry["sections"].([]any); ok {
			for _, rawSection := range sections {
				section, _ := rawSection.(map[string]any)
				jobs, _ := section["jobs"].([]any)
				for _, rawJob := range jobs {
					job, _ := rawJob.(map[string]any)
					ensureSchedulingDiagnosisBinding(job)
				}
			}
		}
	}
}

func ensureSchedulingDiagnosisBinding(value map[string]any) {
	if value == nil {
		return
	}
	if _, ok := value["scheduling_diagnosis"].(map[string]any); ok {
		return
	}
	value["scheduling_diagnosis"] = map[string]any{
		"state": "", "summary": "", "requirements": []any{}, "requirements_label": "",
		"agents": []any{}, "additional_agents_label": "",
	}
}

func numberValue(value any) float64 {
	number, _ := value.(float64)
	return number
}

func boolValue(value any) bool {
	result, _ := value.(bool)
	return result
}

func projectDetailsBindingData(view *cnpv1.ProjectDetailsView) (map[string]any, error) {
	data, err := protobufBindingData("projectDetails", "project-details", view)
	if err != nil {
		return nil, err
	}
	root, ok := data["projectDetails"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("project-details binding is malformed")
	}
	decorateProjectDetails(root)
	ensureExecutionCardBindings(root["history_executions"])
	history, _ := root["history_executions"].([]any)
	root["history_empty"] = len(history) == 0
	return data, nil
}

func decorateProjectDetails(root map[string]any) {
	root["loading"] = false
	root["ready"] = true
	root["load_error"] = ""
	project, _ := root["project"].(map[string]any)
	if project == nil {
		project = map[string]any{}
		root["project"] = project
	}
	defaults := map[string]any{
		"id": float64(0), "name": "Project", "project_icon": "", "project_icon_content_type": "",
		"repo_url": "", "repo_ref": "", "config_file": "", "pipeline_chains": []any{}, "has_pipeline_chains": false,
	}
	for key, value := range defaults {
		if _, exists := project[key]; !exists {
			project[key] = value
		}
	}
	if strings.TrimSpace(fmt.Sprint(project["project_icon"])) == "" {
		if icon, exists := root["project_icon"]; exists && icon != nil {
			project["project_icon"] = icon
		}
		if contentType, exists := root["project_icon_content_type"]; exists && contentType != nil {
			project["project_icon_content_type"] = contentType
		}
	}
	applyProjectStructureFilter(root, "all-pipelines")
}

func applyProjectStructureFilter(root map[string]any, requested string) bool {
	filters, _ := root["structure_filters"].([]any)
	var selected map[string]any
	for _, raw := range filters {
		filter, ok := raw.(map[string]any)
		if ok && strings.TrimSpace(fmt.Sprint(filter["value"])) == requested {
			selected = filter
			break
		}
	}
	if selected == nil && requested != "all-pipelines" {
		return applyProjectStructureFilter(root, "all-pipelines")
	}
	if selected == nil {
		return false
	}
	included := map[string]bool{}
	for _, pipelineID := range stringListValue(selected["pipeline_ids"]) {
		included[pipelineID] = true
	}
	pipelines, _ := root["pipelines"].([]any)
	visible := make([]any, 0, len(pipelines))
	for _, raw := range pipelines {
		pipeline, ok := raw.(map[string]any)
		if ok && included[strings.TrimSpace(fmt.Sprint(pipeline["pipeline_id"]))] {
			visible = append(visible, raw)
		}
	}
	root["structure_filter"] = strings.TrimSpace(fmt.Sprint(selected["value"]))
	root["visible_pipelines"] = visible
	root["show_chain_structure"] = boolValue(selected["show_chain_structure"])
	root["show_pipeline_structure"] = boolValue(selected["show_pipeline_structure"])
	root["structure_root"] = selected["root"]
	return true
}

func stringListValue(value any) []string {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	parts := make([]string, 0, len(items))
	for _, item := range items {
		if text := strings.TrimSpace(fmt.Sprint(item)); text != "" {
			parts = append(parts, text)
		}
	}
	return parts
}

func jobDetailsBindingData(view *cnpv1.JobDetailsView) (map[string]any, error) {
	data, err := protobufBindingData("jobDetails", "job-details", view)
	if err != nil {
		return nil, err
	}
	if root, ok := data["jobDetails"].(map[string]any); ok {
		ensureSchedulingDiagnosisBinding(root)
		for _, key := range []string{"host_tool_requirements", "container_tool_requirements"} {
			if root[key] == nil {
				root[key] = map[string]any{"empty_label": "", "summary": "", "tone": "muted", "issues": []any{}}
			}
		}
		for key, emptyLabel := range map[string]string{
			"artifacts": "No artifacts", "test_report": "No parsed test report", "coverage_report": "No parsed coverage report",
		} {
			if root[key] == nil {
				root[key] = map[string]any{
					"empty_label": emptyLabel, "summary": "", "tone": "muted", "rows": []any{}, "additional_label": "",
					"nodes": []any{}, "filters": []any{}, "filter": "all", "can_download_all": false,
				}
			}
		}
		if root["run_context"] == nil {
			root["run_context"] = map[string]any{
				"available": false, "scope_label": "", "current_execution_id": "loading", "current_pipeline_id": "",
				"current_pipeline_job_id": "", "pipelines": []any{},
			}
		}
		root["output"] = ""
		root["system_output"] = ""
		root["output_search"] = ""
		root["output_search_count"] = "0/0"
		root["tailing_label"] = "Tailing: Off"
		root["tailing_tone"] = "warning"
		if protocol.IsActiveJobExecutionStatus(fmt.Sprint(root["status"])) {
			root["tailing_label"] = "Tailing: On"
			root["tailing_tone"] = "success"
		}
		if groups, ok := root["output_groups"].([]any); ok {
			for _, raw := range groups {
				entry, entryOK := raw.(map[string]any)
				if !entryOK {
					continue
				}
				entry["output"] = ""
				entry["empty_output_label"] = "(no output)"
				if reached, _ := entry["reached"].(bool); !reached {
					entry["empty_output_label"] = "(step was not reached)"
				}
				for _, field := range []string{"details", "yaml_literal", "expanded_command"} {
					if strings.TrimSpace(fmt.Sprint(entry[field])) == "" {
						entry[field] = "(none)"
					}
				}
			}
		}
		if timeline, ok := root["timeline"].([]any); ok && len(timeline) > 0 {
			selected := timeline[0]
			for _, item := range timeline {
				entry, entryOK := item.(map[string]any)
				if !entryOK {
					continue
				}
				status := strings.ToLower(fmt.Sprint(entry["status"]))
				if status == "running" || status == "in progress" || status == "failed" {
					selected = item
					break
				}
			}
			root["selected_timeline_item"] = selected
		} else {
			root["selected_timeline_item"] = map[string]any{
				"id": "", "title": "No execution steps reported", "description": "", "status": "", "status_label": "", "duration": "", "exit_code": "", "error": "",
			}
		}
	}
	return data, nil
}

func settingsBindingData(server *cnpv1.ServerInfo, themes []*uidsl.ThemeDocument, selectedTheme string) (map[string]any, error) {
	serverData, err := protobufBindingData("server", "settings server", server)
	if err != nil {
		return nil, err
	}
	serverBinding, ok := serverData["server"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("settings server binding is malformed")
	}
	themeBindings := make([]any, 0, len(themes))
	selectedDescription := ""
	for _, theme := range themes {
		themeBindings = append(themeBindings, map[string]any{
			"name": theme.Metadata.Name, "title": theme.Metadata.Title, "description": theme.Metadata.Description,
		})
		if theme.Metadata.Name == selectedTheme {
			selectedDescription = theme.Metadata.Description
		}
	}
	return map[string]any{"settings": map[string]any{
		"server": serverBinding, "themes": themeBindings,
		"client_version": "", "server_version": server.GetVersion(), "server_connected": strings.TrimSpace(server.GetVersion()) != "",
		"selected_theme": selectedTheme, "selected_theme_description": selectedDescription, "projects": []any{},
		"connection_mode": connectionModeDiscover, "connection_endpoint": "", "connection_explicit": false,
		"connection_modes": []any{
			map[string]any{"value": connectionModeDiscover, "label": "Automatic discovery"},
			map[string]any{"value": connectionModeExplicit, "label": "Explicit endpoint"},
			map[string]any{"value": connectionModeSSH, "label": "Remote server (SSH)"},
		},
		"ssh": false, "ssh_jump_address": "", "ssh_username": "", "ssh_destination": "",
		"ssh_public_key": "", "ssh_has_key": false, "ssh_authorized_key": "",
		"ssh_host_fingerprint": "", "ssh_pending_fingerprint": "", "ssh_has_pending_fingerprint": false,
		"ssh_has_trusted_fingerprint": false,
		"import_repo_url":             "", "import_repo_ref": "", "import_config_file": "ciwi-project.yaml",
		"update_supported": false, "update_capability_notice": "Update status unavailable", "update_status_label": "", "blocked_agent_notice": "",
		"update_current_version": "", "update_last_apply_status": "",
		"update_versions": versionOptions(nil, "Check for updates"), "selected_update_version": "",
		"rollback_versions": versionOptions(nil, "Refresh versions"), "selected_rollback_version": "",
		"update_result": "", "update_result_tone": "muted", "rollback_result": "", "rollback_result_tone": "muted",
		"loading": false, "ready": true, "load_error": "",
	}}, nil
}

func offlineFrontPageBindingData() (map[string]any, error) {
	data, err := frontPageBindingData(&cnpv1.FrontPageView{
		Server: &cnpv1.ServerInfo{Version: "Unavailable"},
	})
	if err != nil {
		return nil, err
	}
	root := data["frontPage"].(map[string]any)
	root["loading"] = true
	root["queued_empty"] = false
	root["history_empty"] = false
	return data, nil
}

func offlineSettingsBindingData(clientVersion, selectedTheme, mode, endpoint string, sshSettings sshConnectionSettings) (map[string]any, error) {
	themes, err := sharedUI.LoadThemes()
	if err != nil {
		return nil, err
	}
	data, err := settingsBindingData(&cnpv1.ServerInfo{}, themes, selectedTheme)
	if err != nil {
		return nil, err
	}
	settings := data["settings"].(map[string]any)
	settings["client_version"] = strings.TrimSpace(clientVersion)
	settings["server_version"] = "Unavailable"
	settings["server_connected"] = false
	settings["connection_mode"] = mode
	settings["connection_endpoint"] = endpoint
	settings["connection_explicit"] = mode == connectionModeExplicit
	settings["ssh"] = mode == connectionModeSSH
	settings["ssh_jump_address"] = sshSettings.JumpAddress
	settings["ssh_username"] = sshSettings.Username
	settings["ssh_destination"] = sshSettings.Destination
	settings["ssh_public_key"] = sshSettings.PublicKey
	settings["ssh_has_key"] = strings.TrimSpace(sshSettings.PublicKey) != ""
	settings["ssh_authorized_key"] = cnpclient.RestrictedAuthorizedKey(sshSettings.PublicKey, sshSettings.Destination)
	settings["ssh_host_fingerprint"] = sshSettings.HostKeyFingerprint
	settings["ssh_has_trusted_fingerprint"] = strings.TrimSpace(sshSettings.HostKeyFingerprint) != ""
	settings["ssh_pending_fingerprint"] = sshSettings.PendingFingerprint
	settings["ssh_has_pending_fingerprint"] = strings.TrimSpace(sshSettings.PendingFingerprint) != ""
	settings["update_capability_notice"] = "Connect to a server to manage projects and server updates."
	return data, nil
}

func refreshOfflineScreen(renderer nativeRenderer, screens map[string]*uidsl.ScreenDocument, navigation navigationState, clientVersion, themeName, mode, endpoint string, sshSettings sshConnectionSettings) error {
	switch navigation.screen {
	case "front-page":
		data, err := offlineFrontPageBindingData()
		if err != nil {
			return err
		}
		renderer.SetScreenAndData(screens["front-page"], data)
	case "settings":
		data, err := offlineSettingsBindingData(clientVersion, themeName, mode, endpoint, sshSettings)
		if err != nil {
			return err
		}
		renderer.SetScreenAndData(screens["settings"], data)
	default:
		return fmt.Errorf("screen %q needs a server connection", navigation.screen)
	}
	return nil
}

func rememberSuccessfulEndpoint(preferencesPath, address string) {
	address = strings.TrimSpace(address)
	if address == "" {
		return
	}
	_ = updateNativePreferences(preferencesPath, func(preferences *nativePreferences) {
		preferences.LastSuccessfulEndpoint = address
	})
}

func protobufBindingData(root, description string, message proto.Message) (map[string]any, error) {
	payload, err := (protojson.MarshalOptions{UseProtoNames: true, EmitUnpopulated: true}).Marshal(message)
	if err != nil {
		return nil, fmt.Errorf("encode %s binding data: %w", description, err)
	}
	var normalized map[string]any
	if err := json.Unmarshal(payload, &normalized); err != nil {
		return nil, fmt.Errorf("decode %s binding data: %w", description, err)
	}
	normalized["loading"] = false
	normalized["ready"] = true
	normalized["load_error"] = ""
	return map[string]any{root: normalized}, nil
}

func setScreenLifecycle(data map[string]any, screen string, loading, ready bool, loadError string) {
	root, _ := data[screenBindingRoot(screen)].(map[string]any)
	if root == nil {
		return
	}
	root["loading"] = loading
	root["ready"] = ready
	root["load_error"] = loadError
}

func screenBindingRoot(screen string) string {
	return map[string]string{
		"front-page": "frontPage", "project-details": "projectDetails", "job-details": "jobDetails",
		"settings": "settings", "managed-yaml": "managedYAML", "run-options": "runOptions", "agents": "agents",
		"agent-details": "agentDetails", "agent-script": "agentScript", "vault": "vault", "connection": "connection",
	}[screen]
}

func nativeTargets(ctx context.Context, explicit string) ([]string, error) {
	explicit = strings.TrimSpace(explicit)
	if explicit != "" {
		target, err := cnpclient.ParseTarget(explicit)
		if err != nil {
			return nil, err
		}
		return []string{target.String()}, nil
	}
	discoveryCtx, cancel := context.WithTimeout(ctx, 1500*time.Millisecond)
	defer cancel()
	endpoints, err := cnpclient.Discover(discoveryCtx, time.Second)
	if err != nil && len(endpoints) == 0 {
		return nil, fmt.Errorf("native endpoint discovery failed: %w", err)
	}
	if len(endpoints) == 0 {
		return nil, fmt.Errorf("no ciwi native endpoint found; pass -addr [quic|tcp]://host:port")
	}
	targets := make([]string, 0, len(endpoints))
	for _, endpoint := range endpoints {
		targets = append(targets, endpoint.Target().String())
	}
	return targets, nil
}

func relevantScreenChange(screen *uidsl.ScreenDocument, navigation navigationState, change *cnpv1.ChangeEvent) bool {
	if screen == nil || change == nil {
		return false
	}
	if navigation.screen == "job-details" {
		for _, topic := range change.Topics {
			if topic == cnpv1.ChangeTopic_CHANGE_TOPIC_JOB_OUTPUT {
				return false
			}
		}
		for _, topic := range change.Topics {
			if topic == cnpv1.ChangeTopic_CHANGE_TOPIC_QUEUE || topic == cnpv1.ChangeTopic_CHANGE_TOPIC_HISTORY {
				matched := false
				for _, jobID := range change.GetJobExecutionIds() {
					if jobID == navigation.jobID {
						matched = true
						break
					}
				}
				if !matched {
					return false
				}
			}
		}
	}
	viewTopics := make(map[string]bool)
	for _, source := range screen.Screen.DataSources {
		for _, topic := range source.WatchTopics {
			viewTopics[topic] = true
		}
	}
	for _, topic := range change.Topics {
		if viewTopics[nativeChangeTopicName(topic)] {
			return true
		}
	}
	return false
}

func nativeChangeTopicName(topic cnpv1.ChangeTopic) string {
	switch topic {
	case cnpv1.ChangeTopic_CHANGE_TOPIC_SERVER:
		return "server"
	case cnpv1.ChangeTopic_CHANGE_TOPIC_PROJECTS:
		return "projects"
	case cnpv1.ChangeTopic_CHANGE_TOPIC_AGENTS:
		return "agents"
	case cnpv1.ChangeTopic_CHANGE_TOPIC_QUEUE:
		return "queue"
	case cnpv1.ChangeTopic_CHANGE_TOPIC_HISTORY:
		return "history"
	case cnpv1.ChangeTopic_CHANGE_TOPIC_UPDATES:
		return "updates"
	case cnpv1.ChangeTopic_CHANGE_TOPIC_VAULT:
		return "vault"
	case cnpv1.ChangeTopic_CHANGE_TOPIC_AGENT_ELIGIBILITY:
		return "agent-eligibility"
	case cnpv1.ChangeTopic_CHANGE_TOPIC_JOB_OUTPUT:
		return "job-output"
	default:
		return ""
	}
}

func findTheme(name string) (*uidsl.ThemeDocument, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "default"
	}
	themes, err := sharedUI.LoadThemes()
	if err != nil {
		return nil, err
	}
	for _, theme := range themes {
		if theme.Metadata.Name == name {
			return theme, nil
		}
	}
	return nil, fmt.Errorf("unknown theme %q", name)
}
