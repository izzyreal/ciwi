//go:build darwin

package gio

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"gioui.org/app"
	"gioui.org/op"
	cnpv1 "github.com/izzyreal/ciwi/pkg/cnp/v1"
	"github.com/izzyreal/ciwi/pkg/cnpclient"
	"github.com/izzyreal/ciwi/pkg/uidsl"
	sharedUI "github.com/izzyreal/ciwi/ui"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

type Options struct {
	Address string
	Theme   string
	Version string
}

type commandRequest struct {
	action    uidsl.Action
	arguments map[string]string
}

type navigationState struct {
	screen    string
	projectID int64
	jobID     string
}

type jobOutputBuffer struct {
	jobID string
	text  string
}

const maxNativeOutputBytes = 1024 * 1024

func (b *jobOutputBuffer) reset(jobID string) {
	b.jobID = jobID
	b.text = ""
}

func (b *jobOutputBuffer) append(batch *cnpv1.JobOutputBatch) {
	if batch == nil || (b.jobID != "" && batch.JobExecutionId != b.jobID) {
		return
	}
	for _, line := range batch.Lines {
		b.text += line.Text
	}
	if len(b.text) > maxNativeOutputBytes {
		tail := b.text[len(b.text)-maxNativeOutputBytes:]
		b.text = "[ciwi native: earlier output omitted]\n" + strings.ToValidUTF8(tail, "")
	}
}

func (b *jobOutputBuffer) apply(renderer *Renderer) {
	renderer.SetRootBinding("jobDetails", "output", b.text)
}

func Run(options Options) error {
	frontPageScreen, err := sharedUI.LoadScreen("front-page")
	if err != nil {
		return err
	}
	projectDetailsScreen, err := sharedUI.LoadScreen("project-details")
	if err != nil {
		return err
	}
	jobDetailsScreen, err := sharedUI.LoadScreen("job-details")
	if err != nil {
		return err
	}
	settingsScreen, err := sharedUI.LoadScreen("settings")
	if err != nil {
		return err
	}
	screens := map[string]*uidsl.ScreenDocument{
		"front-page": frontPageScreen, "project-details": projectDetailsScreen, "job-details": jobDetailsScreen,
		"settings": settingsScreen,
	}
	preferencesPath, err := nativePreferencesPath()
	if err != nil {
		return err
	}
	preferences, err := loadNativePreferences(preferencesPath)
	if err != nil {
		return err
	}
	themeName := strings.TrimSpace(options.Theme)
	if themeName == "" {
		themeName = strings.TrimSpace(preferences.Theme)
	}
	theme, err := findTheme(themeName)
	if err != nil {
		if strings.TrimSpace(options.Theme) != "" {
			return err
		}
		theme, err = findTheme("default")
		if err != nil {
			return err
		}
	}
	window := new(app.Window)
	window.Option(app.Title("ciwi native"), app.Size(1180, 780))
	commands := make(chan commandRequest, 16)
	var renderer *Renderer
	renderer, err = NewRenderer(frontPageScreen, theme, func(action uidsl.Action, arguments map[string]string) {
		select {
		case commands <- commandRequest{action: action, arguments: arguments}:
		default:
			renderer.SetStatus("Another command is already being processed")
			window.Invalidate()
		}
	})
	if err != nil {
		return err
	}
	renderer.SetDisclosureStates(preferences.Disclosures)
	renderer.SetDisclosureChange(func(states map[string]bool) {
		if err := updateNativePreferences(preferencesPath, func(preferences *nativePreferences) {
			preferences.Disclosures = states
		}); err != nil {
			renderer.SetStatus("Disclosure state could not be saved: " + err.Error())
		}
	})
	renderer.SetInvalidate(window.Invalidate)
	renderer.SetStatus("Connecting to ciwi…")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go runController(ctx, window, renderer, commands, screens, options, preferencesPath)

	var operations op.Ops
	for {
		switch event := window.Event().(type) {
		case app.DestroyEvent:
			return event.Err
		case app.FrameEvent:
			gtx := app.NewContext(&operations, event)
			renderer.Layout(gtx)
			event.Frame(&operations)
		}
	}
}

func runController(ctx context.Context, window *app.Window, renderer *Renderer, commands <-chan commandRequest, screens map[string]*uidsl.ScreenDocument, options Options, preferencesPath string) {
	address, err := nativeAddress(ctx, options.Address)
	if err != nil {
		renderer.SetStatus(err.Error())
		window.Invalidate()
		return
	}
	dialCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	client, err := cnpclient.Dial(dialCtx, address, "ciwi-desktop", options.Version)
	cancel()
	if err != nil {
		renderer.SetStatus(err.Error())
		window.Invalidate()
		return
	}
	var outputBatches <-chan *cnpv1.JobOutputBatch
	var outputErrors <-chan error
	var outputCancel context.CancelFunc
	outputBuffer := &jobOutputBuffer{}
	stopOutput := func() {
		if outputCancel != nil {
			outputCancel()
		}
		outputCancel = nil
		outputBatches = nil
		outputErrors = nil
	}
	startOutput := func(jobID string) {
		stopOutput()
		outputBuffer.reset(jobID)
		outputBuffer.apply(renderer)
		streamCtx, cancelStream := context.WithCancel(ctx)
		batches, errorsOut, streamErr := client.WatchJobOutput(streamCtx, jobID, 0)
		if streamErr != nil {
			cancelStream()
			renderer.SetStatus("Output stream unavailable: " + streamErr.Error())
			return
		}
		outputCancel = cancelStream
		outputBatches = batches
		outputErrors = errorsOut
	}
	defer stopOutput()
	defer client.Close()
	navigation := navigationState{screen: "front-page"}
	if err := refreshScreen(ctx, client, renderer, screens, navigation); err != nil {
		renderer.SetStatus(err.Error())
	} else {
		renderer.SetStatus("Connected to " + address)
	}
	window.Invalidate()

	watchCtx, watchCancel := context.WithCancel(ctx)
	defer watchCancel()
	changes, watchErrors, err := client.WatchChanges(watchCtx)
	if err != nil {
		renderer.SetStatus("Live updates unavailable: " + err.Error())
		window.Invalidate()
		changes = nil
		watchErrors = nil
	}
	for {
		select {
		case <-ctx.Done():
			return
		case change, ok := <-changes:
			if !ok {
				changes = nil
				continue
			}
			if change.ResyncRequired || relevantScreenChange(navigation, change) {
				if err := refreshScreen(ctx, client, renderer, screens, navigation); err != nil {
					renderer.SetStatus("Refresh failed: " + err.Error())
				}
				if navigation.screen == "job-details" {
					outputBuffer.apply(renderer)
				}
				window.Invalidate()
			}
		case watchErr, ok := <-watchErrors:
			if ok && watchErr != nil {
				renderer.SetStatus("Live updates stopped: " + watchErr.Error())
				window.Invalidate()
			}
			watchErrors = nil
		case batch, ok := <-outputBatches:
			if !ok {
				outputBatches = nil
				continue
			}
			outputBuffer.append(batch)
			outputBuffer.apply(renderer)
			window.Invalidate()
		case outputErr, ok := <-outputErrors:
			if !ok {
				outputErrors = nil
				continue
			}
			if outputErr != nil {
				renderer.SetStatus("Output stream stopped: " + outputErr.Error())
				window.Invalidate()
			}
			outputErrors = nil
		case command := <-commands:
			previous := navigation
			handleCommand(ctx, client, renderer, screens, &navigation, command, preferencesPath)
			if navigation != previous {
				if navigation.screen == "job-details" {
					startOutput(navigation.jobID)
				} else {
					stopOutput()
				}
			}
			window.Invalidate()
		}
	}
}

func handleCommand(ctx context.Context, client *cnpclient.Client, renderer *Renderer, screens map[string]*uidsl.ScreenDocument, navigation *navigationState, command commandRequest, preferencesPath string) {
	switch command.action.Command {
	case "run-pipeline":
		pipelineID, err := strconv.ParseInt(command.arguments["pipelineDbId"], 10, 64)
		if err != nil || pipelineID <= 0 {
			renderer.SetStatus("Invalid pipeline identifier")
			return
		}
		dryRun := command.arguments["dryRun"] == "true"
		if dryRun {
			renderer.SetStatus("Queuing pipeline dry run…")
		} else {
			renderer.SetStatus("Queuing pipeline…")
		}
		commandCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		result, err := client.RunPipeline(commandCtx, &cnpv1.RunPipelineRequest{
			PipelineDbId: pipelineID,
			Selection:    pipelineRunSelection(command.arguments),
		}, "")
		cancel()
		if err != nil {
			renderer.SetStatus("Run failed: " + err.Error())
			return
		}
		if dryRun {
			renderer.SetStatus(fmt.Sprintf("Queued %d dry-run execution(s) for %s", result.Enqueued, result.PipelineId))
		} else {
			renderer.SetStatus(fmt.Sprintf("Queued %d execution(s) for %s", result.Enqueued, result.PipelineId))
		}
	case "run-chain":
		projectID, err := strconv.ParseInt(command.arguments["projectId"], 10, 64)
		chainID := strings.TrimSpace(command.arguments["chainId"])
		if err != nil || projectID <= 0 || chainID == "" {
			renderer.SetStatus("Invalid pipeline chain identifier")
			return
		}
		dryRun := command.arguments["dryRun"] == "true"
		if dryRun {
			renderer.SetStatus("Queuing pipeline chain dry run…")
		} else {
			renderer.SetStatus("Queuing pipeline chain…")
		}
		commandCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		result, err := client.RunPipelineChain(commandCtx, &cnpv1.RunPipelineChainRequest{
			ProjectId: projectID, ChainId: chainID,
			Selection: pipelineRunSelection(command.arguments),
		}, "")
		cancel()
		if err != nil {
			renderer.SetStatus("Run chain failed: " + err.Error())
			return
		}
		if dryRun {
			renderer.SetStatus(fmt.Sprintf("Queued %d dry-run execution(s) for chain %s", result.Enqueued, result.ChainId))
		} else {
			renderer.SetStatus(fmt.Sprintf("Queued %d execution(s) for chain %s", result.Enqueued, result.ChainId))
		}
	case "clear-queue":
		renderer.SetStatus("Clearing queued executions…")
		commandCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		result, err := client.ClearExecutionQueue(commandCtx, "")
		cancel()
		if err != nil {
			renderer.SetStatus("Clear queue failed: " + err.Error())
			return
		}
		if err := refreshScreen(ctx, client, renderer, screens, *navigation); err != nil {
			renderer.SetStatus("Queue cleared, but refresh failed: " + err.Error())
			return
		}
		renderer.SetStatus(fmt.Sprintf("Cleared %d queued execution(s)", result.Cleared))
	case "flush-history", "delete-execution":
		request := &cnpv1.FlushExecutionHistoryRequest{All: command.action.Command == "flush-history"}
		if !request.All {
			request.JobExecutionIds = splitExecutionIDs(command.arguments["jobExecutionIds"])
			if len(request.JobExecutionIds) == 0 {
				renderer.SetStatus("No execution identifiers were supplied")
				return
			}
		}
		renderer.SetStatus("Removing execution history…")
		commandCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		result, err := client.FlushExecutionHistory(commandCtx, request, "")
		cancel()
		if err != nil {
			renderer.SetStatus("Flush history failed: " + err.Error())
			return
		}
		if err := refreshScreen(ctx, client, renderer, screens, *navigation); err != nil {
			renderer.SetStatus("History removed, but refresh failed: " + err.Error())
			return
		}
		renderer.SetStatus(fmt.Sprintf("Removed %d execution(s) from history", result.Flushed))
	case "cancel-execution":
		jobID := strings.TrimSpace(command.arguments["jobExecutionId"])
		if jobID == "" {
			renderer.SetStatus("No execution identifier was supplied")
			return
		}
		renderer.SetStatus("Cancelling execution…")
		commandCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		result, err := client.CancelExecution(commandCtx, jobID, "")
		cancel()
		if err != nil {
			renderer.SetStatus("Cancel failed: " + err.Error())
			return
		}
		if err := refreshScreen(ctx, client, renderer, screens, *navigation); err != nil {
			renderer.SetStatus("Execution cancelled, but refresh failed: " + err.Error())
			return
		}
		renderer.SetStatus("Execution " + result.JobExecutionId + " marked failed")
	case "rerun-execution":
		jobID := strings.TrimSpace(command.arguments["jobExecutionId"])
		if jobID == "" {
			renderer.SetStatus("No execution identifier was supplied")
			return
		}
		renderer.SetStatus("Queueing independent rerun…")
		commandCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		result, err := client.RerunExecution(commandCtx, jobID, "")
		cancel()
		if err != nil {
			renderer.SetStatus("Rerun failed: " + err.Error())
			return
		}
		if err := navigate(ctx, client, renderer, screens, navigation, "/jobs/"+result.JobExecutionId); err != nil {
			renderer.SetStatus("Rerun queued, but navigation failed: " + err.Error())
			return
		}
		renderer.SetStatus("Queued rerun " + result.JobExecutionId)
	case "navigate":
		if err := navigate(ctx, client, renderer, screens, navigation, command.arguments["route"]); err != nil {
			renderer.SetStatus("Navigation failed: " + err.Error())
		}
	case "change-theme":
		theme, err := findTheme(command.arguments["theme"])
		if err != nil {
			renderer.SetStatus("Theme change failed: " + err.Error())
			return
		}
		if err := renderer.SetTheme(theme); err != nil {
			renderer.SetStatus("Theme change failed: " + err.Error())
			return
		}
		renderer.SetRootBinding("settings", "selected_theme", theme.Metadata.Name)
		renderer.SetRootBinding("settings", "selected_theme_description", theme.Metadata.Description)
		if err := updateNativePreferences(preferencesPath, func(preferences *nativePreferences) {
			preferences.Theme = theme.Metadata.Name
		}); err != nil {
			renderer.SetStatus("Theme changed, but the preference could not be saved: " + err.Error())
			return
		}
		renderer.SetStatus("Theme: " + theme.Metadata.Title)
	default:
		renderer.SetStatus("Unsupported native action: " + command.action.Command)
	}
}

func pipelineRunSelection(arguments map[string]string) *cnpv1.RunPipelineSelection {
	return &cnpv1.RunPipelineSelection{
		PipelineJobId: strings.TrimSpace(arguments["pipelineJobId"]),
		DryRun:        arguments["dryRun"] == "true",
		SourceRef:     strings.TrimSpace(arguments["sourceRef"]),
		AgentId:       strings.TrimSpace(arguments["agentId"]),
		ExecutionMode: strings.TrimSpace(arguments["executionMode"]),
	}
}

func splitExecutionIDs(raw string) []string {
	parts := strings.Split(raw, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if id := strings.TrimSpace(part); id != "" {
			result = append(result, id)
		}
	}
	return result
}

func navigate(ctx context.Context, client *cnpclient.Client, renderer *Renderer, screens map[string]*uidsl.ScreenDocument, navigation *navigationState, route string) error {
	route = strings.TrimSpace(route)
	next := navigationState{}
	switch {
	case route == "/":
		next.screen = "front-page"
	case strings.HasPrefix(route, "/projects/"):
		projectID, err := strconv.ParseInt(strings.Trim(strings.TrimPrefix(route, "/projects/"), "/"), 10, 64)
		if err != nil || projectID <= 0 {
			return fmt.Errorf("invalid project route %q", route)
		}
		next = navigationState{screen: "project-details", projectID: projectID}
	case strings.HasPrefix(route, "/jobs/"):
		jobID := strings.Trim(strings.TrimPrefix(route, "/jobs/"), "/")
		if jobID == "" || strings.Contains(jobID, "/") {
			return fmt.Errorf("invalid job route %q", route)
		}
		next = navigationState{screen: "job-details", jobID: jobID}
	case route == "/settings":
		next.screen = "settings"
	default:
		return fmt.Errorf("unsupported route %q", route)
	}
	if err := refreshScreen(ctx, client, renderer, screens, next); err != nil {
		return err
	}
	*navigation = next
	if next.screen == "front-page" {
		renderer.SetStatus("Projects")
	} else if next.screen == "project-details" {
		renderer.SetStatus("Project details")
	} else if next.screen == "settings" {
		renderer.SetStatus("Global settings")
	} else {
		renderer.SetStatus("Job details")
	}
	return nil
}

func refreshScreen(ctx context.Context, client *cnpclient.Client, renderer *Renderer, screens map[string]*uidsl.ScreenDocument, navigation navigationState) error {
	screen := screens[navigation.screen]
	if screen == nil {
		return fmt.Errorf("screen %q is unavailable", navigation.screen)
	}
	switch navigation.screen {
	case "front-page":
		return refreshFrontPage(ctx, client, renderer, screen)
	case "project-details":
		return refreshProjectDetails(ctx, client, renderer, screen, navigation.projectID)
	case "job-details":
		return refreshJobDetails(ctx, client, renderer, screen, navigation.jobID)
	case "settings":
		return refreshSettings(ctx, client, renderer, screen)
	default:
		return fmt.Errorf("screen %q is unsupported", navigation.screen)
	}
}

func refreshSettings(ctx context.Context, client *cnpclient.Client, renderer *Renderer, screen *uidsl.ScreenDocument) error {
	requestCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	server, err := client.GetServerInfo(requestCtx)
	if err != nil {
		return err
	}
	themes, err := sharedUI.LoadThemes()
	if err != nil {
		return err
	}
	data, err := settingsBindingData(server, themes, renderer.ThemeName())
	if err != nil {
		return err
	}
	renderer.SetScreenAndData(screen, data)
	return nil
}

func refreshJobDetails(ctx context.Context, client *cnpclient.Client, renderer *Renderer, screen *uidsl.ScreenDocument, jobID string) error {
	requestCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	view, err := client.GetJobDetails(requestCtx, jobID)
	if err != nil {
		return err
	}
	data, err := jobDetailsBindingData(view)
	if err != nil {
		return err
	}
	renderer.SetScreenAndData(screen, data)
	return nil
}

func refreshFrontPage(ctx context.Context, client *cnpclient.Client, renderer *Renderer, screen *uidsl.ScreenDocument) error {
	requestCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	view, err := client.GetFrontPageView(requestCtx)
	if err != nil {
		return err
	}
	data, err := frontPageBindingData(view)
	if err != nil {
		return err
	}
	renderer.SetScreenAndData(screen, data)
	return nil
}

func refreshProjectDetails(ctx context.Context, client *cnpclient.Client, renderer *Renderer, screen *uidsl.ScreenDocument, projectID int64) error {
	requestCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	view, err := client.GetProjectDetails(requestCtx, projectID)
	if err != nil {
		return err
	}
	data, err := projectDetailsBindingData(view)
	if err != nil {
		return err
	}
	renderer.SetScreenAndData(screen, data)
	return nil
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
	decorateExecutionCards(root["queued_executions"], true)
	decorateExecutionCards(root["history_executions"], false)
	return data, nil
}

func decorateExecutionCards(value any, queued bool) {
	cards, ok := value.([]any)
	if !ok {
		return
	}
	for _, card := range cards {
		entry, entryOK := card.(map[string]any)
		summary, summaryOK := entry["summary"].(map[string]any)
		if !entryOK || !summaryOK {
			continue
		}
		status := "succeeded"
		if numberValue(summary["failed"]) > 0 {
			status = "failed"
		} else if queued && numberValue(summary["in_progress"]) > 0 {
			status = "running"
		} else if queued {
			status = "waiting"
		}
		entry["status"] = status
		if ids, ok := entry["job_execution_ids"].([]any); ok {
			parts := make([]string, 0, len(ids))
			for _, id := range ids {
				parts = append(parts, fmt.Sprint(id))
			}
			entry["job_execution_ids_csv"] = strings.Join(parts, ",")
		}
	}
}

func numberValue(value any) float64 {
	number, _ := value.(float64)
	return number
}

func projectDetailsBindingData(view *cnpv1.ProjectDetailsView) (map[string]any, error) {
	return protobufBindingData("projectDetails", "project-details", view)
}

func jobDetailsBindingData(view *cnpv1.JobDetailsView) (map[string]any, error) {
	data, err := protobufBindingData("jobDetails", "job-details", view)
	if err != nil {
		return nil, err
	}
	if root, ok := data["jobDetails"].(map[string]any); ok {
		root["output"] = ""
		root["output_search"] = ""
		root["output_search_count"] = "0/0"
		root["tailing_label"] = "Tailing: Off"
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
		"selected_theme": selectedTheme, "selected_theme_description": selectedDescription,
	}}, nil
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
	return map[string]any{root: normalized}, nil
}

func nativeAddress(ctx context.Context, explicit string) (string, error) {
	explicit = strings.TrimSpace(explicit)
	if explicit != "" {
		if strings.HasPrefix(explicit, ":") {
			return "127.0.0.1" + explicit, nil
		}
		return explicit, nil
	}
	discoveryCtx, cancel := context.WithTimeout(ctx, 1500*time.Millisecond)
	defer cancel()
	endpoints, err := cnpclient.Discover(discoveryCtx, time.Second)
	if err != nil && len(endpoints) == 0 {
		return "", fmt.Errorf("native endpoint discovery failed: %w", err)
	}
	if len(endpoints) == 0 {
		return "", fmt.Errorf("no ciwi native endpoint found; pass -addr host:port")
	}
	return endpoints[0].Address, nil
}

func relevantScreenChange(navigation navigationState, change *cnpv1.ChangeEvent) bool {
	for _, topic := range change.Topics {
		if navigation.screen == "project-details" && topic == cnpv1.ChangeTopic_CHANGE_TOPIC_PROJECTS {
			return true
		}
		if navigation.screen == "job-details" && (topic == cnpv1.ChangeTopic_CHANGE_TOPIC_QUEUE || topic == cnpv1.ChangeTopic_CHANGE_TOPIC_HISTORY) {
			return true
		}
		if navigation.screen == "settings" && topic == cnpv1.ChangeTopic_CHANGE_TOPIC_SERVER {
			return true
		}
		if navigation.screen == "front-page" {
			switch topic {
			case cnpv1.ChangeTopic_CHANGE_TOPIC_SERVER, cnpv1.ChangeTopic_CHANGE_TOPIC_PROJECTS,
				cnpv1.ChangeTopic_CHANGE_TOPIC_QUEUE, cnpv1.ChangeTopic_CHANGE_TOPIC_HISTORY:
				return true
			}
		}
	}
	return false
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
