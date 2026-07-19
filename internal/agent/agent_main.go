package agent

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/izzyreal/ciwi/internal/protocol"
)

const (
	agentJobHTTPTimeout       = 10 * time.Minute
	agentHeartbeatHTTPTimeout = 3 * time.Second
	agentLeaseHTTPTimeout     = 5 * time.Second
)

var (
	selfUpdateExecutablePathFn = os.Executable
	selfUpdateOpenFileFn       = func(name string, flag int, perm fs.FileMode) (*os.File, error) {
		return os.OpenFile(name, flag, perm)
	}
)

type pendingUpdateRequest struct {
	target     string
	repository string
	apiBase    string
}

type deferredControl struct {
	jobInProgress         bool
	pendingUpdate         *pendingUpdateRequest
	pendingRestart        bool
	pendingCacheWipe      bool
	pendingJobHistoryWipe bool
}

type heartbeatResult struct {
	resp              protocol.HeartbeatResponse
	err               error
	sentUpdateFailure string
	sentRestartStatus string
}

type jobResult struct {
	jobID string
	err   error
}

type agentHeartbeatState struct {
	mu                   sync.Mutex
	pendingUpdateFailure string
	updateInProgress     bool
	pendingRestartStatus string
}

func (s *agentHeartbeatState) snapshot() (string, bool, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pendingUpdateFailure, s.updateInProgress, s.pendingRestartStatus
}

func (s *agentHeartbeatState) setPendingUpdateFailure(value string) {
	s.mu.Lock()
	s.pendingUpdateFailure = strings.TrimSpace(value)
	s.mu.Unlock()
}

func (s *agentHeartbeatState) setUpdateInProgress(value bool) {
	s.mu.Lock()
	s.updateInProgress = value
	s.mu.Unlock()
}

func (s *agentHeartbeatState) setPendingRestartStatus(value string) {
	s.mu.Lock()
	s.pendingRestartStatus = strings.TrimSpace(value)
	s.mu.Unlock()
}

func (s *agentHeartbeatState) ack(sentUpdateFailure, sentRestartStatus string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pendingUpdateFailure == sentUpdateFailure {
		s.pendingUpdateFailure = ""
	}
	if s.pendingRestartStatus == sentRestartStatus {
		s.pendingRestartStatus = ""
	}
}

type agentLoopDeps struct {
	ctx              context.Context
	serverURL        string
	agentID          string
	workDir          string
	restartArgs      []string
	jobClient        *http.Client
	leaseClient      *http.Client
	control          *deferredControl
	heartbeatState   *agentHeartbeatState
	jobDoneCh        chan jobResult
	triggerHeartbeat func()
	detectCapsFn     func() map[string]string
	getCapsFn        func() map[string]string
	setCapsFn        func(map[string]string)
	selfUpdateFn     func(context.Context, string, string, string, []string) error
	requestRestartFn func() string
	wipeCacheFn      func(string) (string, error)
	wipeHistoryFn    func(string) (string, error)
	leaseJobFn       func(context.Context, *http.Client, string, string, map[string]string) (*protocol.JobExecution, error)
	executeJobFn     func(context.Context, *http.Client, string, string, string, map[string]string, protocol.JobExecution) error
	heartbeatNowFn   func() heartbeatResult
}

func (d *deferredControl) setJobInProgress(v bool) {
	d.jobInProgress = v
}

func (d *deferredControl) requestUpdate(target, repository, apiBase string, onDefer func(), runNow func(string, string, string)) {
	target = strings.TrimSpace(target)
	if target == "" {
		return
	}
	if d.jobInProgress {
		d.pendingUpdate = &pendingUpdateRequest{target: target, repository: repository, apiBase: apiBase}
		if onDefer != nil {
			onDefer()
		}
		return
	}
	runNow(target, repository, apiBase)
}

func (d *deferredControl) requestRestart(onDefer func(), runNow func()) {
	if d.jobInProgress {
		d.pendingRestart = true
		if onDefer != nil {
			onDefer()
		}
		return
	}
	runNow()
}

func (d *deferredControl) requestCacheWipe(onDefer func(), runNow func()) {
	if d.jobInProgress {
		d.pendingCacheWipe = true
		if onDefer != nil {
			onDefer()
		}
		return
	}
	runNow()
}

func (d *deferredControl) requestJobHistoryWipe(onDefer func(), runNow func()) {
	if d.jobInProgress {
		d.pendingJobHistoryWipe = true
		if onDefer != nil {
			onDefer()
		}
		return
	}
	runNow()
}

func (d *deferredControl) requeueJobHistoryWipe() {
	d.pendingJobHistoryWipe = true
}

func (d *deferredControl) hasDeferred() bool {
	return d.pendingUpdate != nil ||
		d.pendingRestart ||
		d.pendingCacheWipe ||
		d.pendingJobHistoryWipe
}

func (d *deferredControl) flushDeferred(runUpdate func(string, string, string), runRestart, runCacheWipe, runJobHistoryWipe func()) {
	d.jobInProgress = false
	if d.pendingUpdate != nil {
		req := *d.pendingUpdate
		d.pendingUpdate = nil
		runUpdate(req.target, req.repository, req.apiBase)
	}
	if d.pendingRestart {
		d.pendingRestart = false
		runRestart()
	}
	if d.pendingCacheWipe {
		d.pendingCacheWipe = false
		runCacheWipe()
	}
	if d.pendingJobHistoryWipe {
		d.pendingJobHistoryWipe = false
		runJobHistoryWipe()
	}
}

func (d *agentLoopDeps) runOrDeferUpdate(target, repository, apiBase string) {
	d.control.requestUpdate(target, repository, apiBase, func() {
		slog.Info("server requested agent update; deferring until current job completes", "target_version", target)
	}, func(target, repository, apiBase string) {
		slog.Info("server requested agent update", "target_version", target)
		d.heartbeatState.setUpdateInProgress(true)
		d.triggerHeartbeat()
		if err := d.selfUpdateFn(d.ctx, target, repository, apiBase, d.restartArgs); err != nil {
			slog.Error("agent self-update failed", "error", err)
			d.heartbeatState.setUpdateInProgress(false)
			d.heartbeatState.setPendingUpdateFailure(err.Error())
			d.triggerHeartbeat()
		}
	})
}

func (d *agentLoopDeps) runOrDeferRestart() {
	d.control.requestRestart(func() {
		d.heartbeatState.setPendingRestartStatus("restart deferred: agent busy with active job")
		d.triggerHeartbeat()
		slog.Info("server requested agent restart; deferring until current job completes")
	}, func() {
		slog.Info("server requested agent restart")
		d.heartbeatState.setPendingRestartStatus(d.requestRestartFn())
		d.triggerHeartbeat()
	})
}

func (d *agentLoopDeps) runOrDeferCacheWipe() {
	d.control.requestCacheWipe(func() {
		slog.Info("server requested cache wipe; deferring until current job completes")
	}, func() {
		slog.Info("server requested cache wipe")
		msg, err := d.wipeCacheFn(d.workDir)
		if err != nil {
			slog.Error("agent cache wipe failed", "error", err)
			return
		}
		slog.Info(msg)
	})
}

func (d *agentLoopDeps) runOrDeferJobHistoryWipe() {
	d.control.requestJobHistoryWipe(func() {
		slog.Info("server requested local job history wipe; deferring until current job completes")
	}, func() {
		slog.Info("server requested local job history wipe")
		msg, err := d.wipeHistoryFn(d.workDir)
		if err != nil {
			d.control.requeueJobHistoryWipe()
			slog.Error("agent local job history wipe failed", "error", err)
			return
		}
		slog.Info(msg)
	})
}

func (d *agentLoopDeps) flushDeferred() {
	d.control.flushDeferred(d.runOrDeferUpdate, d.runOrDeferRestart, d.runOrDeferCacheWipe, d.runOrDeferJobHistoryWipe)
}

func (d *agentLoopDeps) processHeartbeat(hb protocol.HeartbeatResponse) {
	if hb.RefreshToolsRequested {
		d.setCapsFn(d.detectCapsFn())
		slog.Info("server requested tools refresh")
		d.triggerHeartbeat()
	}
	if hb.UpdateRequested {
		d.runOrDeferUpdate(hb.UpdateTarget, hb.UpdateRepository, hb.UpdateAPIBase)
	}
	if hb.RestartRequested {
		d.runOrDeferRestart()
	}
	if hb.WipeCacheRequested {
		d.runOrDeferCacheWipe()
	}
	if hb.FlushJobHistoryRequested {
		d.runOrDeferJobHistoryWipe()
	}
}

func (d *agentLoopDeps) handleHeartbeatResult(hbRes heartbeatResult) {
	if hbRes.err != nil {
		slog.Error("heartbeat failed", "error", hbRes.err)
		return
	}
	d.heartbeatState.ack(hbRes.sentUpdateFailure, hbRes.sentRestartStatus)
	d.processHeartbeat(hbRes.resp)
	if !d.control.jobInProgress && d.control.hasDeferred() {
		d.flushDeferred()
	}
}

func (d *agentLoopDeps) handleLeaseTick() {
	if !d.control.jobInProgress && d.control.hasDeferred() {
		d.flushDeferred()
		return
	}
	if d.control.jobInProgress {
		return
	}
	if d.heartbeatNowFn != nil {
		hbRes := d.heartbeatNowFn()
		d.handleHeartbeatResult(hbRes)
		if hbRes.err != nil {
			return
		}
		if hbRes.resp.UpdateRequested {
			return
		}
		if !d.control.jobInProgress && d.control.hasDeferred() {
			d.flushDeferred()
			return
		}
		if d.control.jobInProgress {
			return
		}
	}
	job, err := d.leaseJobFn(d.ctx, d.leaseClient, d.serverURL, d.agentID, d.getCapsFn())
	if err != nil {
		slog.Error("lease failed", "error", err)
		return
	}
	if job == nil {
		return
	}
	d.control.setJobInProgress(true)
	jobCaps := d.getCapsFn()
	go func(leased protocol.JobExecution, caps map[string]string) {
		d.jobDoneCh <- jobResult{
			jobID: leased.ID,
			err:   d.executeJobFn(d.ctx, d.jobClient, d.serverURL, d.agentID, d.workDir, caps, leased),
		}
	}(*job, jobCaps)
}

func Run(ctx context.Context) error {
	loadAgentPlatformEnv()
	if handled, err := runAsWindowsServiceIfNeeded(runLoop); handled {
		return err
	}
	return runLoop(ctx)
}

func runLoop(ctx context.Context) error {
	serverURL := envOrDefault("CIWI_SERVER_URL", "http://127.0.0.1:8112")
	agentID := envOrDefault("CIWI_AGENT_ID", defaultAgentID())
	hostname, _ := os.Hostname()
	workDir := agentWorkDir()

	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return fmt.Errorf("create agent workdir: %w", err)
	}
	if reason := selfUpdateWritabilityWarning(); reason != "" {
		slog.Warn("agent self-update preflight warning", "reason", reason)
	}

	slog.Info("ciwi agent started", "agent_id", agentID, "version", currentVersion(), "server_url", serverURL)
	defer slog.Info("ciwi agent stopped", "agent_id", agentID)

	jobClient := &http.Client{Timeout: agentJobHTTPTimeout}
	heartbeatClient := &http.Client{Timeout: agentHeartbeatHTTPTimeout}
	leaseClient := &http.Client{Timeout: agentLeaseHTTPTimeout}
	leaseTicker := time.NewTicker(3 * time.Second)
	defer leaseTicker.Stop()
	capabilities := detectAgentCapabilities()
	var capabilitiesMu sync.Mutex
	getCapabilities := func() map[string]string {
		capabilitiesMu.Lock()
		defer capabilitiesMu.Unlock()
		return cloneMap(capabilities)
	}
	setCapabilities := func(next map[string]string) {
		capabilitiesMu.Lock()
		capabilities = next
		capabilitiesMu.Unlock()
	}

	heartbeatState := &agentHeartbeatState{pendingRestartStatus: startupHeartbeatGreeting()}
	control := &deferredControl{}
	jobDoneCh := make(chan jobResult, 1)
	heartbeatRespCh := make(chan heartbeatResult, 4)
	heartbeatKickCh := make(chan struct{}, 1)
	triggerHeartbeat := func() {
		select {
		case heartbeatKickCh <- struct{}{}:
		default:
		}
	}
	loopDeps := &agentLoopDeps{
		ctx:              ctx,
		serverURL:        serverURL,
		agentID:          agentID,
		workDir:          workDir,
		restartArgs:      os.Args[1:],
		jobClient:        jobClient,
		leaseClient:      leaseClient,
		control:          control,
		heartbeatState:   heartbeatState,
		jobDoneCh:        jobDoneCh,
		triggerHeartbeat: triggerHeartbeat,
		detectCapsFn:     detectAgentCapabilities,
		getCapsFn:        getCapabilities,
		setCapsFn:        setCapabilities,
		selfUpdateFn:     selfUpdateAndRestart,
		requestRestartFn: requestAgentRestart,
		wipeCacheFn:      wipeAgentCache,
		wipeHistoryFn:    wipeAgentJobHistory,
		leaseJobFn:       leaseJob,
		executeJobFn:     executeLeasedJob,
	}

	go func() {
		ticker := time.NewTicker(protocol.AgentHeartbeatInterval)
		defer ticker.Stop()

		send := func() heartbeatResult {
			updateFailure, updateInProgress, restartStatus := heartbeatState.snapshot()
			hb, err := sendHeartbeat(ctx, heartbeatClient, serverURL, agentID, hostname, getCapabilities(), updateFailure, updateInProgress, restartStatus)
			return heartbeatResult{
				resp:              hb,
				err:               err,
				sentUpdateFailure: updateFailure,
				sentRestartStatus: restartStatus,
			}
		}
		sendAsync := func() {
			res := send()
			select {
			case heartbeatRespCh <- res:
			case <-ctx.Done():
			}
		}

		sendAsync()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				sendAsync()
			case <-heartbeatKickCh:
				sendAsync()
			}
		}
	}()
	loopDeps.heartbeatNowFn = func() heartbeatResult {
		updateFailure, updateInProgress, restartStatus := heartbeatState.snapshot()
		hb, err := sendHeartbeat(ctx, heartbeatClient, serverURL, agentID, hostname, getCapabilities(), updateFailure, updateInProgress, restartStatus)
		return heartbeatResult{
			resp:              hb,
			err:               err,
			sentUpdateFailure: updateFailure,
			sentRestartStatus: restartStatus,
		}
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case done := <-jobDoneCh:
			if done.err != nil {
				slog.Error("execute job failed", "job_execution_id", done.jobID, "error", done.err)
			}
			loopDeps.flushDeferred()
		case hbRes := <-heartbeatRespCh:
			loopDeps.handleHeartbeatResult(hbRes)
		case <-leaseTicker.C:
			loopDeps.handleLeaseTick()
		}
	}
}

func selfUpdateWritabilityWarning() string {
	if reason := selfUpdateServiceModeReason(); reason != "" {
		return reason
	}
	exePath, err := selfUpdateExecutablePathFn()
	if err != nil {
		return "cannot resolve executable path: " + err.Error()
	}
	if looksLikeGoRunBinary(exePath) {
		return "running via go run binary path; self-update is unavailable"
	}
	f, err := selfUpdateOpenFileFn(exePath, os.O_WRONLY, 0)
	if err != nil {
		return "binary path is not writable by current user (" + strings.TrimSpace(exePath) + "): " + err.Error()
	}
	_ = f.Close()
	return ""
}

func startupHeartbeatGreeting() string {
	return fmt.Sprintf("ciwi agent has (re)started (pid=%d)", os.Getpid())
}
