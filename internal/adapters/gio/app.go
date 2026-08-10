//go:build darwin || ios || linux || windows

package gio

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"strconv"
	"strings"
	"time"

	"gioui.org/app"
	"gioui.org/op"
	"gioui.org/x/explorer"
	"github.com/izzyreal/ciwi/internal/presentation"
	"github.com/izzyreal/ciwi/internal/presentation/operations"
	cnpv1 "github.com/izzyreal/ciwi/pkg/cnp/v1"
	"github.com/izzyreal/ciwi/pkg/cnpclient"
	"github.com/izzyreal/ciwi/pkg/uidsl"
	sharedUI "github.com/izzyreal/ciwi/ui"
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
	jobID    string
	events   []*cnpv1.JobOutputEvent
	omitted  map[string]bool
	bytes    int
	snapshot jobOutputSnapshot
	dirty    bool
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
	b.bytes = 0
	b.snapshot = jobOutputSnapshot{Outputs: map[string]string{}, Errors: map[string]string{}, ExitCodes: map[string]string{}}
	b.dirty = true
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
		switch eventCopy.Type {
		case "system-message":
			b.snapshot.System += eventCopy.Text
		case "output":
			b.snapshot.Outputs[eventCopy.ItemId] += eventCopy.Text
		case "finished":
			if eventCopy.Error != "" {
				b.snapshot.Errors[eventCopy.ItemId] = eventCopy.Error
			}
			if eventCopy.ExitCode != "" {
				b.snapshot.ExitCodes[eventCopy.ItemId] = eventCopy.ExitCode
			}
		}
		if eventCopy.Text != "" {
			b.events = append(b.events, eventCopy)
			b.bytes += len(eventCopy.Text)
		}
		b.dirty = true
	}
	for b.bytes > maxNativeOutputBytes && len(b.events) > 0 {
		removed := b.events[0]
		b.events = b.events[1:]
		b.bytes -= len(removed.Text)
		b.omitted[removed.ItemId] = true
		if removed.Type == "system-message" {
			b.snapshot.System = strings.TrimPrefix(b.snapshot.System, removed.Text)
		} else if removed.Type == "output" {
			b.snapshot.Outputs[removed.ItemId] = strings.TrimPrefix(b.snapshot.Outputs[removed.ItemId], removed.Text)
		}
	}
}

func (b *jobOutputBuffer) apply(renderer *Renderer) {
	if !b.dirty {
		return
	}
	snapshot := jobOutputSnapshot{
		System:  b.snapshot.System,
		Outputs: maps.Clone(b.snapshot.Outputs), Errors: maps.Clone(b.snapshot.Errors), ExitCodes: maps.Clone(b.snapshot.ExitCodes),
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
	b.dirty = false
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
	fileExplorer := explorer.NewExplorer(window)
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
	renderer.SetActionCatalog(actionCatalog)
	initialData, err := offlineFrontPageBindingData()
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
			renderer.ShowNotice("Disclosure state could not be saved: "+err.Error(), "", uidsl.Action{}, nil, presentation.TransientNoticeDuration)
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
			renderer.ShowNotice("View preference could not be saved: "+err.Error(), "", uidsl.Action{}, nil, presentation.TransientNoticeDuration)
		}
	})
	renderer.SetInvalidate(window.Invalidate)
	go runController(ctx, window, renderer, commands, screens, actionCatalog, options, preferencesPath, preferences, coordinator, clientBroker, operationJournal, fileExplorer)

	var operations op.Ops
	for {
		event := window.Event()
		fileExplorer.ListenEvents(event)
		switch event := event.(type) {
		case app.DestroyEvent:
			return event.Err
		case app.FrameEvent:
			gtx := app.NewContext(&operations, event)
			renderer.Layout(gtx)
			event.Frame(&operations)
		}
	}
}

func runController(ctx context.Context, window *app.Window, renderer *Renderer, commands <-chan commandRequest, screens map[string]*uidsl.ScreenDocument, actionCatalog *uidsl.ActionCatalogDocument, options Options, preferencesPath string, preferences nativePreferences, coordinator *operations.Coordinator, clientBroker *nativeClientBroker, operationJournal *nativeOperationJournal, artifactPicker artifactDestinationPicker) {
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
	var outputApplyTimer *time.Timer
	var outputApply <-chan time.Time
	terminalOutputRefreshedJobID := ""
	stopOutputApplyTimer := func() {
		if outputApplyTimer != nil {
			outputApplyTimer.Stop()
		}
		outputApplyTimer = nil
		outputApply = nil
	}
	stopOutput := func() {
		if outputCancel != nil {
			outputCancel()
		}
		outputCancel = nil
		outputBatches = nil
		outputErrors = nil
		stopOutputApplyTimer()
	}
	startOutput := func(jobID string) {
		stopOutput()
		if outputBuffer.jobID != jobID {
			terminalOutputRefreshedJobID = ""
		}
		outputBuffer.reset(jobID)
		outputBuffer.apply(renderer)
		if client == nil {
			return
		}
		streamCtx, cancelStream := context.WithCancel(ctx)
		batches, errorsOut, streamErr := client.WatchJobOutput(streamCtx, jobID, 0)
		if streamErr != nil {
			cancelStream()
			renderer.ShowNotice("Output stream unavailable: "+streamErr.Error(), "", uidsl.Action{}, nil, presentation.TransientNoticeDuration)
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
	artifactDownloads := make(chan artifactDownloadResult, 4)
	var screenLoadCancel context.CancelFunc
	var screenLoadGeneration uint64
	var screenRefreshes screenRefreshCoordinator
	launchScreenLoad := func(target navigationState, recoverMissingRoute bool) {
		if screenLoadCancel != nil {
			// launchScreenLoad is only called after the previous load completed or
			// after its cancellation was explicitly requested by navigation.
			screenLoadCancel = nil
		}
		loadCtx, cancelLoad := context.WithCancel(ctx)
		screenLoadCancel = cancelLoad
		screenLoadGeneration++
		generation := screenLoadGeneration
		activeClient := client
		themeName := renderer.ThemeName()
		go func() {
			defer cancelLoad()
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
	queueScreenLoad := func(target navigationState, recoverMissingRoute bool) {
		if screenLoadCancel != nil {
			screenLoadCancel()
			screenLoadCancel = nil
		}
		screenRefreshes.supersede()
		launchScreenLoad(target, recoverMissingRoute)
	}
	requestScreenRefresh := func(target navigationState, recoverMissingRoute bool) {
		if screenRefreshes.request(recoverMissingRoute) {
			launchScreenLoad(target, recoverMissingRoute)
		}
	}
	startScreenLoad := func(target navigationState) { queueScreenLoad(target, false) }
	startResyncLoad := func(target navigationState) { queueScreenLoad(target, true) }
	requestScreenLoad := func(target navigationState) { requestScreenRefresh(target, false) }
	requestResyncLoad := func(target navigationState) { requestScreenRefresh(target, true) }
	var pendingNavigation *navigationState
	defer func() {
		if screenLoadCancel != nil {
			screenLoadCancel()
		}
	}()
	navigation := navigationState{screen: "front-page"}
	if strings.TrimSpace(options.Route) != "" {
		navigation, _ = navigationForRoute(options.Route)
	}
	navigationHistory := []navigationState{}
	if initialConnectErr != nil {
		if navigation.screen != "front-page" && navigation.screen != "settings" {
			navigation = navigationState{screen: "front-page"}
		}
		if err := refreshOfflineScreen(renderer, screens, navigation, options.Version, mode, endpoint, sshSettings); err != nil {
			renderer.ShowAlert("Screen unavailable", err.Error())
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
		if target.screen == "project-details" {
			if frontPage, ok := screenCache.Get(navigationState{screen: "front-page"}); ok {
				seedProjectDetailsLoadingData(data, frontPage, target.projectID)
			}
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
		if cached, ok := screenCache.Get(target); ok {
			navigation = target
			pending := target
			pendingNavigation = &pending
			if err := showScreenData(target, cached); err != nil {
				return err
			}
			if target.screen == "job-details" {
				startOutput(target.jobID)
			} else {
				stopOutput()
			}
		} else if target.screen != "connection" {
			navigation = target
			pending := target
			pendingNavigation = &pending
			if err := showScreenLoading(target); err != nil {
				return err
			}
			stopOutput()
		} else {
			pending := target
			pendingNavigation = &pending
		}
		if recoverMissingRoute {
			startResyncLoad(target)
		} else {
			startScreenLoad(target)
		}
		return nil
	}
	beginNavigation := func(target navigationState) error { return beginNavigationWith(target, false) }
	beginResyncNavigation := func(target navigationState) error { return beginNavigationWith(target, true) }
	beginForwardNavigation := func(target navigationState) error {
		previous := navigation
		if err := beginNavigation(target); err != nil {
			return err
		}
		if previous != target {
			navigationHistory = append(navigationHistory, previous)
		}
		return nil
	}
	beginBackNavigation := func(fallbackRoute string) error {
		target, popHistory, err := nativeBackNavigationTarget(navigationHistory, fallbackRoute)
		if err != nil {
			return err
		}
		if err := beginNavigation(target); err != nil {
			return err
		}
		if popHistory {
			navigationHistory = navigationHistory[:len(navigationHistory)-1]
		}
		return nil
	}
	if client != nil {
		if err := beginResyncNavigation(navigation); err != nil {
			renderer.ShowAlert("Could not load the initial screen", err.Error())
		}
	}
	window.Invalidate()
	var noticeTimer *time.Timer
	var noticeExpiry <-chan time.Time
	scheduleNoticeExpiry := func() {
		expires := renderer.NoticeExpiry()
		if expires.IsZero() {
			if noticeTimer != nil {
				noticeTimer.Stop()
			}
			noticeExpiry = nil
			return
		}
		delay := time.Until(expires)
		if delay < 0 {
			delay = 0
		}
		if noticeTimer == nil {
			noticeTimer = time.NewTimer(delay)
		} else {
			if !noticeTimer.Stop() {
				select {
				case <-noticeTimer.C:
				default:
				}
			}
			noticeTimer.Reset(delay)
		}
		noticeExpiry = noticeTimer.C
	}
	scheduleNoticeExpiry()
	defer func() {
		if noticeTimer != nil {
			noticeTimer.Stop()
		}
	}()
	reconcileOperations := func() {
		if client == nil || operationJournal == nil {
			return
		}
		resumed, message, reconcileErr := operationJournal.reconcile(ctx, client, coordinator)
		if reconcileErr != nil {
			renderer.ShowNotice("Could not reconcile earlier actions: "+reconcileErr.Error(), "", uidsl.Action{}, nil, presentation.TransientNoticeDuration)
			return
		}
		if resumed > 0 {
			renderer.ShowNotice(fmt.Sprintf("Resumed %d interrupted action(s)", resumed), "", uidsl.Action{}, nil, presentation.TransientNoticeDuration)
		} else if message != "" {
			renderer.ShowNotice(message, "", uidsl.Action{}, nil, presentation.TransientNoticeDuration)
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
		screenRefreshes.cancel()
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
		window.Invalidate()
	}
	scheduleReconnectAfter := func(status string, delay time.Duration) {
		disconnect()
		applyNativeConnectionState(renderer, nativeConnectionState{connecting: true, status: status})
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
		navigated := false
		if effect.NavigateBack && client != nil && nativeRunOptionsOperationMatches(navigation, operation) {
			if err := beginBackNavigation(operation.Arguments["fallbackRoute"]); err != nil {
				renderer.ShowAlert("Navigation failed", effect.Message+", but navigation failed: "+err.Error())
				return
			}
			navigated = true
		} else if effect.NavigateRoute != "" && client != nil {
			next, parseErr := navigationForRoute(effect.NavigateRoute)
			if parseErr != nil {
				renderer.ShowAlert("Navigation failed", effect.Message+", but navigation failed: "+parseErr.Error())
				return
			}
			if err := beginForwardNavigation(next); err != nil {
				renderer.ShowAlert("Navigation failed", effect.Message+", but navigation failed: "+err.Error())
				return
			}
			navigated = true
		}
		if !navigated && client != nil && shouldRefreshAfterNativeOperation(actionCatalog, operation, effect) {
			requestResyncLoad(navigation)
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
		case now := <-noticeExpiry:
			if renderer.ClearExpiredNotice(now) {
				window.Invalidate()
			}
			noticeExpiry = nil
			scheduleNoticeExpiry()
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
					renderer.ShowAlert("Screen unavailable", err.Error())
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
			scheduleNoticeExpiry()
			window.Invalidate()
		case change, ok := <-changes:
			if !ok {
				scheduleReconnect("")
				continue
			}
			if change.ResyncRequired || relevantScreenChange(screens[navigation.screen], navigation, change) {
				requestScreenLoad(navigation)
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
			if batch.GetTerminal() && !batch.GetHasMore() {
				stopOutputApplyTimer()
				outputBuffer.apply(renderer)
				if navigation.screen == "job-details" && navigation.jobID == batch.GetJobExecutionId() && terminalOutputRefreshedJobID != navigation.jobID {
					terminalOutputRefreshedJobID = navigation.jobID
					requestScreenLoad(navigation)
				}
				window.Invalidate()
			} else if outputApplyTimer == nil {
				outputApplyTimer = time.NewTimer(33 * time.Millisecond)
				outputApply = outputApplyTimer.C
			}
		case <-outputApply:
			outputApplyTimer = nil
			outputApply = nil
			outputBuffer.apply(renderer)
			window.Invalidate()
		case outputErr, ok := <-outputErrors:
			if !ok {
				outputErrors = nil
				continue
			}
			if outputErr != nil {
				renderer.ShowNotice("Output stream stopped: "+outputErr.Error(), "", uidsl.Action{}, nil, presentation.TransientNoticeDuration)
				window.Invalidate()
			}
			outputErrors = nil
		case result := <-screenLoads:
			expectedNavigation := navigation
			if pendingNavigation != nil {
				expectedNavigation = *pendingNavigation
			}
			if expectedNavigation != result.navigation || result.generation != screenLoadGeneration {
				continue
			}
			screenLoadCancel = nil
			if startTrailing, recoverMissingRoute := screenRefreshes.complete(); startTrailing && client != nil {
				launchScreenLoad(result.navigation, recoverMissingRoute)
			}
			if result.err != nil {
				if result.recoverMissingRoute && result.navigation.screen != "front-page" {
					if err := beginResyncNavigation(navigationState{screen: "front-page"}); err != nil {
						renderer.ShowAlert("Loading failed", result.err.Error())
					}
					window.Invalidate()
					continue
				}
				if result.recoverMissingRoute {
					pendingNavigation = nil
					scheduleReconnect("resynchronize: " + result.err.Error())
					continue
				}
				if navigation == result.navigation && pendingNavigation != nil {
					pendingNavigation = nil
					rootName := screenBindingRoot(result.navigation.screen)
					ready := screenCache.Has(result.navigation)
					renderer.SetRootBinding(rootName, "loading", false)
					renderer.SetRootBinding(rootName, "ready", ready)
					renderer.SetRootBinding(rootName, "load_error", result.err.Error())
					if ready {
						screenCache.SetRootBinding(result.navigation, rootName, "load_error", result.err.Error())
					}
					window.Invalidate()
					continue
				}
				if pendingNavigation != nil {
					pendingNavigation = nil
					renderer.ShowAlert("Loading failed", "Showing the previous screen: "+result.err.Error())
				} else if screenCache.Has(navigation) {
					renderer.ShowNotice("Refresh failed; showing last known data: "+result.err.Error(), "", uidsl.Action{}, nil, presentation.TransientNoticeDuration)
				} else {
					renderer.ShowAlert("Loading failed", result.err.Error())
				}
			} else {
				if err := validateNativeBindings(screens[result.navigation.screen], result.data); err != nil {
					if navigation == result.navigation && pendingNavigation != nil {
						pendingNavigation = nil
						rootName := screenBindingRoot(result.navigation.screen)
						ready := screenCache.Has(result.navigation)
						renderer.SetRootBinding(rootName, "loading", false)
						renderer.SetRootBinding(rootName, "ready", ready)
						renderer.SetRootBinding(rootName, "load_error", err.Error())
						if ready {
							screenCache.SetRootBinding(result.navigation, rootName, "load_error", err.Error())
						}
						window.Invalidate()
						continue
					}
					if pendingNavigation != nil {
						pendingNavigation = nil
						renderer.ShowAlert("Loading failed", "Showing the previous screen: "+err.Error())
					} else {
						renderer.ShowAlert("Loading failed", err.Error())
					}
					window.Invalidate()
					continue
				}
				if result.navigation.screen == "job-details" && pendingCancellations[result.navigation.jobID] {
					if root, ok := result.data["jobDetails"].(map[string]any); ok {
						if canCancel, _ := root["can_cancel"].(bool); canCancel {
							root["can_cancel"] = false
						} else {
							delete(pendingCancellations, result.navigation.jobID)
						}
					}
				}
				preserveOutput := navigation.screen == "job-details" && result.navigation.screen == "job-details" &&
					navigation.jobID == result.navigation.jobID && outputBuffer.jobID == result.navigation.jobID
				navigation = result.navigation
				pendingNavigation = nil
				screenCache.Put(navigation, result.data)
				renderer.SetScreenAndData(screens[navigation.screen], result.data)
				if navigation.screen == "job-details" {
					if !preserveOutput {
						startOutput(navigation.jobID)
					}
				} else {
					stopOutput()
				}
				if navigation.screen == "settings" {
					applyConnectionBindings(renderer, "settings", mode, endpoint, sshSettings)
					renderer.SetRootBinding("settings", "client_version", options.Version)
				}
				if navigation.screen == "job-details" {
					outputBuffer.dirty = true
					outputBuffer.apply(renderer)
				}
			}
			window.Invalidate()
		case result := <-artifactDownloads:
			label := strings.TrimSpace(result.label)
			if label == "" {
				label = "Artifact"
			}
			if errors.Is(result.err, errArtifactDownloadCancelled) {
				renderer.ShowNotice(label+" download cancelled", "", uidsl.Action{}, nil, presentation.TransientNoticeDuration)
			} else if result.err != nil {
				renderer.ShowNotice(label+" download failed: "+result.err.Error(), "", uidsl.Action{}, nil, presentation.TransientNoticeDuration)
			} else {
				renderer.ShowNotice("Downloaded "+strings.ToLower(label)+": "+result.path, "", uidsl.Action{}, nil, presentation.TransientNoticeDuration)
			}
			scheduleNoticeExpiry()
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
			scheduleNoticeExpiry()
			window.Invalidate()
		case command := <-commands:
			switch command.action.Command {
			case "refresh":
				if client == nil {
					renderer.ShowAlert("Server offline", "Reconnect is in progress.")
					window.Invalidate()
					continue
				}
				if navigation.screen == "project-details" {
					renderer.SetRootBinding("projectDetails", "loading", true)
					renderer.SetRootBinding("projectDetails", "ready", false)
					renderer.SetRootBinding("projectDetails", "load_error", "")
					pending := navigation
					pendingNavigation = &pending
				}
				requestScreenLoad(navigation)
				window.Invalidate()
				continue
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
					renderer.ShowAlert("SSH key generation failed", generateErr.Error())
					window.Invalidate()
					continue
				}
				sshSettings.PrivateKey = privateKey
				sshSettings.PublicKey = publicKey
				connectionSettings.SSH = sshSettings
				if saveErr := updateNativePreferences(preferencesPath, func(preferences *nativePreferences) {
					preferences.SSH.PublicKey = publicKey
				}); saveErr != nil {
					renderer.ShowAlert("SSH key preference could not be saved", saveErr.Error())
					window.Invalidate()
					continue
				}
				preferences.SSH.PublicKey = publicKey
				applyConnectionBindings(renderer, navigation.screen, mode, endpoint, sshSettings)
				renderer.ShowNotice("Generated a device-specific SSH key. Add the restricted public key to the jump host.", "", uidsl.Action{}, nil, presentation.TransientNoticeDuration)
				window.Invalidate()
				continue
			case "trust-ssh-host-key":
				fingerprint := strings.TrimSpace(sshSettings.PendingFingerprint)
				if fingerprint == "" {
					renderer.ShowAlert("SSH host key unavailable", "Connect once to inspect the SSH host key.")
					window.Invalidate()
					continue
				}
				sshSettings.HostKeyFingerprint = fingerprint
				sshSettings.PendingFingerprint = ""
				connectionSettings.SSH = sshSettings
				if saveErr := updateNativePreferences(preferencesPath, func(preferences *nativePreferences) {
					preferences.SSH.HostKeyFingerprint = fingerprint
				}); saveErr != nil {
					renderer.ShowAlert("SSH host key trust could not be saved", saveErr.Error())
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
					renderer.ShowAlert("No pending SSH host key", "Connect first to inspect a host key.")
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
					renderer.ShowAlert("Connection mode required", "Select a connection mode.")
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
						renderer.ShowAlert("Invalid native endpoint", parseErr.Error())
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
						renderer.ShowAlert("Remote server details required", "Jump host, username, and destination are required.")
						window.Invalidate()
						continue
					}
					if len(sshSettings.PrivateKey) == 0 {
						renderer.ShowAlert("SSH device key required", "Generate this device's SSH key before connecting.")
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
					renderer.ShowAlert("Connection preference could not be saved", saveErr.Error())
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
			case "set-report-filter":
				renderer.SetNestedBinding("jobDetails", "test_report", "filter", strings.TrimSpace(command.arguments["value"]))
				window.Invalidate()
				continue
			case "download-artifact":
				activeClient := client
				arguments := command.arguments
				go func() {
					path, downloadErr := downloadArtifact(ctx, activeClient, artifactPicker, arguments)
					select {
					case artifactDownloads <- artifactDownloadResult{path: path, label: "Artifact", err: downloadErr}:
					case <-ctx.Done():
					}
				}()
				window.Invalidate()
				continue
			case "download-job-log":
				activeClient := client
				format := strings.ToLower(strings.TrimSpace(command.arguments["format"]))
				if format != "clean" && format != "raw" {
					renderer.ShowAlert("Log download unavailable", "Log format must be clean or raw.")
					window.Invalidate()
					continue
				}
				arguments := map[string]string{
					"jobExecutionId": command.arguments["jobExecutionId"],
					"kind":           "log-" + format,
				}
				label := strings.ToUpper(format[:1]) + format[1:] + " log"
				go func() {
					path, downloadErr := downloadArtifact(ctx, activeClient, artifactPicker, arguments)
					select {
					case artifactDownloads <- artifactDownloadResult{path: path, label: label, err: downloadErr}:
					case <-ctx.Done():
					}
				}()
				window.Invalidate()
				continue
			}
			if command.action.Command == "open-url" {
				if openErr := openExternalURL(command.arguments["url"]); openErr != nil {
					renderer.ShowAlert("Could not open link", openErr.Error())
				}
				window.Invalidate()
				continue
			}
			if command.action.Command == "navigate-back" {
				if client == nil {
					renderer.ShowAlert("Server connection required", "This screen needs a server connection.")
					window.Invalidate()
					continue
				}
				if err := beginBackNavigation(command.arguments["fallbackRoute"]); err != nil {
					renderer.ShowAlert("Navigation failed", err.Error())
				}
				window.Invalidate()
				continue
			}
			if command.action.Command == "navigate" {
				next, parseErr := navigationForRoute(command.arguments["route"])
				if parseErr != nil {
					renderer.ShowAlert("Navigation failed", parseErr.Error())
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
						renderer.ShowAlert("Server connection required", "This screen needs a server connection.")
						window.Invalidate()
						continue
					}
					previous := navigation
					navigation = next
					if previous != next {
						navigationHistory = append(navigationHistory, previous)
					}
					stopOutput()
					if err := refreshOfflineScreen(renderer, screens, navigation, options.Version, mode, endpoint, sshSettings); err != nil {
						renderer.ShowAlert("Screen unavailable", err.Error())
					}
					applyNativeConnectionState(renderer, nativeConnectionState{connecting: reconnect != nil, status: "Server unavailable; reconnecting…"})
					window.Invalidate()
					continue
				}
				if err := beginForwardNavigation(next); err != nil {
					renderer.ShowAlert("Navigation failed", err.Error())
				}
				window.Invalidate()
				continue
			}
			if client == nil && command.action.Command != "change-theme" && command.action.Command != "set-project-import-field" {
				renderer.ShowAlert("Server offline", "Reconnect is in progress.")
				window.Invalidate()
				continue
			}
			if command.action.Command == "set-run-option" && navigation.screen == "run-options" {
				field := strings.TrimSpace(command.arguments["field"])
				value := strings.TrimSpace(command.arguments["value"])
				refreshEligibility, selectionErr := applyRunOptionSelection(renderer, &navigation, field, value)
				if selectionErr != nil {
					renderer.ShowAlert("Invalid run option", selectionErr.Error())
					window.Invalidate()
					continue
				}
				if !refreshEligibility {
					window.Invalidate()
					continue
				}
				renderer.SetRootBinding("runOptions", "target_kind", "loading")
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
			scheduleNoticeExpiry()
			window.Invalidate()
		}
	}
}

func shouldRefreshAfterNativeOperation(catalog *uidsl.ActionCatalogDocument, operation operations.Operation, effect nativeOperationEffect) bool {
	if effect.Refresh {
		return true
	}
	spec, ok := catalog.Spec(operation.Command)
	return ok && spec.RefreshOnSuccess
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
			renderer.ShowAlert("Project structure unavailable", "The selected project structure filter is unavailable.")
			return
		}
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
			renderer.ShowAlert("Invalid action", "Unknown agent script field.")
		}
	case "set-project-import-field":
		binding := map[string]string{"repoUrl": "import_repo_url", "repoRef": "import_repo_ref", "configFile": "import_config_file"}[strings.TrimSpace(command.arguments["field"])]
		if binding == "" {
			renderer.ShowAlert("Invalid action", "Unknown project import field.")
			return
		}
		renderer.SetRootBinding("settings", binding, command.arguments["value"])
	case "set-managed-yaml-field":
		if strings.TrimSpace(command.arguments["field"]) != "yaml" {
			renderer.ShowAlert("Invalid action", "Unknown managed YAML field.")
			return
		}
		renderer.SetRootBinding("managedYAML", "yaml", command.arguments["value"])
	case "set-vault-field":
		field := strings.TrimSpace(command.arguments["field"])
		if field != "name" && field != "url" && field != "role_id" && field != "approle_mount" && field != "secret_id_env" {
			renderer.ShowAlert("Invalid action", "Unknown Vault connection field.")
			return
		}
		renderer.SetRootBinding("vault", field, command.arguments["value"])
	case "set-server-update-option":
		binding := map[string]string{
			"update": "selected_update_version", "rollback": "selected_rollback_version",
		}[strings.TrimSpace(command.arguments["field"])]
		if binding == "" {
			renderer.ShowAlert("Invalid action", "Unknown server update option.")
			return
		}
		renderer.SetRootBinding("settings", binding, strings.TrimSpace(command.arguments["value"]))
	case "change-theme":
		theme, err := findTheme(command.arguments["theme"])
		if err != nil {
			renderer.ShowAlert("Theme change failed", err.Error())
			return
		}
		if err := renderer.SetTheme(theme); err != nil {
			renderer.ShowAlert("Theme change failed", err.Error())
			return
		}
		renderer.SetRootBinding("settings", "selected_theme", theme.Metadata.Name)
		renderer.SetRootBinding("settings", "selected_theme_description", theme.Metadata.Description)
		if err := updateNativePreferences(preferencesPath, func(preferences *nativePreferences) {
			preferences.Theme = theme.Metadata.Name
		}); err != nil {
			renderer.ShowNotice("Theme changed, but the preference could not be saved: "+err.Error(), "", uidsl.Action{}, nil, presentation.TransientNoticeDuration)
			return
		}
	default:
		renderer.ShowAlert("Unsupported native action", command.action.Command)
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

func nativeBackNavigationTarget(history []navigationState, fallbackRoute string) (navigationState, bool, error) {
	if len(history) > 0 {
		return history[len(history)-1], true, nil
	}
	target, err := navigationForRoute(fallbackRoute)
	if err == nil {
		return target, false, nil
	}
	target, rootErr := navigationForRoute("/")
	if rootErr != nil {
		return navigationState{}, false, rootErr
	}
	return target, false, nil
}

func nativeRunOptionsOperationMatches(navigation navigationState, operation operations.Operation) bool {
	if navigation.screen != "run-options" {
		return false
	}
	switch operation.Command {
	case "run-pipeline":
		pipelineID, err := strconv.ParseInt(strings.TrimSpace(operation.Arguments["pipelineDbId"]), 10, 64)
		return err == nil && pipelineID > 0 && navigation.pipelineDBID == pipelineID
	case "run-chain":
		projectID, err := strconv.ParseInt(strings.TrimSpace(operation.Arguments["projectId"]), 10, 64)
		return err == nil && projectID > 0 && navigation.projectID == projectID && navigation.chainID == strings.TrimSpace(operation.Arguments["chainId"])
	default:
		return false
	}
}
