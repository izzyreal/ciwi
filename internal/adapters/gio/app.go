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

func Run(options Options) error {
	screen, err := sharedUI.LoadScreen("front-page")
	if err != nil {
		return err
	}
	theme, err := findTheme(options.Theme)
	if err != nil {
		return err
	}
	window := new(app.Window)
	window.Option(app.Title("ciwi native"), app.Size(1180, 780))
	commands := make(chan commandRequest, 16)
	var renderer *Renderer
	renderer, err = NewRenderer(screen, theme, func(action uidsl.Action, arguments map[string]string) {
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
	go runController(ctx, window, renderer, commands, options)

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

func runController(ctx context.Context, window *app.Window, renderer *Renderer, commands <-chan commandRequest, options Options) {
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
	if err := refreshFrontPage(ctx, client, renderer); err != nil {
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
			if change.ResyncRequired || relevantFrontPageChange(change) {
				if err := refreshFrontPage(ctx, client, renderer); err != nil {
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
			handleCommand(ctx, client, renderer, command)
			window.Invalidate()
		}
	}
}

func handleCommand(ctx context.Context, client *cnpclient.Client, renderer *Renderer, command commandRequest) {
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
		renderer.SetStatus("Navigation is the next native-client slice (target " + command.arguments["route"] + ")")
	default:
		renderer.SetStatus("Unsupported native action: " + command.action.Command)
	}
}

func refreshFrontPage(ctx context.Context, client *cnpclient.Client, renderer *Renderer) error {
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
	renderer.SetData(data)
	return nil
}

func frontPageBindingData(view *cnpv1.FrontPageView) (map[string]any, error) {
	payload, err := (protojson.MarshalOptions{UseProtoNames: true, EmitUnpopulated: true}).Marshal(view)
	if err != nil {
		return nil, fmt.Errorf("encode front-page binding data: %w", err)
	}
	var normalized map[string]any
	if err := json.Unmarshal(payload, &normalized); err != nil {
		return nil, fmt.Errorf("decode front-page binding data: %w", err)
	}
	return map[string]any{"frontPage": normalized}, nil
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

func relevantFrontPageChange(change *cnpv1.ChangeEvent) bool {
	for _, topic := range change.Topics {
		switch topic {
		case cnpv1.ChangeTopic_CHANGE_TOPIC_SERVER, cnpv1.ChangeTopic_CHANGE_TOPIC_PROJECTS,
			cnpv1.ChangeTopic_CHANGE_TOPIC_QUEUE, cnpv1.ChangeTopic_CHANGE_TOPIC_HISTORY:
			return true
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
