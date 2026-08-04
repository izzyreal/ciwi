//go:build darwin || ios || linux || windows

package gio

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"gioui.org/app"
	"gioui.org/op"
	"github.com/izzyreal/ciwi/internal/protocol"
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
	Route   string
}

type commandRequest struct {
	action    uidsl.Action
	arguments map[string]string
}

type navigationState struct {
	screen         string
	projectID      int64
	pipelineDBID   int64
	chainID        string
	jobID          string
	sourceRef      string
	agentID        string
	agentDetailsID string
}

type runOptionsLoadResult struct {
	navigation navigationState
	generation uint64
	data       map[string]any
	err        error
}

type jobOutputBuffer struct {
	jobID   string
	events  []*cnpv1.JobOutputEvent
	omitted map[string]bool
}

const (
	maxNativeOutputBytes = 1024 * 1024
	nativeNoticeDuration = 6 * time.Second
	nativeReconnectMax   = 8 * time.Second
)

type nativeSession struct {
	client      *cnpclient.Client
	address     string
	changes     <-chan *cnpv1.ChangeEvent
	watchErrors <-chan error
	cancelWatch context.CancelFunc
}

type nativeConnectionState struct {
	connected  bool
	connecting bool
	address    string
	status     string
}

func connectNativeSession(ctx context.Context, address, version string) (*nativeSession, error) {
	targets, err := nativeTargets(ctx, address)
	if err != nil {
		return nil, err
	}
	connectCtx, cancelConnect := context.WithTimeout(ctx, 8*time.Second)
	defer cancelConnect()
	client, target, err := dialNativeTargets(connectCtx, targets, version)
	if err != nil {
		return nil, fmt.Errorf("connect to ciwi native endpoint: %w", err)
	}
	watchCtx, cancelWatch := context.WithCancel(ctx)
	changes, watchErrors, watchErr := client.WatchChanges(watchCtx)
	if watchErr != nil {
		cancelWatch()
		_ = client.Close()
		return nil, fmt.Errorf("connect to ciwi native endpoint: %s: start live updates: %w", target, watchErr)
	}
	return &nativeSession{client: client, address: target, changes: changes, watchErrors: watchErrors, cancelWatch: cancelWatch}, nil
}

func connectConfiguredNativeSession(ctx context.Context, settings nativeConnectionSettings, version string) (*nativeSession, error) {
	preferred := strings.TrimSpace(settings.PreferredAddress)
	if preferred != "" {
		connected, err := connectNativeSession(ctx, preferred, version)
		if err == nil || !settings.DiscoverFallback {
			return connected, err
		}
	}
	return connectNativeSession(ctx, "", version)
}

func (s nativeConnectionState) binding() map[string]any {
	tone := "danger"
	if s.connected {
		tone = "success"
	} else if s.connecting {
		tone = "warning"
	}
	return map[string]any{
		"connected": s.connected, "connecting": s.connecting,
		"offline": !s.connected && !s.connecting, "address": s.address,
		"status": s.status, "tone": tone,
	}
}

func applyNativeConnectionState(renderer *Renderer, state nativeConnectionState) {
	renderer.SetDataBinding("client", state.binding())
}

type nativeDialResult struct {
	index  int
	target string
	client *cnpclient.Client
	err    error
}

func dialNativeTargets(ctx context.Context, targets []string, version string) (*cnpclient.Client, string, error) {
	return dialNativeTargetsWith(ctx, targets, version, cnpclient.Dial)
}

func dialNativeTargetsWith(
	ctx context.Context,
	targets []string,
	version string,
	dial func(context.Context, string, string, string) (*cnpclient.Client, error),
) (*cnpclient.Client, string, error) {
	if len(targets) == 0 {
		return nil, "", fmt.Errorf("no native endpoint targets")
	}
	raceCtx, cancelRace := context.WithCancel(ctx)
	defer cancelRace()
	results := make(chan nativeDialResult, len(targets))
	for index, target := range targets {
		go func() {
			dialCtx, cancelDial := context.WithTimeout(raceCtx, 3*time.Second)
			defer cancelDial()
			client, err := dial(dialCtx, target, "ciwi-desktop", version)
			results <- nativeDialResult{index: index, target: target, client: client, err: err}
		}()
	}
	errorsByTarget := make([]error, len(targets))
	var winner nativeDialResult
	for range targets {
		result := <-results
		if result.err != nil {
			errorsByTarget[result.index] = fmt.Errorf("%s: %w", result.target, result.err)
			continue
		}
		if winner.client == nil {
			winner = result
			cancelRace()
			continue
		}
		_ = result.client.Close()
	}
	if winner.client != nil {
		return winner.client, winner.target, nil
	}
	joined := make([]error, 0, len(errorsByTarget))
	for _, err := range errorsByTarget {
		if err != nil {
			joined = append(joined, err)
		}
	}
	return nil, "", errors.Join(joined...)
}

func (s *nativeSession) close() {
	if s == nil {
		return
	}
	if s.cancelWatch != nil {
		s.cancelWatch()
	}
	if s.client != nil {
		_ = s.client.Close()
	}
}

func nextReconnectDelay(current time.Duration) time.Duration {
	if current <= 0 {
		return time.Second
	}
	next := current * 2
	if next > nativeReconnectMax {
		return nativeReconnectMax
	}
	return next
}

func (b *jobOutputBuffer) reset(jobID string) {
	b.jobID = jobID
	b.events = nil
	b.omitted = map[string]bool{}
}

func (b *jobOutputBuffer) append(batch *cnpv1.JobOutputBatch) {
	if batch == nil || (b.jobID != "" && batch.JobExecutionId != b.jobID) {
		return
	}
	for _, event := range batch.Events {
		if event == nil {
			continue
		}
		eventCopy := proto.Clone(event).(*cnpv1.JobOutputEvent)
		if len(eventCopy.Text) > maxNativeOutputBytes {
			eventCopy.Text = strings.ToValidUTF8(eventCopy.Text[len(eventCopy.Text)-maxNativeOutputBytes:], "")
			b.omitted[eventCopy.ItemId] = true
		}
		b.events = append(b.events, eventCopy)
	}
	for bufferedOutputBytes(b.events) > maxNativeOutputBytes && len(b.events) > 1 {
		removed := b.events[0]
		b.events = b.events[1:]
		if removed.Text != "" {
			b.omitted[removed.ItemId] = true
		}
	}
}

func (b *jobOutputBuffer) apply(renderer *Renderer) {
	snapshot := jobOutputSnapshot{Outputs: map[string]string{}, Errors: map[string]string{}, ExitCodes: map[string]string{}}
	for _, event := range b.events {
		switch event.Type {
		case "system-message":
			snapshot.System += event.Text
		case "output":
			snapshot.Outputs[event.ItemId] += event.Text
		case "finished":
			if event.Error != "" {
				snapshot.Errors[event.ItemId] = event.Error
			}
			if event.ExitCode != "" {
				snapshot.ExitCodes[event.ItemId] = event.ExitCode
			}
		}
	}
	const omitted = "[ciwi native: earlier output omitted]\n"
	for itemID := range b.omitted {
		if itemID == "" {
			snapshot.System = omitted + snapshot.System
		} else {
			snapshot.Outputs[itemID] = omitted + snapshot.Outputs[itemID]
		}
	}
	renderer.ApplyJobOutput(snapshot)
}

func bufferedOutputBytes(events []*cnpv1.JobOutputEvent) int {
	total := 0
	for _, event := range events {
		if event != nil {
			total += len(event.Text)
		}
	}
	return total
}

func Run(options Options) error {
	if strings.TrimSpace(options.Route) != "" {
		if _, err := navigationForRoute(options.Route); err != nil {
			return fmt.Errorf("initial native route: %w", err)
		}
	}
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
	runOptionsScreen, err := sharedUI.LoadScreen("run-options")
	if err != nil {
		return err
	}
	agentsScreen, err := sharedUI.LoadScreen("agents")
	if err != nil {
		return err
	}
	agentDetailsScreen, err := sharedUI.LoadScreen("agent-details")
	if err != nil {
		return err
	}
	screens := map[string]*uidsl.ScreenDocument{
		"front-page": frontPageScreen, "project-details": projectDetailsScreen, "job-details": jobDetailsScreen,
		"settings": settingsScreen, "run-options": runOptionsScreen, "agents": agentsScreen, "agent-details": agentDetailsScreen,
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
	initialData, err := offlineFrontPageBindingData(options.Version)
	if err != nil {
		return err
	}
	renderer.SetScreenAndData(frontPageScreen, initialData)
	applyNativeConnectionState(renderer, nativeConnectionState{connecting: true, status: "Trying to connect…"})
	renderer.SetDisclosureStates(preferences.Disclosures)
	renderer.SetDisclosureChange(func(states map[string]bool) {
		if err := updateNativePreferences(preferencesPath, func(preferences *nativePreferences) {
			preferences.Disclosures = states
		}); err != nil {
			renderer.SetStatus("Disclosure state could not be saved: " + err.Error())
		}
	})
	renderer.SetViewStates(preferences.Views)
	renderer.SetViewChange(func(states map[string]string) {
		if err := updateNativePreferences(preferencesPath, func(preferences *nativePreferences) {
			if preferences.Views == nil {
				preferences.Views = map[string]string{}
			}
			for key, mode := range states {
				preferences.Views[key] = mode
			}
		}); err != nil {
			renderer.SetStatus("View preference could not be saved: " + err.Error())
		}
	})
	renderer.SetInvalidate(window.Invalidate)
	renderer.SetStatus("")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go runController(ctx, window, renderer, commands, screens, options, preferencesPath, preferences)

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

func runController(ctx context.Context, window *app.Window, renderer *Renderer, commands <-chan commandRequest, screens map[string]*uidsl.ScreenDocument, options Options, preferencesPath string, preferences nativePreferences) {
	connectionSettings := nativeConnectionSettingsForLaunch(preferences, options.Address)
	mode, endpoint := connectionSettings.Mode, connectionSettings.Endpoint
	var session *nativeSession
	var client *cnpclient.Client
	var changes <-chan *cnpv1.ChangeEvent
	var watchErrors <-chan error
	address := ""
	reconnectDelay := time.Second
	connected, initialConnectErr := connectConfiguredNativeSession(ctx, connectionSettings, options.Version)
	if initialConnectErr == nil {
		session = connected
		client = connected.client
		changes = connected.changes
		watchErrors = connected.watchErrors
		address = connected.address
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
		if client == nil {
			return
		}
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
	defer func() {
		if session != nil {
			session.close()
		}
	}()
	runOptionsLoads := make(chan runOptionsLoadResult, 8)
	var runOptionsCancel context.CancelFunc
	var runOptionsGeneration uint64
	startRunOptionsLoad := func(target navigationState) {
		if runOptionsCancel != nil {
			runOptionsCancel()
		}
		loadCtx, cancelLoad := context.WithCancel(ctx)
		runOptionsCancel = cancelLoad
		runOptionsGeneration++
		generation := runOptionsGeneration
		activeClient := client
		go func() {
			if activeClient == nil {
				return
			}
			data, loadErr := loadRunOptions(loadCtx, activeClient, target)
			select {
			case runOptionsLoads <- runOptionsLoadResult{navigation: target, generation: generation, data: data, err: loadErr}:
			case <-ctx.Done():
			}
		}()
	}
	defer func() {
		if runOptionsCancel != nil {
			runOptionsCancel()
		}
	}()
	navigation := navigationState{screen: "front-page"}
	if strings.TrimSpace(options.Route) != "" {
		navigation, _ = navigationForRoute(options.Route)
	}
	if initialConnectErr == nil {
		if refreshErr := refreshScreen(ctx, client, renderer, screens, navigation); refreshErr != nil {
			// A remembered deep route can disappear between sessions. Keep a
			// healthy connection and recover to the main screen in that case.
			navigation = navigationState{screen: "front-page"}
			if fallbackErr := refreshScreen(ctx, client, renderer, screens, navigation); fallbackErr != nil {
				initialConnectErr = fmt.Errorf("resynchronize: %w", fallbackErr)
				if session != nil {
					session.close()
					session = nil
				}
				client = nil
				changes = nil
				watchErrors = nil
			}
		}
	}
	if initialConnectErr != nil {
		if navigation.screen != "front-page" && navigation.screen != "settings" {
			navigation = navigationState{screen: "front-page"}
		}
		if err := refreshOfflineScreen(renderer, screens, navigation, options.Version, mode, endpoint); err != nil {
			renderer.SetStatus(err.Error())
		}
		applyNativeConnectionState(renderer, nativeConnectionState{connecting: true, status: "Server unavailable; reconnecting…"})
	} else {
		rememberSuccessfulEndpoint(preferencesPath, address)
		preferences.LastSuccessfulEndpoint = address
		if mode == connectionModeDiscover {
			connectionSettings.PreferredAddress = address
			connectionSettings.DiscoverFallback = true
		}
		applyNativeConnectionState(renderer, nativeConnectionState{connected: true, address: address, status: "Connected to " + address})
		if navigation.screen == "settings" {
			applyConnectionBindings(renderer, "settings", mode, endpoint)
			renderer.SetRootBinding("settings", "client_version", options.Version)
		}
		renderer.SetTransientStatus("Connected to "+address, nativeNoticeDuration)
	}
	window.Invalidate()
	var statusTimer *time.Timer
	var statusExpiry <-chan time.Time
	scheduleStatusExpiry := func() {
		expires := renderer.StatusExpiry()
		if expires.IsZero() {
			if statusTimer != nil {
				statusTimer.Stop()
			}
			statusExpiry = nil
			return
		}
		delay := time.Until(expires)
		if delay < 0 {
			delay = 0
		}
		if statusTimer == nil {
			statusTimer = time.NewTimer(delay)
		} else {
			if !statusTimer.Stop() {
				select {
				case <-statusTimer.C:
				default:
				}
			}
			statusTimer.Reset(delay)
		}
		statusExpiry = statusTimer.C
	}
	scheduleStatusExpiry()
	defer func() {
		if statusTimer != nil {
			statusTimer.Stop()
		}
	}()

	var reconnectTimer *time.Timer
	var reconnect <-chan time.Time
	scheduleReconnectAfter := func(status string, delay time.Duration) {
		if session != nil {
			session.close()
			session = nil
		}
		client = nil
		changes = nil
		watchErrors = nil
		stopOutput()
		if runOptionsCancel != nil {
			runOptionsCancel()
			runOptionsCancel = nil
			runOptionsGeneration++
		}
		applyNativeConnectionState(renderer, nativeConnectionState{connecting: true, status: status})
		renderer.SetStatus("")
		if reconnectTimer == nil {
			reconnectTimer = time.NewTimer(delay)
		} else {
			if !reconnectTimer.Stop() {
				select {
				case <-reconnectTimer.C:
				default:
				}
			}
			reconnectTimer.Reset(delay)
		}
		reconnect = reconnectTimer.C
		window.Invalidate()
	}
	scheduleReconnect := func(reason string) {
		status := "Connection lost; reconnecting…"
		if reason != "" {
			status = "Connection lost: " + reason
		}
		scheduleReconnectAfter(status, reconnectDelay)
	}
	if initialConnectErr != nil {
		scheduleReconnectAfter("Server unavailable: "+initialConnectErr.Error(), reconnectDelay)
	}
	defer func() {
		if reconnectTimer != nil {
			reconnectTimer.Stop()
		}
	}()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-statusExpiry:
			if renderer.ClearExpiredStatus(now) {
				window.Invalidate()
			}
			statusExpiry = nil
		case <-reconnect:
			reconnect = nil
			connected, connectErr := connectConfiguredNativeSession(ctx, connectionSettings, options.Version)
			if connectErr != nil {
				reconnectDelay = nextReconnectDelay(reconnectDelay)
				scheduleReconnect(connectErr.Error())
				continue
			}
			session = connected
			client = connected.client
			changes = connected.changes
			watchErrors = connected.watchErrors
			address = connected.address
			reconnectDelay = time.Second
			if refreshErr := refreshScreen(ctx, client, renderer, screens, navigation); refreshErr != nil {
				// A stale deep link should not make a healthy native connection look
				// broken. Fall back to the main screen and keep the session.
				navigation = navigationState{screen: "front-page"}
				if fallbackErr := refreshScreen(ctx, client, renderer, screens, navigation); fallbackErr != nil {
					scheduleReconnect("resynchronize: " + fallbackErr.Error())
					continue
				}
			}
			rememberSuccessfulEndpoint(preferencesPath, address)
			preferences.LastSuccessfulEndpoint = address
			if mode == connectionModeDiscover {
				connectionSettings.PreferredAddress = address
				connectionSettings.DiscoverFallback = true
			}
			applyNativeConnectionState(renderer, nativeConnectionState{connected: true, address: address, status: "Connected to " + address})
			if navigation.screen == "settings" {
				applyConnectionBindings(renderer, "settings", mode, endpoint)
				renderer.SetRootBinding("settings", "client_version", options.Version)
			}
			if navigation.screen == "job-details" {
				startOutput(navigation.jobID)
			}
			renderer.SetTransientStatus("Reconnected to "+address, nativeNoticeDuration)
			scheduleStatusExpiry()
			window.Invalidate()
		case change, ok := <-changes:
			if !ok {
				scheduleReconnect("")
				continue
			}
			if change.ResyncRequired || relevantScreenChange(navigation, change) {
				if navigation.screen == "run-options" {
					renderer.SetStatus("Refreshing run options…")
					startRunOptionsLoad(navigation)
					window.Invalidate()
					continue
				}
				if err := refreshScreen(ctx, client, renderer, screens, navigation); err != nil {
					renderer.SetStatus("Refresh failed: " + err.Error())
				}
				if navigation.screen == "settings" {
					applyConnectionBindings(renderer, "settings", mode, endpoint)
					renderer.SetRootBinding("settings", "client_version", options.Version)
				}
				if navigation.screen == "job-details" {
					outputBuffer.apply(renderer)
				}
				window.Invalidate()
			}
		case watchErr, ok := <-watchErrors:
			if !ok {
				scheduleReconnect("")
				continue
			}
			if watchErr != nil {
				scheduleReconnect(watchErr.Error())
			}
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
		case result := <-runOptionsLoads:
			if navigation != result.navigation || result.generation != runOptionsGeneration {
				continue
			}
			if result.err != nil {
				renderer.SetStatus("Run options failed: " + result.err.Error())
			} else {
				renderer.SetScreenAndData(screens["run-options"], result.data)
				renderer.SetStatus("")
			}
			window.Invalidate()
		case command := <-commands:
			switch command.action.Command {
			case "set-connection-field":
				switch strings.TrimSpace(command.arguments["field"]) {
				case "mode":
					candidate := strings.TrimSpace(command.arguments["value"])
					if candidate == connectionModeDiscover || candidate == connectionModeExplicit {
						mode = candidate
					}
				case "endpoint":
					endpoint = command.arguments["value"]
				}
				applyConnectionBindings(renderer, navigation.screen, mode, endpoint)
				window.Invalidate()
				continue
			case "save-connection":
				mode = strings.TrimSpace(command.arguments["mode"])
				endpoint = strings.TrimSpace(command.arguments["endpoint"])
				if mode != connectionModeDiscover && mode != connectionModeExplicit {
					renderer.SetStatus("Select a connection mode")
					if navigation.screen == "connection" {
						renderer.SetRootBinding("connection", "status", "Select a connection mode")
						renderer.SetRootBinding("connection", "status_tone", "danger")
					}
					window.Invalidate()
					continue
				}
				connectionSettings = nativeConnectionSettings{Mode: mode, Endpoint: endpoint}
				if mode == connectionModeExplicit {
					target, parseErr := cnpclient.ParseTarget(endpoint)
					if parseErr != nil {
						status := "Invalid native endpoint: " + parseErr.Error()
						renderer.SetStatus(status)
						if navigation.screen == "connection" {
							renderer.SetRootBinding("connection", "status", status)
							renderer.SetRootBinding("connection", "status_tone", "danger")
						}
						window.Invalidate()
						continue
					}
					endpoint = target.String()
					connectionSettings.Endpoint = endpoint
					connectionSettings.PreferredAddress = endpoint
				} else {
					connectionSettings.PreferredAddress = strings.TrimSpace(preferences.LastSuccessfulEndpoint)
					connectionSettings.DiscoverFallback = true
				}
				if saveErr := updateNativePreferences(preferencesPath, func(preferences *nativePreferences) {
					preferences.ConnectionMode = mode
					preferences.ServerEndpoint = endpoint
				}); saveErr != nil {
					renderer.SetStatus("Connection preference could not be saved: " + saveErr.Error())
					window.Invalidate()
					continue
				}
				preferences.ConnectionMode = mode
				preferences.ServerEndpoint = endpoint
				reconnectDelay = time.Second
				scheduleReconnectAfter("Connection settings saved; reconnecting…", 0)
				continue
			case "retry-connection":
				reconnectDelay = time.Second
				scheduleReconnectAfter("Retrying connection…", 0)
				continue
			}
			if command.action.Command == "open-url" {
				if openErr := openExternalURL(command.arguments["url"]); openErr != nil {
					renderer.SetStatus("Could not open link: " + openErr.Error())
				}
				window.Invalidate()
				continue
			}
			if command.action.Command == "navigate" {
				next, parseErr := navigationForRoute(command.arguments["route"])
				if parseErr != nil {
					renderer.SetStatus("Navigation failed: " + parseErr.Error())
					window.Invalidate()
					continue
				}
				if next.screen == "connection" {
					next = navigationState{screen: "settings"}
				}
				if client == nil {
					if next.screen != "front-page" && next.screen != "settings" {
						renderer.SetStatus("This screen needs a server connection")
						window.Invalidate()
						continue
					}
					navigation = next
					stopOutput()
					if err := refreshOfflineScreen(renderer, screens, navigation, options.Version, mode, endpoint); err != nil {
						renderer.SetStatus(err.Error())
					}
					applyNativeConnectionState(renderer, nativeConnectionState{connecting: reconnect != nil, status: "Server unavailable; reconnecting…"})
					window.Invalidate()
					continue
				}
				if next.screen == "run-options" {
					navigation = next
					stopOutput()
					renderer.SetScreenAndData(screens["run-options"], runOptionsLoadingData(next))
					renderer.SetStatus("Loading run options…")
					startRunOptionsLoad(next)
					window.Invalidate()
					continue
				}
				if navigation.screen == "run-options" && runOptionsCancel != nil {
					runOptionsCancel()
					runOptionsCancel = nil
					runOptionsGeneration++
				}
			}
			if client == nil && command.action.Command != "change-theme" && command.action.Command != "set-project-import-field" {
				renderer.SetStatus("Server is offline; reconnecting…")
				window.Invalidate()
				continue
			}
			if command.action.Command == "set-run-option" && navigation.screen == "run-options" {
				field := strings.TrimSpace(command.arguments["field"])
				value := strings.TrimSpace(command.arguments["value"])
				refreshEligibility, selectionErr := applyRunOptionSelection(renderer, &navigation, field, value)
				if selectionErr != nil {
					renderer.SetStatus(selectionErr.Error())
					window.Invalidate()
					continue
				}
				if !refreshEligibility {
					window.Invalidate()
					continue
				}
				renderer.SetRootBinding("runOptions", "target_kind", "loading")
				renderer.SetStatus("Refreshing eligible agents…")
				startRunOptionsLoad(navigation)
				window.Invalidate()
				continue
			}
			previous := navigation
			handleCommand(ctx, client, renderer, screens, &navigation, command, preferencesPath)
			if navigation.screen == "settings" {
				applyConnectionBindings(renderer, "settings", mode, endpoint)
				renderer.SetRootBinding("settings", "client_version", options.Version)
			}
			if navigation != previous {
				if navigation.screen == "job-details" {
					startOutput(navigation.jobID)
				} else {
					stopOutput()
				}
			}
			scheduleStatusExpiry()
			window.Invalidate()
		}
	}
}

func openExternalURL(raw string) error {
	return openPlatformURL(raw)
}

func connectionBindingData(mode, endpoint, status string, canBack bool) map[string]any {
	if mode != connectionModeExplicit {
		mode = connectionModeDiscover
	}
	return map[string]any{"connection": map[string]any{
		"mode": mode, "endpoint": endpoint, "explicit": mode == connectionModeExplicit,
		"status": status, "status_tone": connectionStatusTone(status), "can_back": canBack,
		"modes": []any{
			map[string]any{"value": connectionModeDiscover, "label": "Automatic discovery"},
			map[string]any{"value": connectionModeExplicit, "label": "Explicit endpoint"},
		},
	}}
}

func connectionStatus(client *cnpclient.Client, address string) string {
	if client == nil {
		return "Not connected"
	}
	return "Connected to " + address
}

func connectionStatusTone(status string) string {
	if strings.HasPrefix(status, "Connected to ") {
		return "success"
	}
	return "danger"
}

func applyConnectionBindings(renderer *Renderer, screen, mode, endpoint string) {
	explicit := mode == connectionModeExplicit
	if screen == "settings" {
		renderer.SetRootBinding("settings", "connection_mode", mode)
		renderer.SetRootBinding("settings", "connection_endpoint", endpoint)
		renderer.SetRootBinding("settings", "connection_explicit", explicit)
		return
	}
	renderer.SetRootBinding("connection", "mode", mode)
	renderer.SetRootBinding("connection", "endpoint", endpoint)
	renderer.SetRootBinding("connection", "explicit", explicit)
}

func handleCommand(ctx context.Context, client *cnpclient.Client, renderer *Renderer, screens map[string]*uidsl.ScreenDocument, navigation *navigationState, command commandRequest, preferencesPath string) {
	switch command.action.Command {
	case "set-project-structure-filter":
		filter := strings.TrimSpace(command.arguments["value"])
		if filter == "" || !renderer.SetProjectStructureFilter(filter) {
			renderer.SetStatus("Project structure filter is unavailable")
			return
		}
		renderer.SetStatus("")
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
		target := strings.TrimSpace(result.PipelineId)
		if jobID := strings.TrimSpace(command.arguments["pipelineJobId"]); jobID != "" {
			target += " / " + jobID
		}
		if dryRun {
			renderer.SetTransientStatus(fmt.Sprintf("Queued %d dry-run execution(s) for %s", result.Enqueued, target), nativeNoticeDuration)
		} else {
			renderer.SetTransientStatus(fmt.Sprintf("Queued %d execution(s) for %s", result.Enqueued, target), nativeNoticeDuration)
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
		chainLabel := strings.TrimSpace(result.ChainName)
		if chainLabel == "" {
			chainLabel = strings.TrimSpace(result.ChainId)
		}
		if dryRun {
			renderer.SetTransientStatus(fmt.Sprintf("Queued %d dry-run execution(s) for chain %s", result.Enqueued, chainLabel), nativeNoticeDuration)
		} else {
			renderer.SetTransientStatus(fmt.Sprintf("Queued %d execution(s) for chain %s", result.Enqueued, chainLabel), nativeNoticeDuration)
		}
	case "set-run-option":
		field := strings.TrimSpace(command.arguments["field"])
		value := strings.TrimSpace(command.arguments["value"])
		refreshEligibility, err := applyRunOptionSelection(renderer, navigation, field, value)
		if err != nil {
			renderer.SetStatus(err.Error())
			return
		}
		if refreshEligibility {
			if err := refreshRunOptions(ctx, client, renderer, screens["run-options"], *navigation); err != nil {
				renderer.SetStatus("Run options refresh failed: " + err.Error())
			}
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
		renderer.SetTransientStatus(fmt.Sprintf("Cleared %d queued execution(s)", result.Cleared), nativeNoticeDuration)
	case "remove-execution":
		jobID := strings.TrimSpace(command.arguments["jobExecutionId"])
		if jobID == "" {
			renderer.SetStatus("No execution identifier was supplied")
			return
		}
		renderer.SetStatus("Removing queued execution…")
		commandCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		result, err := client.RemoveQueuedExecution(commandCtx, jobID, "")
		cancel()
		if err != nil {
			renderer.SetStatus("Remove failed: " + err.Error())
			return
		}
		if err := refreshScreen(ctx, client, renderer, screens, *navigation); err != nil {
			renderer.SetStatus("Execution removed, but refresh failed: " + err.Error())
			return
		}
		renderer.SetTransientStatus("Removed queued execution "+result.JobExecutionId, nativeNoticeDuration)
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
		renderer.SetTransientStatus(fmt.Sprintf("Removed %d execution(s) from history", result.Flushed), nativeNoticeDuration)
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
		renderer.SetTransientStatus("Execution "+result.JobExecutionId+" marked failed", nativeNoticeDuration)
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
		renderer.SetTransientStatus("Queued rerun "+result.JobExecutionId, nativeNoticeDuration)
	case "agent-action":
		agentID := strings.TrimSpace(command.arguments["agentId"])
		action := strings.TrimSpace(command.arguments["action"])
		if agentID == "" || action == "" {
			renderer.SetStatus("Agent action is incomplete")
			return
		}
		renderer.SetStatus("Sending agent request…")
		commandCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		result, err := client.AgentAction(commandCtx, &cnpv1.AgentActionRequest{AgentId: agentID, Action: action}, "")
		cancel()
		if err != nil {
			renderer.SetStatus("Agent action failed: " + err.Error())
			return
		}
		if err := refreshScreen(ctx, client, renderer, screens, *navigation); err != nil {
			renderer.SetStatus("Agent action accepted, but refresh failed: " + err.Error())
			return
		}
		message := strings.TrimSpace(result.Message)
		if message == "" {
			message = "Agent request accepted"
		}
		renderer.SetTransientStatus(message, nativeNoticeDuration)
	case "project-action":
		projectID, err := strconv.ParseInt(strings.TrimSpace(command.arguments["projectId"]), 10, 64)
		action := strings.TrimSpace(command.arguments["action"])
		if err != nil || projectID <= 0 || action == "" {
			renderer.SetStatus("Project action is incomplete")
			return
		}
		projectKey := strconv.FormatInt(projectID, 10)
		progress := "Updating…"
		if action == "reload" {
			progress = "Reloading…"
		} else if action == "delete" {
			progress = "Deleting…"
		}
		renderer.SetRepeatedItemBinding("settings", "projects", "id", projectKey, "action_status", progress)
		renderer.SetRepeatedItemBinding("settings", "projects", "id", projectKey, "action_tone", "muted")
		commandCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
		result, err := client.ProjectAction(commandCtx, projectID, action, "")
		cancel()
		if err != nil {
			failure := "Action failed: " + err.Error()
			if action == "reload" {
				failure = "Reload failed: " + err.Error()
			}
			renderer.SetRepeatedItemBinding("settings", "projects", "id", projectKey, "action_status", failure)
			renderer.SetRepeatedItemBinding("settings", "projects", "id", projectKey, "action_tone", "danger")
			return
		}
		if err := refreshScreen(ctx, client, renderer, screens, *navigation); err != nil {
			renderer.SetStatus("Project updated, but refresh failed: " + err.Error())
			return
		}
		if action == "reload" {
			renderer.SetRepeatedItemBinding("settings", "projects", "id", projectKey, "action_status", "Reloaded successfully")
			renderer.SetRepeatedItemBinding("settings", "projects", "id", projectKey, "action_tone", "success")
		} else {
			renderer.SetTransientStatus(result.Message, nativeNoticeDuration)
		}
	case "set-project-import-field":
		binding := map[string]string{"repoUrl": "import_repo_url", "repoRef": "import_repo_ref", "configFile": "import_config_file"}[strings.TrimSpace(command.arguments["field"])]
		if binding == "" {
			renderer.SetStatus("Unknown project import field")
			return
		}
		renderer.SetRootBinding("settings", binding, command.arguments["value"])
	case "import-project":
		repoURL := strings.TrimSpace(command.arguments["repoUrl"])
		if repoURL == "" {
			renderer.SetStatus("Repository URL is required")
			return
		}
		renderer.SetStatus("Importing project…")
		commandCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
		result, err := client.ImportProject(commandCtx, &cnpv1.ImportProjectRequest{
			RepoUrl: repoURL, RepoRef: strings.TrimSpace(command.arguments["repoRef"]),
			ConfigFile: strings.TrimSpace(command.arguments["configFile"]),
		}, "")
		cancel()
		if err != nil {
			renderer.SetStatus("Project import failed: " + err.Error())
			return
		}
		if err := refreshScreen(ctx, client, renderer, screens, *navigation); err != nil {
			renderer.SetStatus("Project imported, but refresh failed: " + err.Error())
			return
		}
		renderer.SetTransientStatus("Imported "+result.ProjectName, nativeNoticeDuration)
	case "set-server-update-option":
		binding := map[string]string{
			"update": "selected_update_version", "rollback": "selected_rollback_version",
		}[strings.TrimSpace(command.arguments["field"])]
		if binding == "" {
			renderer.SetStatus("Unknown server update option")
			return
		}
		renderer.SetRootBinding("settings", binding, strings.TrimSpace(command.arguments["value"]))
	case "check-server-updates":
		renderer.SetRootBinding("settings", "update_result", "Checking for updates…")
		renderer.SetRootBinding("settings", "update_result_tone", "muted")
		commandCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
		result, err := client.CheckServerUpdates(commandCtx)
		cancel()
		if err != nil {
			renderer.SetRootBinding("settings", "update_versions", versionOptions(nil, "Update check failed"))
			renderer.SetRootBinding("settings", "selected_update_version", "")
			renderer.SetRootBinding("settings", "update_result", "Update check failed: "+err.Error())
			renderer.SetRootBinding("settings", "update_result_tone", "danger")
			return
		}
		renderer.SetRootBinding("settings", "update_versions", versionOptions(result.AvailableVersions, "No newer versions available"))
		selected := ""
		if len(result.AvailableVersions) > 0 {
			selected = result.AvailableVersions[0]
		}
		renderer.SetRootBinding("settings", "selected_update_version", selected)
		message := strings.TrimSpace(result.Message)
		if result.UpdateAvailable {
			message = "Update available: " + result.CurrentVersion + " → " + result.LatestVersion
			if result.AssetName != "" {
				message += " (" + result.AssetName + ")"
			}
		} else if message == "" {
			message = "Up to date (" + result.CurrentVersion + ")"
		}
		renderer.SetRootBinding("settings", "update_result", message)
		renderer.SetRootBinding("settings", "update_result_tone", "success")
	case "refresh-rollback-versions":
		renderer.SetRootBinding("settings", "rollback_result", "Refreshing versions…")
		renderer.SetRootBinding("settings", "rollback_result_tone", "muted")
		commandCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
		result, err := client.ListServerUpdateVersions(commandCtx)
		cancel()
		if err != nil {
			renderer.SetRootBinding("settings", "rollback_versions", versionOptions(nil, "Version load failed"))
			renderer.SetRootBinding("settings", "selected_rollback_version", "")
			renderer.SetRootBinding("settings", "rollback_result", "Version load failed: "+err.Error())
			renderer.SetRootBinding("settings", "rollback_result_tone", "danger")
			return
		}
		renderer.SetRootBinding("settings", "rollback_versions", versionOptions(result.Versions, "No lower versions available"))
		selected := ""
		if len(result.Versions) > 0 {
			selected = result.Versions[0]
		}
		renderer.SetRootBinding("settings", "selected_rollback_version", selected)
		renderer.SetRootBinding("settings", "rollback_result", fmt.Sprintf("Found %d rollback version(s)", len(result.Versions)))
		renderer.SetRootBinding("settings", "rollback_result_tone", "success")
	case "server-update-action":
		action := strings.TrimSpace(command.arguments["action"])
		target := strings.TrimSpace(command.arguments["targetVersion"])
		resultField := "update_result"
		if action == "rollback" {
			resultField = "rollback_result"
		}
		if (action == "apply" || action == "rollback") && target == "" {
			renderer.SetRootBinding("settings", resultField, "Select a version first")
			renderer.SetRootBinding("settings", resultField+"_tone", "danger")
			return
		}
		progress := "Starting " + action + "…"
		if action == "restart" {
			progress = "Requesting server restart…"
		}
		renderer.SetRootBinding("settings", resultField, progress)
		renderer.SetRootBinding("settings", resultField+"_tone", "muted")
		commandCtx, cancel := context.WithTimeout(ctx, 35*time.Second)
		result, err := client.ServerUpdateAction(commandCtx, action, target)
		cancel()
		if err != nil {
			label := map[string]string{"apply": "Update", "rollback": "Rollback", "restart": "Restart"}[action]
			if label == "" {
				label = "Request"
			}
			renderer.SetRootBinding("settings", resultField, label+" failed: "+err.Error())
			renderer.SetRootBinding("settings", resultField+"_tone", "danger")
			return
		}
		message := strings.TrimSpace(result.Message)
		if message == "" {
			message = "Request accepted"
		}
		renderer.SetRootBinding("settings", resultField, message)
		renderer.SetRootBinding("settings", resultField+"_tone", "success")
	case "refresh":
		if err := refreshScreen(ctx, client, renderer, screens, *navigation); err != nil {
			renderer.SetStatus("Refresh failed: " + err.Error())
			return
		}
		renderer.SetTransientStatus("Refreshed", nativeNoticeDuration)
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
		renderer.SetTransientStatus("Theme: "+theme.Metadata.Title, nativeNoticeDuration)
	default:
		renderer.SetStatus("Unsupported native action: " + command.action.Command)
	}
}

func applyRunOptionSelection(renderer *Renderer, navigation *navigationState, field, value string) (bool, error) {
	switch field {
	case "sourceRef":
		navigation.sourceRef = value
		navigation.agentID = ""
		renderer.SetRootBinding("runOptions", "selected_source_ref", value)
		renderer.SetRootBinding("runOptions", "selected_agent_id", "")
		return true, nil
	case "agentId":
		navigation.agentID = value
		renderer.SetRootBinding("runOptions", "selected_agent_id", value)
		return false, nil
	default:
		return false, fmt.Errorf("unsupported run option")
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
	next, err := navigationForRoute(route)
	if err != nil {
		return err
	}
	if err := refreshScreen(ctx, client, renderer, screens, next); err != nil {
		return err
	}
	*navigation = next
	renderer.SetStatus("")
	return nil
}

func navigationForRoute(route string) (navigationState, error) {
	route = strings.TrimSpace(route)
	next := navigationState{}
	switch {
	case route == "/":
		next.screen = "front-page"
	case strings.HasPrefix(route, "/projects/"):
		projectID, err := strconv.ParseInt(strings.Trim(strings.TrimPrefix(route, "/projects/"), "/"), 10, 64)
		if err != nil || projectID <= 0 {
			return navigationState{}, fmt.Errorf("invalid project route %q", route)
		}
		next = navigationState{screen: "project-details", projectID: projectID}
	case strings.HasPrefix(route, "/jobs/"):
		jobID := strings.Trim(strings.TrimPrefix(route, "/jobs/"), "/")
		if jobID == "" || strings.Contains(jobID, "/") {
			return navigationState{}, fmt.Errorf("invalid job route %q", route)
		}
		next = navigationState{screen: "job-details", jobID: jobID}
	case strings.HasPrefix(route, "/run-options/projects/") && strings.Contains(route, "/pipelines/"):
		parts := strings.Split(strings.Trim(strings.TrimPrefix(route, "/run-options/projects/"), "/"), "/")
		if len(parts) != 3 || parts[1] != "pipelines" {
			return navigationState{}, fmt.Errorf("invalid pipeline run-options route %q", route)
		}
		projectID, projectErr := strconv.ParseInt(parts[0], 10, 64)
		pipelineID, pipelineErr := strconv.ParseInt(parts[2], 10, 64)
		if projectErr != nil || projectID <= 0 || pipelineErr != nil || pipelineID <= 0 {
			return navigationState{}, fmt.Errorf("invalid pipeline run-options route %q", route)
		}
		next = navigationState{screen: "run-options", projectID: projectID, pipelineDBID: pipelineID}
	case strings.HasPrefix(route, "/run-options/pipelines/"):
		pipelineID, err := strconv.ParseInt(strings.Trim(strings.TrimPrefix(route, "/run-options/pipelines/"), "/"), 10, 64)
		if err != nil || pipelineID <= 0 {
			return navigationState{}, fmt.Errorf("invalid pipeline run-options route %q", route)
		}
		next = navigationState{screen: "run-options", pipelineDBID: pipelineID}
	case strings.HasPrefix(route, "/run-options/projects/"):
		parts := strings.Split(strings.Trim(strings.TrimPrefix(route, "/run-options/projects/"), "/"), "/")
		if len(parts) != 3 || parts[1] != "chains" {
			return navigationState{}, fmt.Errorf("invalid chain run-options route %q", route)
		}
		projectID, err := strconv.ParseInt(parts[0], 10, 64)
		if err != nil || projectID <= 0 || strings.TrimSpace(parts[2]) == "" {
			return navigationState{}, fmt.Errorf("invalid chain run-options route %q", route)
		}
		next = navigationState{screen: "run-options", projectID: projectID, chainID: parts[2]}
	case route == "/settings":
		next.screen = "settings"
	case route == "/agents":
		next.screen = "agents"
	case strings.HasPrefix(route, "/agents/"):
		agentID := strings.Trim(strings.TrimPrefix(route, "/agents/"), "/")
		if agentID == "" || strings.Contains(agentID, "/") {
			return navigationState{}, fmt.Errorf("invalid agent route %q", route)
		}
		next = navigationState{screen: "agent-details", agentDetailsID: agentID}
	case route == "/connection":
		next.screen = "connection"
	default:
		return navigationState{}, fmt.Errorf("unsupported route %q", route)
	}
	return next, nil
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
	case "run-options":
		return refreshRunOptions(ctx, client, renderer, screen, navigation)
	case "agents":
		return refreshAgents(ctx, client, renderer, screen)
	case "agent-details":
		return refreshAgentDetails(ctx, client, renderer, screen, navigation.agentDetailsID)
	case "connection":
		return nil
	default:
		return fmt.Errorf("screen %q is unsupported", navigation.screen)
	}
}

func refreshAgents(ctx context.Context, client *cnpclient.Client, renderer *Renderer, screen *uidsl.ScreenDocument) error {
	requestCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	view, err := client.GetAgentsView(requestCtx)
	if err != nil {
		return err
	}
	data, err := protobufBindingData("agents", "agents", view)
	if err != nil {
		return err
	}
	renderer.SetScreenAndData(screen, data)
	return nil
}

func refreshAgentDetails(ctx context.Context, client *cnpclient.Client, renderer *Renderer, screen *uidsl.ScreenDocument, agentID string) error {
	requestCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	view, err := client.GetAgentDetails(requestCtx, agentID)
	if err != nil {
		return err
	}
	data, err := protobufBindingData("agentDetails", "agent details", view)
	if err != nil {
		return err
	}
	renderer.SetScreenAndData(screen, data)
	return nil
}

func refreshRunOptions(ctx context.Context, client *cnpclient.Client, renderer *Renderer, screen *uidsl.ScreenDocument, navigation navigationState) error {
	data, err := loadRunOptions(ctx, client, navigation)
	if err != nil {
		return err
	}
	renderer.SetScreenAndData(screen, data)
	return nil
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

func refreshSettings(ctx context.Context, client *cnpclient.Client, renderer *Renderer, screen *uidsl.ScreenDocument) error {
	requestCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	server, err := client.GetServerInfo(requestCtx)
	if err != nil {
		return err
	}
	projects, err := client.ListProjects(requestCtx)
	if err != nil {
		return err
	}
	updateStatus, updateStatusErr := client.GetServerUpdateStatus(requestCtx)
	themes, err := sharedUI.LoadThemes()
	if err != nil {
		return err
	}
	data, err := settingsBindingData(server, themes, renderer.ThemeName())
	if err != nil {
		return err
	}
	settings, ok := data["settings"].(map[string]any)
	if !ok {
		return fmt.Errorf("settings binding is malformed")
	}
	projectData, err := protobufBindingData("projects", "settings projects", projects)
	if err != nil {
		return err
	}
	projectRoot, ok := projectData["projects"].(map[string]any)
	if !ok {
		return fmt.Errorf("settings projects binding is malformed")
	}
	projectItems, _ := projectRoot["projects"].([]any)
	decorateSettingsProjects(projectItems)
	settings["projects"] = projectItems
	decorateSettingsUpdate(settings, updateStatus, updateStatusErr)
	renderer.SetScreenAndData(screen, data)
	return nil
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
		sourceKind := strings.TrimSpace(fmt.Sprint(project["source_kind"]))
		project["can_reload"] = sourceKind != "managed_yaml"
		project["action_status"] = ""
		project["action_tone"] = "muted"
		commit := strings.TrimSpace(fmt.Sprint(project["loaded_commit"]))
		shortCommit := commit
		if len(shortCommit) > 8 {
			shortCommit = shortCommit[:8]
		}
		project["loaded_commit_short"] = shortCommit
		project["loaded_commit_url"] = loadedCommitURL(strings.TrimSpace(fmt.Sprint(project["repo_url"])), commit)
		project["updated_label"] = formatLoadedProjectTime(project["updated_unix_ms"])
		project["has_loaded_commit"] = commit != ""
		if sourceKind == "managed_yaml" {
			project["source_label"] = "Managed YAML stored in ciwi"
			continue
		}
		label := strings.TrimSpace(fmt.Sprint(project["repo_url"]))
		if ref := strings.TrimSpace(fmt.Sprint(project["repo_ref"])); ref != "" {
			label += " · " + ref
		}
		project["source_label"] = label
	}
}

func loadedCommitURL(repoURL, commit string) string {
	if repoURL == "" || commit == "" {
		return ""
	}
	repoURL = strings.TrimSuffix(repoURL, ".git")
	if strings.HasPrefix(repoURL, "https://") || strings.HasPrefix(repoURL, "http://") {
		return strings.TrimRight(repoURL, "/") + "/commit/" + commit
	}
	return ""
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
	decorateFrontPageProjects(root["projects"])
	decorateExecutionCards(root["queued_executions"], true)
	decorateExecutionCards(root["history_executions"], false)
	queued, _ := root["queued_executions"].([]any)
	history, _ := root["history_executions"].([]any)
	root["queued_empty"] = len(queued) == 0
	root["history_empty"] = len(history) == 0
	return data, nil
}

func decorateFrontPageProjects(value any) {
	projects, ok := value.([]any)
	if !ok {
		return
	}
	for _, raw := range projects {
		project, projectOK := raw.(map[string]any)
		if !projectOK {
			continue
		}
		pipelines, _ := project["pipelines"].([]any)
		label := "pipelines"
		if len(pipelines) == 1 {
			label = "pipeline"
		}
		project["pipeline_count_label"] = fmt.Sprintf("%d %s", len(pipelines), label)
	}
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
		if sections, ok := entry["sections"].([]any); ok {
			for _, rawSection := range sections {
				section, _ := rawSection.(map[string]any)
				jobs, _ := section["jobs"].([]any)
				for _, rawJob := range jobs {
					job, _ := rawJob.(map[string]any)
					ensureSchedulingDiagnosisBinding(job)
					decorateExecutionCardJob(job)
				}
			}
		}
	}
}

func decorateExecutionCardJob(job map[string]any) {
	if job == nil {
		return
	}
	created := parseExecutionCardTime(job["created_utc"])
	started := parseExecutionCardTime(job["started_utc"])
	finished := parseExecutionCardTime(job["finished_utc"])
	job["created_label"] = formatExecutionCardTimestamp(created)
	job["duration_label"] = formatExecutionCardDuration(started, finished, strings.TrimSpace(fmt.Sprint(job["status"])))
}

func parseExecutionCardTime(value any) time.Time {
	parsed, _ := time.Parse(time.RFC3339Nano, strings.TrimSpace(fmt.Sprint(value)))
	return parsed
}

func formatExecutionCardTimestamp(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.Local().Format("Mon 02 Jan, 15:04:05")
}

func formatExecutionCardDuration(started, finished time.Time, status string) string {
	if started.IsZero() {
		return ""
	}
	end := finished
	if end.IsZero() && protocol.NormalizeJobExecutionStatus(status) == protocol.JobExecutionStatusRunning {
		end = time.Now()
	}
	if end.IsZero() || end.Before(started) {
		return ""
	}
	totalSeconds := int(end.Sub(started).Seconds())
	hours := totalSeconds / 3600
	minutes := (totalSeconds % 3600) / 60
	seconds := totalSeconds % 60
	if hours > 0 {
		return fmt.Sprintf("%02dh %02dm %02ds", hours, minutes, seconds)
	}
	return fmt.Sprintf("%02dm %02ds", minutes, seconds)
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
	decorateExecutionCards(root["history_executions"], false)
	history, _ := root["history_executions"].([]any)
	root["history_empty"] = len(history) == 0
	return data, nil
}

func decorateProjectDetails(root map[string]any) {
	if project, ok := root["project"].(map[string]any); ok {
		metadata := make([]string, 0, 2)
		if repoRef := strings.TrimSpace(fmt.Sprint(project["repo_ref"])); repoRef != "" {
			metadata = append(metadata, "branch: "+repoRef)
		}
		if configFile := strings.TrimSpace(fmt.Sprint(project["config_file"])); configFile != "" {
			metadata = append(metadata, configFile)
		}
		project["source_metadata"] = strings.Join(metadata, " · ")
		chains, _ := project["pipeline_chains"].([]any)
		project["has_pipeline_chains"] = len(chains) > 0
	}
	pipelines, _ := root["pipelines"].([]any)
	root["structure_filter"] = "all-pipelines"
	root["structure_filters"] = projectStructureFilterOptions(root, pipelines)
	root["visible_pipelines"] = append([]any(nil), pipelines...)
	for _, rawPipeline := range pipelines {
		pipeline, pipelineOK := rawPipeline.(map[string]any)
		if !pipelineOK {
			continue
		}
		jobsCount := int(numberValue(pipeline["jobs_count"]))
		jobNoun := "jobs"
		if jobsCount == 1 {
			jobNoun = "job"
		}
		dependencies := strings.TrimSpace(fmt.Sprint(pipeline["dependencies"]))
		if dependencies == "" {
			dependencies = "none"
		}
		pipeline["summary_label"] = fmt.Sprintf("%d %s · depends on: %s", jobsCount, jobNoun, dependencies)
		dependsOn, _ := pipeline["depends_on"].([]any)
		dependencyNoun := "dependencies"
		if len(dependsOn) == 1 {
			dependencyNoun = "dependency"
		}
		pipeline["graph_summary_label"] = fmt.Sprintf("%d %s · %d %s", jobsCount, jobNoun, len(dependsOn), dependencyNoun)
		jobs, _ := pipeline["jobs"].([]any)
		for _, rawJob := range jobs {
			job, jobOK := rawJob.(map[string]any)
			if !jobOK {
				continue
			}
			stepsCount := int(numberValue(job["steps_count"]))
			stepNoun := "steps"
			if stepsCount == 1 {
				stepNoun = "step"
			}
			runsOn := defaultLabel(job["runs_on_label"], "unspecified")
			job["needs_label"] = defaultLabel(job["needs_label"], "none")
			job["tools_label"] = defaultLabel(job["tools_label"], "none")
			job["summary_label"] = fmt.Sprintf("%d %s · runs on: %s", stepsCount, stepNoun, runsOn)
			job["timeout_label"] = fmt.Sprintf("Timeout: %ds", int(numberValue(job["timeout_seconds"])))
			matrixCount := int(numberValue(job["matrix_count"]))
			if matrixCount == 0 {
				job["matrix_label"] = "Matrix: none"
			} else {
				matrixNoun := "executions"
				if matrixCount == 1 {
					matrixNoun = "execution"
				}
				job["matrix_label"] = fmt.Sprintf("Matrix: %d %s", matrixCount, matrixNoun)
			}
			steps, _ := job["steps"].([]any)
			for _, rawStep := range steps {
				step, stepOK := rawStep.(map[string]any)
				if !stepOK {
					continue
				}
				step["environment_label"] = stringListLabel(step["environment"])
				if strings.TrimSpace(fmt.Sprint(step["command"])) == "" {
					step["command"] = "(no command)"
				}
			}
		}
	}
}

func projectStructureFilterOptions(root map[string]any, pipelines []any) []any {
	options := []any{
		map[string]any{"value": "all-pipelines", "label": "All Pipelines"},
		map[string]any{"value": "all-chains", "label": "All chains"},
	}
	project, _ := root["project"].(map[string]any)
	chains, _ := project["pipeline_chains"].([]any)
	for _, raw := range chains {
		chain, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		id := strings.TrimSpace(fmt.Sprint(chain["id"]))
		if id == "" {
			continue
		}
		name := strings.TrimSpace(fmt.Sprint(chain["name"]))
		if name == "" {
			name = strings.TrimSpace(fmt.Sprint(chain["sequence_label"]))
		}
		options = append(options, map[string]any{"value": "chain:" + id, "label": name + " (chain)"})
	}
	return options
}

func defaultLabel(value any, fallback string) string {
	if text := strings.TrimSpace(fmt.Sprint(value)); text != "" {
		return text
	}
	return fallback
}

func stringListLabel(value any) string {
	items, ok := value.([]any)
	if !ok {
		return ""
	}
	parts := make([]string, 0, len(items))
	for _, item := range items {
		if text := strings.TrimSpace(fmt.Sprint(item)); text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, " · ")
}

func jobDetailsBindingData(view *cnpv1.JobDetailsView) (map[string]any, error) {
	data, err := protobufBindingData("jobDetails", "job-details", view)
	if err != nil {
		return nil, err
	}
	if root, ok := data["jobDetails"].(map[string]any); ok {
		ensureSchedulingDiagnosisBinding(root)
		root["output"] = ""
		root["system_output"] = ""
		root["output_search"] = ""
		root["output_search_count"] = "0/0"
		root["tailing_label"] = "Tailing: Off"
		if groups, ok := root["output_groups"].([]any); ok {
			for _, raw := range groups {
				entry, entryOK := raw.(map[string]any)
				if !entryOK {
					continue
				}
				entry["output"] = ""
				entry["is_phase"] = fmt.Sprint(entry["kind"]) == "phase"
				entry["is_step"] = fmt.Sprint(entry["kind"]) != "phase"
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
		},
		"import_repo_url": "", "import_repo_ref": "", "import_config_file": "ciwi-project.yaml",
		"update_supported": false, "update_capability_notice": "Update status unavailable", "update_status_label": "", "blocked_agent_notice": "",
		"update_current_version": "", "update_last_apply_status": "",
		"update_versions": versionOptions(nil, "Check for updates"), "selected_update_version": "",
		"rollback_versions": versionOptions(nil, "Refresh versions"), "selected_rollback_version": "",
		"update_result": "", "update_result_tone": "muted", "rollback_result": "", "rollback_result_tone": "muted",
	}}, nil
}

func offlineFrontPageBindingData(clientVersion string) (map[string]any, error) {
	return frontPageBindingData(&cnpv1.FrontPageView{
		Server: &cnpv1.ServerInfo{Version: strings.TrimSpace(clientVersion)},
	})
}

func offlineSettingsBindingData(clientVersion, selectedTheme, mode, endpoint string) (map[string]any, error) {
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
	settings["update_capability_notice"] = "Connect to a server to manage projects and server updates."
	return data, nil
}

func refreshOfflineScreen(renderer *Renderer, screens map[string]*uidsl.ScreenDocument, navigation navigationState, clientVersion, mode, endpoint string) error {
	switch navigation.screen {
	case "front-page":
		data, err := offlineFrontPageBindingData(clientVersion)
		if err != nil {
			return err
		}
		renderer.SetScreenAndData(screens["front-page"], data)
	case "settings":
		data, err := offlineSettingsBindingData(clientVersion, renderer.ThemeName(), mode, endpoint)
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
	return map[string]any{root: normalized}, nil
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

func relevantScreenChange(navigation navigationState, change *cnpv1.ChangeEvent) bool {
	for _, topic := range change.Topics {
		if navigation.screen == "project-details" && topic == cnpv1.ChangeTopic_CHANGE_TOPIC_PROJECTS {
			return true
		}
		if navigation.screen == "job-details" && (topic == cnpv1.ChangeTopic_CHANGE_TOPIC_QUEUE || topic == cnpv1.ChangeTopic_CHANGE_TOPIC_HISTORY) {
			return true
		}
		if navigation.screen == "settings" && (topic == cnpv1.ChangeTopic_CHANGE_TOPIC_SERVER || topic == cnpv1.ChangeTopic_CHANGE_TOPIC_PROJECTS || topic == cnpv1.ChangeTopic_CHANGE_TOPIC_UPDATES) {
			return true
		}
		if navigation.screen == "run-options" && (topic == cnpv1.ChangeTopic_CHANGE_TOPIC_PROJECTS || topic == cnpv1.ChangeTopic_CHANGE_TOPIC_AGENT_ELIGIBILITY) {
			return true
		}
		if navigation.screen == "agents" && topic == cnpv1.ChangeTopic_CHANGE_TOPIC_AGENTS {
			return true
		}
		if navigation.screen == "agent-details" && topic == cnpv1.ChangeTopic_CHANGE_TOPIC_AGENTS {
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
