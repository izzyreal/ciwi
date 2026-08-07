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
	"github.com/izzyreal/ciwi/internal/domain"
	"github.com/izzyreal/ciwi/internal/presentation"
	"github.com/izzyreal/ciwi/internal/presentation/operations"
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
	agentScriptID  string
	scriptShell    string
	script         string
}

type screenLoadResult struct {
	navigation          navigationState
	generation          uint64
	recoverMissingRoute bool
	data                map[string]any
	err                 error
}

type jobOutputBuffer struct {
	jobID   string
	events  []*cnpv1.JobOutputEvent
	omitted map[string]bool
}

const (
	maxNativeOutputBytes = 1024 * 1024
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
	return watchNativeSession(ctx, client, target)
}

func connectSSHNativeSession(ctx context.Context, settings sshConnectionSettings, version string) (*nativeSession, error) {
	connectCtx, cancelConnect := context.WithTimeout(ctx, 15*time.Second)
	defer cancelConnect()
	client, err := cnpclient.DialSSH(connectCtx, cnpclient.SSHConfig{
		JumpAddress: settings.JumpAddress, Username: settings.Username, Destination: settings.Destination,
		PrivateKeyPEM: settings.PrivateKey, HostKeyFingerprint: settings.HostKeyFingerprint,
	}, "ciwi-desktop", version)
	if err != nil {
		return nil, fmt.Errorf("connect through remote server: %w", err)
	}
	address := fmt.Sprintf("SSH %s@%s → %s", strings.TrimSpace(settings.Username), strings.TrimSpace(settings.JumpAddress), strings.TrimSpace(settings.Destination))
	return watchNativeSession(ctx, client, address)
}

func watchNativeSession(ctx context.Context, client *cnpclient.Client, address string) (*nativeSession, error) {
	watchCtx, cancelWatch := context.WithCancel(ctx)
	changes, watchErrors, watchErr := client.WatchChanges(watchCtx)
	if watchErr != nil {
		cancelWatch()
		_ = client.Close()
		return nil, fmt.Errorf("connect to ciwi native endpoint: %s: start live updates: %w", address, watchErr)
	}
	return &nativeSession{client: client, address: address, changes: changes, watchErrors: watchErrors, cancelWatch: cancelWatch}, nil
}

func connectConfiguredNativeSession(ctx context.Context, settings nativeConnectionSettings, version string) (*nativeSession, error) {
	if settings.Mode == connectionModeSSH {
		return connectSSHNativeSession(ctx, settings.SSH, version)
	}
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

func validateNativeBindings(screen *uidsl.ScreenDocument, data map[string]any) error {
	validationData := make(map[string]any, len(data)+1)
	for key, value := range data {
		validationData[key] = value
	}
	if _, ok := validationData["client"]; !ok {
		validationData["client"] = nativeConnectionState{}.binding()
	}
	return uidsl.ValidateBindings(screen, validationData, "gio")
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
	managedYAMLScreen, err := sharedUI.LoadScreen("managed-yaml")
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
	agentScriptScreen, err := sharedUI.LoadScreen("agent-script")
	if err != nil {
		return err
	}
	vaultScreen, err := sharedUI.LoadScreen("vault")
	if err != nil {
		return err
	}
	screens := map[string]*uidsl.ScreenDocument{
		"front-page": frontPageScreen, "project-details": projectDetailsScreen, "job-details": jobDetailsScreen,
		"settings": settingsScreen, "managed-yaml": managedYAMLScreen, "run-options": runOptionsScreen, "agents": agentsScreen, "agent-details": agentDetailsScreen,
		"agent-script": agentScriptScreen, "vault": vaultScreen,
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
	actionCatalog, err := sharedUI.LoadActionCatalog()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	clientBroker := newNativeClientBroker()
	operationJournal := newNativeOperationJournal(preferencesPath, clientBroker.ServerInstallationID)
	coordinator := operations.New(ctx, 4, nativeOperationExecutor{clients: clientBroker}, operationJournal)
	defer coordinator.Close()
	commands := make(chan commandRequest, 256)
	var renderer *Renderer
	renderer, err = NewRenderer(frontPageScreen, theme, func(action uidsl.Action, arguments map[string]string) {
		spec, ok := actionCatalog.Spec(action.Command)
		if ok && spec.Class != uidsl.ActionClassLocal {
			submission, submitErr := coordinator.Submit(operations.Request{
				Definition: operations.Definition{
					Command: action.Command, Class: operations.Class(spec.Class), Scope: spec.ResolveScope(arguments),
					Pending: spec.Pending, Persistence: spec.Persistence,
				},
				Arguments: arguments,
			})
			if submitErr != nil {
				renderer.ShowAlert("Action could not be started", submitErr.Error())
				window.Invalidate()
				return
			}
			switch submission.Disposition {
			case operations.DispositionDuplicate:
				renderer.ShowAlert("Action already in progress", "That action is already in progress.")
			case operations.DispositionConflict:
				message := "A conflicting action is already in progress"
				if submission.Conflict != nil && strings.TrimSpace(submission.Conflict.PendingLabel) != "" {
					message = submission.Conflict.PendingLabel
				}
				renderer.ShowAlert("Action unavailable", message)
			}
			renderer.SetOperations(coordinator.Snapshot())
			window.Invalidate()
			return
		}
		select {
		case commands <- commandRequest{action: action, arguments: arguments}:
		default:
			renderer.ShowAlert("Action unavailable", "Another command is already being processed.")
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
	go runController(ctx, window, renderer, commands, screens, options, preferencesPath, preferences, coordinator, clientBroker, operationJournal)

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

func runController(ctx context.Context, window *app.Window, renderer *Renderer, commands <-chan commandRequest, screens map[string]*uidsl.ScreenDocument, options Options, preferencesPath string, preferences nativePreferences, coordinator *operations.Coordinator, clientBroker *nativeClientBroker, operationJournal *nativeOperationJournal) {
	screenCache := newNativeScreenCache()
	pendingCancellations := map[string]bool{}
	connectionSettings := nativeConnectionSettingsForLaunch(preferences, options.Address)
	mode, endpoint := connectionSettings.Mode, connectionSettings.Endpoint
	sshSettings := connectionSettings.SSH
	privateKey, privateKeyErr := loadSSHDevicePrivateKey(preferencesPath)
	if privateKeyErr == nil {
		sshSettings.PrivateKey = privateKey
		connectionSettings.SSH.PrivateKey = privateKey
	}
	var session *nativeSession
	var client *cnpclient.Client
	var changes <-chan *cnpv1.ChangeEvent
	var watchErrors <-chan error
	address := ""
	reconnectDelay := time.Second
	var connected *nativeSession
	var initialConnectErr error
	if mode == connectionModeSSH {
		initialConnectErr = privateKeyErr
	}
	if initialConnectErr == nil {
		connected, initialConnectErr = connectConfiguredNativeSession(ctx, connectionSettings, options.Version)
	}
	initialHostKeyPending := captureSSHHostKeyError(&sshSettings, initialConnectErr)
	connectionSettings.SSH = sshSettings
	if initialConnectErr == nil {
		session = connected
		client = connected.client
		clientBroker.Set(client)
		changes = connected.changes
		watchErrors = connected.watchErrors
		address = connected.address
		screenCache.SetServerInstallationID(client.Welcome().GetServerInstallationId())
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
	screenLoads := make(chan screenLoadResult, 8)
	var screenLoadCancel context.CancelFunc
	var screenLoadGeneration uint64
	queueScreenLoad := func(target navigationState, recoverMissingRoute bool) {
		if screenLoadCancel != nil {
			screenLoadCancel()
		}
		loadCtx, cancelLoad := context.WithCancel(ctx)
		screenLoadCancel = cancelLoad
		screenLoadGeneration++
		generation := screenLoadGeneration
		activeClient := client
		themeName := renderer.ThemeName()
		go func() {
			if activeClient == nil {
				return
			}
			data, loadErr := loadScreenData(loadCtx, activeClient, target, themeName)
			select {
			case screenLoads <- screenLoadResult{
				navigation: target, generation: generation, recoverMissingRoute: recoverMissingRoute,
				data: data, err: loadErr,
			}:
			case <-ctx.Done():
			}
		}()
	}
	startScreenLoad := func(target navigationState) { queueScreenLoad(target, false) }
	startResyncLoad := func(target navigationState) { queueScreenLoad(target, true) }
	defer func() {
		if screenLoadCancel != nil {
			screenLoadCancel()
		}
	}()
	navigation := navigationState{screen: "front-page"}
	if strings.TrimSpace(options.Route) != "" {
		navigation, _ = navigationForRoute(options.Route)
	}
	if initialConnectErr != nil {
		if navigation.screen != "front-page" && navigation.screen != "settings" {
			navigation = navigationState{screen: "front-page"}
		}
		if err := refreshOfflineScreen(renderer, screens, navigation, options.Version, mode, endpoint, sshSettings); err != nil {
			renderer.SetStatus(err.Error())
		}
		state := nativeConnectionState{connecting: true, status: "Server unavailable; reconnecting…"}
		if initialHostKeyPending {
			state = nativeConnectionState{status: "SSH host key verification required. Connection attempts are paused."}
		}
		applyNativeConnectionState(renderer, state)
	} else {
		if mode != connectionModeSSH {
			rememberSuccessfulEndpoint(preferencesPath, address)
			preferences.LastSuccessfulEndpoint = address
		}
		if mode == connectionModeDiscover {
			connectionSettings.PreferredAddress = address
			connectionSettings.DiscoverFallback = true
		}
		applyNativeConnectionState(renderer, nativeConnectionState{connected: true, address: address, status: "Connected to " + address})
		if navigation.screen == "settings" {
			applyConnectionBindings(renderer, "settings", mode, endpoint, sshSettings)
			renderer.SetRootBinding("settings", "client_version", options.Version)
		}
	}
	showScreenLoading := func(target navigationState) error {
		screen := screens[target.screen]
		if screen == nil {
			return fmt.Errorf("screen %q is unavailable", target.screen)
		}
		data, loadErr := screenLoadingData(target, options.Version, renderer.ThemeName(), mode, endpoint, sshSettings)
		if loadErr != nil {
			return loadErr
		}
		if err := validateNativeBindings(screen, data); err != nil {
			return err
		}
		renderer.SetScreenAndData(screen, data)
		if target.screen == "settings" {
			applyConnectionBindings(renderer, "settings", mode, endpoint, sshSettings)
			renderer.SetRootBinding("settings", "client_version", options.Version)
		}
		return nil
	}
	showScreenData := func(target navigationState, data map[string]any) error {
		screen := screens[target.screen]
		if screen == nil {
			return fmt.Errorf("screen %q is unavailable", target.screen)
		}
		if err := validateNativeBindings(screen, data); err != nil {
			return err
		}
		renderer.SetScreenAndData(screen, data)
		if target.screen == "settings" {
			applyConnectionBindings(renderer, "settings", mode, endpoint, sshSettings)
			renderer.SetRootBinding("settings", "client_version", options.Version)
		}
		return nil
	}
	beginNavigationWith := func(target navigationState, recoverMissingRoute bool) error {
		navigation = target
		usedCache := false
		if cached, ok := screenCache.Get(target); ok {
			if err := showScreenData(target, cached); err != nil {
				return err
			}
			usedCache = true
		} else if err := showScreenLoading(target); err != nil {
			return err
		}
		if target.screen == "job-details" {
			startOutput(target.jobID)
		} else {
			stopOutput()
		}
		if recoverMissingRoute {
			startResyncLoad(target)
		} else {
			startScreenLoad(target)
		}
		if !usedCache {
			renderer.SetStatus(loadingScreenLabel(target.screen))
		} else if isScreenLoadingStatus(renderer.status) {
			renderer.SetStatus("")
		}
		return nil
	}
	beginNavigation := func(target navigationState) error { return beginNavigationWith(target, false) }
	beginResyncNavigation := func(target navigationState) error { return beginNavigationWith(target, true) }
	if client != nil {
		if err := beginResyncNavigation(navigation); err != nil {
			renderer.SetStatus("Could not load the initial screen: " + err.Error())
		}
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
	reconcileOperations := func() {
		if client == nil || operationJournal == nil {
			return
		}
		resumed, message, reconcileErr := operationJournal.reconcile(ctx, client, coordinator)
		if reconcileErr != nil {
			renderer.SetStatus("Could not reconcile earlier actions: " + reconcileErr.Error())
			return
		}
		if resumed > 0 {
			renderer.ShowNotice(fmt.Sprintf("Resumed %d interrupted action(s)", resumed), "", uidsl.Action{}, nil, presentation.TransientNoticeDuration)
		} else if message != "" {
			renderer.SetStatus(message)
		}
	}
	if initialConnectErr == nil {
		reconcileOperations()
	}

	var reconnectTimer *time.Timer
	var reconnect <-chan time.Time
	disconnect := func() {
		if session != nil {
			session.close()
			session = nil
		}
		client = nil
		clientBroker.Set(nil)
		changes = nil
		watchErrors = nil
		stopOutput()
		if screenLoadCancel != nil {
			screenLoadCancel()
			screenLoadCancel = nil
			screenLoadGeneration++
		}
	}
	pauseReconnect := func(status string) {
		disconnect()
		if reconnectTimer != nil {
			if !reconnectTimer.Stop() {
				select {
				case <-reconnectTimer.C:
				default:
				}
			}
		}
		reconnect = nil
		applyNativeConnectionState(renderer, nativeConnectionState{status: status})
		renderer.SetStatus("")
		window.Invalidate()
	}
	scheduleReconnectAfter := func(status string, delay time.Duration) {
		disconnect()
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
		if initialHostKeyPending {
			pauseReconnect("SSH host key verification required. Connection attempts are paused.")
		} else {
			scheduleReconnectAfter("Server unavailable: "+initialConnectErr.Error(), reconnectDelay)
		}
	}
	defer func() {
		if reconnectTimer != nil {
			reconnectTimer.Stop()
		}
	}()
	applyOperationOutcome := func(operation operations.Operation) {
		if operation.State != operations.StateSucceeded {
			message := strings.TrimSpace(operation.Message)
			if message == "" {
				message = "The action did not complete"
			}
			renderer.ShowAlert("Action failed", message)
			return
		}
		effect, ok := operation.Value.(nativeOperationEffect)
		if !ok {
			if operation.Message != "" {
				renderer.ShowNotice(operation.Message, "", uidsl.Action{}, nil, presentation.TransientNoticeDuration)
			}
			return
		}
		switch result := effect.Value.(type) {
		case *cnpv1.ServerUpdateCheckResult:
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
		case *cnpv1.ServerUpdateVersions:
			renderer.SetRootBinding("settings", "rollback_versions", versionOptions(result.Versions, "No lower versions available"))
			selected := ""
			if len(result.Versions) > 0 {
				selected = result.Versions[0]
			}
			renderer.SetRootBinding("settings", "selected_rollback_version", selected)
			renderer.SetRootBinding("settings", "rollback_result", fmt.Sprintf("Found %d rollback version(s)", len(result.Versions)))
			renderer.SetRootBinding("settings", "rollback_result_tone", "success")
		case *cnpv1.ServerUpdateActionResult:
			field := "update_result"
			if operation.Arguments["action"] == "rollback" {
				field = "rollback_result"
			}
			renderer.SetRootBinding("settings", field, effect.Message)
			renderer.SetRootBinding("settings", field+"_tone", "success")
		}
		if effect.CancelledJob != "" && navigation.screen == "job-details" && navigation.jobID == effect.CancelledJob {
			pendingCancellations[effect.CancelledJob] = true
			renderer.SetRootBinding("jobDetails", "can_cancel", false)
			screenCache.SetRootBinding(navigation, "jobDetails", "can_cancel", false)
		}
		if effect.NavigateRoute != "" && client != nil {
			next, parseErr := navigationForRoute(effect.NavigateRoute)
			if parseErr != nil {
				renderer.SetStatus(effect.Message + ", but navigation failed: " + parseErr.Error())
				return
			}
			if err := beginNavigation(next); err != nil {
				renderer.SetStatus(effect.Message + ", but navigation failed: " + err.Error())
				return
			}
		} else if effect.Refresh && client != nil {
			startResyncLoad(navigation)
		}
		if effect.Notice && effect.Message != "" {
			action := uidsl.Action{}
			arguments := map[string]string{}
			if effect.NoticeRoute != "" {
				action.Command = "navigate"
				arguments["route"] = effect.NoticeRoute
				if effect.NoticeSection != "" {
					arguments["section"] = effect.NoticeSection
				}
			}
			renderer.ShowNotice(effect.Message, effect.NoticeLabel, action, arguments, presentation.TransientNoticeDuration)
		}
	}
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-statusExpiry:
			if renderer.ClearExpiredStatus(now) {
				window.Invalidate()
			}
			statusExpiry = nil
			scheduleStatusExpiry()
		case <-reconnect:
			reconnect = nil
			connected, connectErr := connectConfiguredNativeSession(ctx, connectionSettings, options.Version)
			if connectErr != nil {
				hostKeyPending := captureSSHHostKeyError(&sshSettings, connectErr)
				connectionSettings.SSH = sshSettings
				if navigation.screen == "settings" {
					applyConnectionBindings(renderer, "settings", mode, endpoint, sshSettings)
				}
				if hostKeyPending {
					pauseReconnect("SSH host key verification required. Connection attempts are paused.")
					continue
				}
				reconnectDelay = nextReconnectDelay(reconnectDelay)
				scheduleReconnect(connectErr.Error())
				continue
			}
			session = connected
			client = connected.client
			clientBroker.Set(client)
			changes = connected.changes
			watchErrors = connected.watchErrors
			address = connected.address
			reconnectDelay = time.Second
			if screenCache.SetServerInstallationID(client.Welcome().GetServerInstallationId()) {
				if err := showScreenLoading(navigation); err != nil {
					renderer.SetStatus(err.Error())
				}
			}
			startScreenLoad(navigation)
			if mode != connectionModeSSH {
				rememberSuccessfulEndpoint(preferencesPath, address)
				preferences.LastSuccessfulEndpoint = address
			}
			if mode == connectionModeDiscover {
				connectionSettings.PreferredAddress = address
				connectionSettings.DiscoverFallback = true
			}
			applyNativeConnectionState(renderer, nativeConnectionState{connected: true, address: address, status: "Connected to " + address})
			if navigation.screen == "settings" {
				applyConnectionBindings(renderer, "settings", mode, endpoint, sshSettings)
				renderer.SetRootBinding("settings", "client_version", options.Version)
			}
			if navigation.screen == "job-details" {
				startOutput(navigation.jobID)
			}
			reconcileOperations()
			scheduleStatusExpiry()
			window.Invalidate()
		case change, ok := <-changes:
			if !ok {
				scheduleReconnect("")
				continue
			}
			if change.ResyncRequired || relevantScreenChange(navigation, change) {
				startScreenLoad(navigation)
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
		case result := <-screenLoads:
			if navigation != result.navigation || result.generation != screenLoadGeneration {
				continue
			}
			if result.err != nil {
				if result.recoverMissingRoute && navigation.screen != "front-page" {
					if err := beginResyncNavigation(navigationState{screen: "front-page"}); err != nil {
						renderer.SetStatus("Loading failed: " + result.err.Error())
					}
					window.Invalidate()
					continue
				}
				if result.recoverMissingRoute {
					scheduleReconnect("resynchronize: " + result.err.Error())
					continue
				}
				if screenCache.Has(navigation) {
					renderer.SetStatus("Refresh failed; showing last known data: " + result.err.Error())
				} else {
					renderer.SetStatus("Loading failed: " + result.err.Error())
				}
			} else {
				if err := validateNativeBindings(screens[navigation.screen], result.data); err != nil {
					renderer.SetStatus("Loading failed: " + err.Error())
					window.Invalidate()
					continue
				}
				if navigation.screen == "job-details" && pendingCancellations[navigation.jobID] {
					if root, ok := result.data["jobDetails"].(map[string]any); ok {
						if canCancel, _ := root["can_cancel"].(bool); canCancel {
							root["can_cancel"] = false
						} else {
							delete(pendingCancellations, navigation.jobID)
						}
					}
				}
				screenCache.Put(navigation, result.data)
				renderer.SetScreenAndData(screens[navigation.screen], result.data)
				if navigation.screen == "settings" {
					applyConnectionBindings(renderer, "settings", mode, endpoint, sshSettings)
					renderer.SetRootBinding("settings", "client_version", options.Version)
				}
				if navigation.screen == "job-details" {
					outputBuffer.apply(renderer)
				}
				if isScreenLoadingStatus(renderer.status) || strings.HasPrefix(renderer.status, "Refresh failed; showing last known data:") {
					renderer.SetStatus("")
				}
			}
			window.Invalidate()
		case <-coordinator.Changed():
			snapshot := coordinator.Snapshot()
			renderer.SetOperations(snapshot)
			for _, operation := range snapshot {
				if !operation.State.Terminal() {
					continue
				}
				applyOperationOutcome(operation)
				coordinator.Forget(operation.ID)
			}
			renderer.SetOperations(coordinator.Snapshot())
			scheduleStatusExpiry()
			window.Invalidate()
		case command := <-commands:
			switch command.action.Command {
			case "set-connection-field":
				switch strings.TrimSpace(command.arguments["field"]) {
				case "mode":
					candidate := strings.TrimSpace(command.arguments["value"])
					if candidate == connectionModeDiscover || candidate == connectionModeExplicit || candidate == connectionModeSSH {
						mode = candidate
					}
				case "endpoint":
					endpoint = command.arguments["value"]
				case "ssh-jump-address":
					sshSettings.JumpAddress = command.arguments["value"]
				case "ssh-username":
					sshSettings.Username = command.arguments["value"]
				case "ssh-destination":
					sshSettings.Destination = command.arguments["value"]
				}
				applyConnectionBindings(renderer, navigation.screen, mode, endpoint, sshSettings)
				window.Invalidate()
				continue
			case "generate-ssh-device-key":
				privateKey, publicKey, generateErr := cnpclient.GenerateSSHDeviceKey("ciwi-native-device")
				if generateErr == nil {
					generateErr = saveSSHDevicePrivateKey(preferencesPath, privateKey)
				}
				if generateErr != nil {
					renderer.SetStatus("SSH device key could not be generated: " + generateErr.Error())
					window.Invalidate()
					continue
				}
				sshSettings.PrivateKey = privateKey
				sshSettings.PublicKey = publicKey
				connectionSettings.SSH = sshSettings
				if saveErr := updateNativePreferences(preferencesPath, func(preferences *nativePreferences) {
					preferences.SSH.PublicKey = publicKey
				}); saveErr != nil {
					renderer.SetStatus("SSH public key preference could not be saved: " + saveErr.Error())
					window.Invalidate()
					continue
				}
				preferences.SSH.PublicKey = publicKey
				applyConnectionBindings(renderer, navigation.screen, mode, endpoint, sshSettings)
				renderer.SetStatus("Generated a device-specific SSH key. Add the restricted public key to the jump host.")
				window.Invalidate()
				continue
			case "trust-ssh-host-key":
				fingerprint := strings.TrimSpace(sshSettings.PendingFingerprint)
				if fingerprint == "" {
					renderer.SetStatus("Connect once to inspect the SSH host key")
					window.Invalidate()
					continue
				}
				sshSettings.HostKeyFingerprint = fingerprint
				sshSettings.PendingFingerprint = ""
				connectionSettings.SSH = sshSettings
				if saveErr := updateNativePreferences(preferencesPath, func(preferences *nativePreferences) {
					preferences.SSH.HostKeyFingerprint = fingerprint
				}); saveErr != nil {
					renderer.SetStatus("SSH host key trust could not be saved: " + saveErr.Error())
					window.Invalidate()
					continue
				}
				preferences.SSH.HostKeyFingerprint = fingerprint
				applyConnectionBindings(renderer, navigation.screen, mode, endpoint, sshSettings)
				reconnectDelay = time.Second
				scheduleReconnectAfter("SSH host key trusted; reconnecting…", 0)
				continue
			case "reject-ssh-host-key":
				if strings.TrimSpace(sshSettings.PendingFingerprint) == "" {
					renderer.SetStatus("No pending SSH host key to reject")
					window.Invalidate()
					continue
				}
				sshSettings.PendingFingerprint = ""
				connectionSettings.SSH = sshSettings
				applyConnectionBindings(renderer, navigation.screen, mode, endpoint, sshSettings)
				pauseReconnect("SSH host key rejected. Connection attempts are paused.")
				continue
			case "save-connection":
				mode = strings.TrimSpace(command.arguments["mode"])
				endpoint = strings.TrimSpace(command.arguments["endpoint"])
				if mode != connectionModeDiscover && mode != connectionModeExplicit && mode != connectionModeSSH {
					renderer.SetStatus("Select a connection mode")
					if navigation.screen == "connection" {
						renderer.SetRootBinding("connection", "status", "Select a connection mode")
						renderer.SetRootBinding("connection", "status_tone", "danger")
					}
					window.Invalidate()
					continue
				}
				connectionSettings = nativeConnectionSettings{Mode: mode, Endpoint: endpoint, SSH: sshSettings}
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
				} else if mode == connectionModeDiscover {
					connectionSettings.PreferredAddress = strings.TrimSpace(preferences.LastSuccessfulEndpoint)
					connectionSettings.DiscoverFallback = true
				} else {
					if strings.TrimSpace(sshSettings.JumpAddress) == "" || strings.TrimSpace(sshSettings.Username) == "" || strings.TrimSpace(sshSettings.Destination) == "" {
						renderer.SetStatus("Jump host, username, and destination are required for a remote server")
						window.Invalidate()
						continue
					}
					if len(sshSettings.PrivateKey) == 0 {
						renderer.SetStatus("Generate this device's SSH key before connecting")
						window.Invalidate()
						continue
					}
				}
				if saveErr := updateNativePreferences(preferencesPath, func(preferences *nativePreferences) {
					preferences.ConnectionMode = mode
					preferences.ServerEndpoint = endpoint
					preferences.SSH = sshPreferences{
						JumpAddress: strings.TrimSpace(sshSettings.JumpAddress), Username: strings.TrimSpace(sshSettings.Username),
						Destination: strings.TrimSpace(sshSettings.Destination), PublicKey: sshSettings.PublicKey,
						HostKeyFingerprint: sshSettings.HostKeyFingerprint,
					}
				}); saveErr != nil {
					renderer.SetStatus("Connection preference could not be saved: " + saveErr.Error())
					window.Invalidate()
					continue
				}
				preferences.ConnectionMode = mode
				preferences.ServerEndpoint = endpoint
				preferences.SSH = sshPreferences{
					JumpAddress: strings.TrimSpace(sshSettings.JumpAddress), Username: strings.TrimSpace(sshSettings.Username),
					Destination: strings.TrimSpace(sshSettings.Destination), PublicKey: sshSettings.PublicKey,
					HostKeyFingerprint: sshSettings.HostKeyFingerprint,
				}
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
				if section := strings.TrimSpace(command.arguments["section"]); section != "" {
					renderer.ScrollToSection(section)
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
					if err := refreshOfflineScreen(renderer, screens, navigation, options.Version, mode, endpoint, sshSettings); err != nil {
						renderer.SetStatus(err.Error())
					}
					applyNativeConnectionState(renderer, nativeConnectionState{connecting: reconnect != nil, status: "Server unavailable; reconnecting…"})
					window.Invalidate()
					continue
				}
				if err := beginNavigation(next); err != nil {
					renderer.SetStatus("Navigation failed: " + err.Error())
				}
				window.Invalidate()
				continue
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
				startScreenLoad(navigation)
				window.Invalidate()
				continue
			}
			previous := navigation
			handleCommand(renderer, &navigation, command, preferencesPath)
			if navigation.screen == "settings" {
				applyConnectionBindings(renderer, "settings", mode, endpoint, sshSettings)
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
	if mode != connectionModeExplicit && mode != connectionModeSSH {
		mode = connectionModeDiscover
	}
	return map[string]any{"connection": map[string]any{
		"mode": mode, "endpoint": endpoint, "explicit": mode == connectionModeExplicit,
		"ssh":    mode == connectionModeSSH,
		"status": status, "status_tone": connectionStatusTone(status), "can_back": canBack,
		"modes": []any{
			map[string]any{"value": connectionModeDiscover, "label": "Automatic discovery"},
			map[string]any{"value": connectionModeExplicit, "label": "Explicit endpoint"},
			map[string]any{"value": connectionModeSSH, "label": "Remote server (SSH)"},
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

func applyConnectionBindings(renderer *Renderer, screen, mode, endpoint string, sshSettings sshConnectionSettings) {
	explicit := mode == connectionModeExplicit
	remote := mode == connectionModeSSH
	values := map[string]any{
		"ssh": remote, "ssh_jump_address": sshSettings.JumpAddress, "ssh_username": sshSettings.Username,
		"ssh_destination": sshSettings.Destination, "ssh_public_key": sshSettings.PublicKey,
		"ssh_has_key":                 strings.TrimSpace(sshSettings.PublicKey) != "",
		"ssh_authorized_key":          cnpclient.RestrictedAuthorizedKey(sshSettings.PublicKey, sshSettings.Destination),
		"ssh_host_fingerprint":        sshSettings.HostKeyFingerprint,
		"ssh_has_trusted_fingerprint": strings.TrimSpace(sshSettings.HostKeyFingerprint) != "",
		"ssh_pending_fingerprint":     sshSettings.PendingFingerprint,
		"ssh_has_pending_fingerprint": strings.TrimSpace(sshSettings.PendingFingerprint) != "",
	}
	if screen == "settings" {
		renderer.SetRootBinding("settings", "connection_mode", mode)
		renderer.SetRootBinding("settings", "connection_endpoint", endpoint)
		renderer.SetRootBinding("settings", "connection_explicit", explicit)
		for key, value := range values {
			renderer.SetRootBinding("settings", key, value)
		}
		return
	}
	renderer.SetRootBinding("connection", "mode", mode)
	renderer.SetRootBinding("connection", "endpoint", endpoint)
	renderer.SetRootBinding("connection", "explicit", explicit)
	for key, value := range values {
		renderer.SetRootBinding("connection", key, value)
	}
}

func captureSSHHostKeyError(settings *sshConnectionSettings, err error) bool {
	var hostKeyErr *cnpclient.SSHHostKeyError
	if settings == nil || !errors.As(err, &hostKeyErr) {
		return false
	}
	settings.PendingFingerprint = hostKeyErr.Fingerprint
	return true
}

func handleCommand(renderer *Renderer, navigation *navigationState, command commandRequest, preferencesPath string) {
	switch command.action.Command {
	case "set-project-structure-filter":
		filter := strings.TrimSpace(command.arguments["value"])
		if filter == "" || !renderer.SetProjectStructureFilter(filter) {
			renderer.SetStatus("Project structure filter is unavailable")
			return
		}
		renderer.SetStatus("")
	case "set-agent-script-field":
		field := strings.TrimSpace(command.arguments["field"])
		value := command.arguments["value"]
		switch field {
		case "shell":
			navigation.scriptShell = strings.TrimSpace(value)
			navigation.script = presentation.ExampleAgentScript(navigation.scriptShell)
			renderer.SetRootBinding("agentScript", "selected_shell", navigation.scriptShell)
			renderer.SetRootBinding("agentScript", "script", navigation.script)
		case "script":
			navigation.script = value
			renderer.SetRootBinding("agentScript", "script", value)
		default:
			renderer.SetStatus("Unknown agent script field")
		}
	case "set-project-import-field":
		binding := map[string]string{"repoUrl": "import_repo_url", "repoRef": "import_repo_ref", "configFile": "import_config_file"}[strings.TrimSpace(command.arguments["field"])]
		if binding == "" {
			renderer.SetStatus("Unknown project import field")
			return
		}
		renderer.SetRootBinding("settings", binding, command.arguments["value"])
	case "set-managed-yaml-field":
		if strings.TrimSpace(command.arguments["field"]) != "yaml" {
			renderer.SetStatus("Unknown managed YAML field")
			return
		}
		renderer.SetRootBinding("managedYAML", "yaml", command.arguments["value"])
	case "set-vault-field":
		field := strings.TrimSpace(command.arguments["field"])
		if field != "name" && field != "url" && field != "role_id" && field != "approle_mount" && field != "secret_id_env" {
			renderer.SetStatus("Unknown Vault connection field")
			return
		}
		renderer.SetRootBinding("vault", field, command.arguments["value"])
	case "set-server-update-option":
		binding := map[string]string{
			"update": "selected_update_version", "rollback": "selected_rollback_version",
		}[strings.TrimSpace(command.arguments["field"])]
		if binding == "" {
			renderer.SetStatus("Unknown server update option")
			return
		}
		renderer.SetRootBinding("settings", binding, strings.TrimSpace(command.arguments["value"]))
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
		renderer.SetStatus("")
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

func navigationForRoute(route string) (navigationState, error) {
	route = strings.TrimSpace(route)
	routes, err := sharedUI.LoadRoutes()
	if err != nil {
		return navigationState{}, err
	}
	match, ok := routes.Match(route, "gio")
	if !ok {
		return navigationState{}, fmt.Errorf("unsupported route %q", route)
	}
	next := navigationState{screen: match.Route.Screen}
	parsePositiveID := func(name string) (int64, error) {
		value, parseErr := strconv.ParseInt(strings.TrimSpace(match.Params[name]), 10, 64)
		if parseErr != nil || value <= 0 {
			return 0, fmt.Errorf("invalid %s in route %q", name, route)
		}
		return value, nil
	}
	switch match.Route.Name {
	case "project-details", "managed-yaml":
		next.projectID, err = parsePositiveID("projectId")
	case "job-details":
		next.jobID = strings.TrimSpace(match.Params["jobId"])
	case "pipeline-run-options":
		next.projectID, err = parsePositiveID("projectId")
		if err == nil {
			next.pipelineDBID, err = parsePositiveID("pipelineId")
		}
	case "legacy-pipeline-run-options":
		next.pipelineDBID, err = parsePositiveID("pipelineId")
	case "chain-run-options":
		next.projectID, err = parsePositiveID("projectId")
		next.chainID = strings.TrimSpace(match.Params["chainId"])
	case "agent-script":
		next.agentScriptID = strings.TrimSpace(match.Params["agentId"])
	case "agent-details":
		next.agentDetailsID = strings.TrimSpace(match.Params["agentId"])
	}
	if err != nil || next.jobID == "" && match.Route.Name == "job-details" || next.chainID == "" && match.Route.Name == "chain-run-options" || next.agentScriptID == "" && match.Route.Name == "agent-script" || next.agentDetailsID == "" && match.Route.Name == "agent-details" {
		if err != nil {
			return navigationState{}, err
		}
		return navigationState{}, fmt.Errorf("invalid route %q", route)
	}
	return next, nil
}

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
		"result": "", "result_tone": "muted",
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
	switch navigation.screen {
	case "front-page":
		return offlineFrontPageBindingData(clientVersion)
	case "project-details":
		return projectDetailsBindingData(&cnpv1.ProjectDetailsView{
			Project: &cnpv1.ProjectSummary{Id: navigation.projectID},
		})
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

func loadingScreenLabel(screen string) string {
	if screen == "run-options" {
		return "Loading run options…"
	}
	return "Loading…"
}

func isScreenLoadingStatus(status string) bool {
	switch strings.TrimSpace(status) {
	case "Loading…", "Loading run options…", "Refreshing eligible agents…":
		return true
	default:
		return false
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
		sourceKind := strings.TrimSpace(fmt.Sprint(project["source_kind"]))
		managed := sourceKind == "managed_yaml"
		project["is_managed"] = managed
		project["can_reload"] = !managed
		repoURL := strings.TrimSpace(fmt.Sprint(project["repo_url"]))
		project["has_repo"] = repoURL != ""
		ref := strings.TrimSpace(fmt.Sprint(project["repo_ref"]))
		if ref == "" {
			ref = "default"
		}
		project["repo_ref_label"] = ref
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
		if managed {
			project["source_label"] = "Managed YAML stored in ciwi"
			continue
		}
		label := repoURL
		if ref != "default" {
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
	root["loading"] = false
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
		project["pipeline_count_label"] = presentation.PipelineCountLabel(len(pipelines))
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
		card := domain.ExecutionCard{Summary: domain.ExecutionSummary{
			TotalJobs: int(numberValue(summary["total_jobs"])), Succeeded: int(numberValue(summary["succeeded"])),
			Failed: int(numberValue(summary["failed"])), InProgress: int(numberValue(summary["in_progress"])), Waiting: int(numberValue(summary["waiting"])),
		}}
		if ids, ok := entry["job_execution_ids"].([]any); ok {
			parts := make([]string, 0, len(ids))
			for _, id := range ids {
				parts = append(parts, fmt.Sprint(id))
			}
			card.JobExecutionIDs = parts
		}
		display := presentation.PresentExecutionCard(card, queued)
		entry["status"] = display.Status
		entry["summary_tone"] = display.SummaryTone
		entry["summary_label"] = display.SummaryLabel
		entry["job_execution_ids_csv"] = display.JobExecutionIDsCSV
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
	display := presentation.PresentExecutionCardJob(domain.ExecutionCardJob{
		Status: strings.TrimSpace(fmt.Sprint(job["status"])), CreatedUTC: parseExecutionCardTime(job["created_utc"]),
		StartedUTC: parseExecutionCardTime(job["started_utc"]), FinishedUTC: parseExecutionCardTime(job["finished_utc"]),
	}, time.Now())
	job["created_label"] = display.CreatedLabel
	job["duration_label"] = display.DurationLabel
}

func parseExecutionCardTime(value any) time.Time {
	parsed, _ := time.Parse(time.RFC3339Nano, strings.TrimSpace(fmt.Sprint(value)))
	return parsed
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
		if strings.TrimSpace(fmt.Sprint(project["project_icon"])) == "" {
			project["project_icon"] = root["project_icon"]
			project["project_icon_content_type"] = root["project_icon_content_type"]
		}
		project["source_metadata"] = presentation.ProjectSourceMetadata(fmt.Sprint(project["repo_ref"]), fmt.Sprint(project["config_file"]))
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
		dependencies := strings.TrimSpace(fmt.Sprint(pipeline["dependencies"]))
		pipeline["summary_label"] = presentation.PipelineSummaryLabel(jobsCount, dependencies)
		dependsOn, _ := pipeline["depends_on"].([]any)
		pipeline["graph_summary_label"] = presentation.PipelineGraphSummaryLabel(jobsCount, len(dependsOn))
		jobs, _ := pipeline["jobs"].([]any)
		for _, rawJob := range jobs {
			job, jobOK := rawJob.(map[string]any)
			if !jobOK {
				continue
			}
			stepsCount := int(numberValue(job["steps_count"]))
			runsOn := presentation.DeclarativeDefaultLabel(fmt.Sprint(job["runs_on_label"]), "unspecified")
			job["needs_label"] = presentation.DeclarativeDefaultLabel(fmt.Sprint(job["needs_label"]), "none")
			job["tools_label"] = presentation.DeclarativeDefaultLabel(fmt.Sprint(job["tools_label"]), "none")
			job["summary_label"] = presentation.ProjectJobSummaryLabel(stepsCount, runsOn)
			job["timeout_label"] = presentation.ProjectJobTimeoutLabel(int(numberValue(job["timeout_seconds"])))
			matrixCount := int(numberValue(job["matrix_count"]))
			job["matrix_label"] = presentation.ProjectJobMatrixLabel(matrixCount)
			steps, _ := job["steps"].([]any)
			for _, rawStep := range steps {
				step, stepOK := rawStep.(map[string]any)
				if !stepOK {
					continue
				}
				step["environment_label"] = presentation.ProjectStepEnvironmentLabel(stringListValue(step["environment"]))
				step["command"] = presentation.ProjectStepCommand(fmt.Sprint(step["command"]))
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
				root[key] = map[string]any{"empty_label": emptyLabel, "summary": "", "tone": "muted", "rows": []any{}, "additional_label": ""}
			}
		}
		if root["run_context"] == nil {
			root["run_context"] = map[string]any{"available": false, "scope_label": "", "pipelines": []any{}}
		}
		root["output"] = ""
		root["system_output"] = ""
		root["output_search"] = ""
		root["output_search_count"] = "0/0"
		root["tailing_label"] = "Tailing: On"
		root["tailing_tone"] = "success"
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
	}}, nil
}

func offlineFrontPageBindingData(clientVersion string) (map[string]any, error) {
	data, err := frontPageBindingData(&cnpv1.FrontPageView{
		Server: &cnpv1.ServerInfo{Version: strings.TrimSpace(clientVersion)},
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

func refreshOfflineScreen(renderer *Renderer, screens map[string]*uidsl.ScreenDocument, navigation navigationState, clientVersion, mode, endpoint string, sshSettings sshConnectionSettings) error {
	switch navigation.screen {
	case "front-page":
		data, err := offlineFrontPageBindingData(clientVersion)
		if err != nil {
			return err
		}
		renderer.SetScreenAndData(screens["front-page"], data)
	case "settings":
		data, err := offlineSettingsBindingData(clientVersion, renderer.ThemeName(), mode, endpoint, sshSettings)
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
