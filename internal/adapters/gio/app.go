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
	screens := map[string]*uidsl.ScreenDocument{
		"front-page": frontPageScreen, "project-details": projectDetailsScreen, "job-details": jobDetailsScreen,
	}
	theme, err := findTheme(options.Theme)
	if err != nil {
		return err
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
	renderer.SetInvalidate(window.Invalidate)
	renderer.SetStatus("Connecting to ciwi…")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go runController(ctx, window, renderer, commands, screens, options)

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

func runController(ctx context.Context, window *app.Window, renderer *Renderer, commands <-chan commandRequest, screens map[string]*uidsl.ScreenDocument, options Options) {
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
				window.Invalidate()
			}
		case watchErr, ok := <-watchErrors:
			if ok && watchErr != nil {
				renderer.SetStatus("Live updates stopped: " + watchErr.Error())
				window.Invalidate()
			}
			watchErrors = nil
		case command := <-commands:
			handleCommand(ctx, client, renderer, screens, &navigation, command)
			window.Invalidate()
		}
	}
}

func handleCommand(ctx context.Context, client *cnpclient.Client, renderer *Renderer, screens map[string]*uidsl.ScreenDocument, navigation *navigationState, command commandRequest) {
	switch command.action.Command {
	case "run-pipeline":
		pipelineID, err := strconv.ParseInt(command.arguments["pipelineDbId"], 10, 64)
		if err != nil || pipelineID <= 0 {
			renderer.SetStatus("Invalid pipeline identifier")
			return
		}
		renderer.SetStatus("Queuing pipeline…")
		commandCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		result, err := client.RunPipeline(commandCtx, &cnpv1.RunPipelineRequest{PipelineDbId: pipelineID}, "")
		cancel()
		if err != nil {
			renderer.SetStatus("Run failed: " + err.Error())
			return
		}
		renderer.SetStatus(fmt.Sprintf("Queued %d execution(s) for %s", result.Enqueued, result.PipelineId))
	case "navigate":
		if err := navigate(ctx, client, renderer, screens, navigation, command.arguments["route"]); err != nil {
			renderer.SetStatus("Navigation failed: " + err.Error())
		}
	default:
		renderer.SetStatus("Unsupported native action: " + command.action.Command)
	}
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
	default:
		return fmt.Errorf("screen %q is unsupported", navigation.screen)
	}
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
	return protobufBindingData("frontPage", "front-page", view)
}

func projectDetailsBindingData(view *cnpv1.ProjectDetailsView) (map[string]any, error) {
	return protobufBindingData("projectDetails", "project-details", view)
}

func jobDetailsBindingData(view *cnpv1.JobDetailsView) (map[string]any, error) {
	return protobufBindingData("jobDetails", "job-details", view)
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
