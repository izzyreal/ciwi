package agent

import (
	"context"
	"fmt"
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

	slog.Info("ciwi agent started", "agent_id", agentID, "server_url", serverURL)
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

	pendingUpdateFailure := ""
	pendingRestartStatus := ""
	var heartbeatStateMu sync.Mutex
	getHeartbeatState := func() (string, string) {
		heartbeatStateMu.Lock()
		defer heartbeatStateMu.Unlock()
		return pendingUpdateFailure, pendingRestartStatus
	}
	setPendingUpdateFailure := func(value string) {
		heartbeatStateMu.Lock()
		pendingUpdateFailure = strings.TrimSpace(value)
		heartbeatStateMu.Unlock()
	}
	setPendingRestartStatus := func(value string) {
		heartbeatStateMu.Lock()
		pendingRestartStatus = strings.TrimSpace(value)
		heartbeatStateMu.Unlock()
	}
	ackHeartbeatState := func(sentUpdateFailure, sentRestartStatus string) {
		heartbeatStateMu.Lock()
		defer heartbeatStateMu.Unlock()
		if pendingUpdateFailure == sentUpdateFailure {
			pendingUpdateFailure = ""
		}
		if pendingRestartStatus == sentRestartStatus {
			pendingRestartStatus = ""
		}
	}
	control := &deferredControl{}
	type jobResult struct {
		jobID string
		err   error
	}
	jobDoneCh := make(chan jobResult, 1)
	heartbeatRespCh := make(chan heartbeatResult, 4)
	heartbeatKickCh := make(chan struct{}, 1)
	triggerHeartbeat := func() {
		select {
		case heartbeatKickCh <- struct{}{}:
		default:
		}
	}

	runOrDeferUpdate := func(target, repository, apiBase string) {
		control.requestUpdate(target, repository, apiBase, func() {
			slog.Info("server requested agent update; deferring until current job completes", "target_version", target)
		}, func(target, repository, apiBase string) {
			slog.Info("server requested agent update", "target_version", target)
			if err := selfUpdateAndRestart(ctx, target, repository, apiBase, os.Args[1:]); err != nil {
				slog.Error("agent self-update failed", "error", err)
				setPendingUpdateFailure(err.Error())
				triggerHeartbeat()
			}
		})
	}
	runOrDeferRestart := func() {
		control.requestRestart(func() {
			setPendingRestartStatus("restart deferred: agent busy with active job")
			triggerHeartbeat()
			slog.Info("server requested agent restart; deferring until current job completes")
		}, func() {
			slog.Info("server requested agent restart")
			setPendingRestartStatus(requestAgentRestart())
			triggerHeartbeat()
		})
	}
	runOrDeferCacheWipe := func() {
		control.requestCacheWipe(func() {
			slog.Info("server requested cache wipe; deferring until current job completes")
		}, func() {
			slog.Info("server requested cache wipe")
			msg, err := wipeAgentCache(workDir)
			if err != nil {
				slog.Error("agent cache wipe failed", "error", err)
				return
			}
			slog.Info(msg)
		})
	}
	runOrDeferJobHistoryWipe := func() {
		control.requestJobHistoryWipe(func() {
			slog.Info("server requested local job history wipe; deferring until current job completes")
		}, func() {
			slog.Info("server requested local job history wipe")
			msg, err := wipeAgentJobHistory(workDir)
			if err != nil {
				slog.Error("agent local job history wipe failed", "error", err)
				return
			}
			slog.Info(msg)
		})
	}

	processHeartbeat := func(hb protocol.HeartbeatResponse) {
		if hb.RefreshToolsRequested {
			setCapabilities(detectAgentCapabilities())
			slog.Info("server requested tools refresh")
			triggerHeartbeat()
		}
		if hb.UpdateRequested {
			runOrDeferUpdate(hb.UpdateTarget, hb.UpdateRepository, hb.UpdateAPIBase)
		}
		if hb.RestartRequested {
			runOrDeferRestart()
		}
		if hb.WipeCacheRequested {
			runOrDeferCacheWipe()
		}
		if hb.FlushJobHistoryRequested {
			runOrDeferJobHistoryWipe()
		}
	}

	go func() {
		ticker := time.NewTicker(protocol.AgentHeartbeatInterval)
		defer ticker.Stop()

		send := func() {
			updateFailure, restartStatus := getHeartbeatState()
			hb, err := sendHeartbeat(ctx, heartbeatClient, serverURL, agentID, hostname, getCapabilities(), updateFailure, restartStatus)
			res := heartbeatResult{
				resp:              hb,
				err:               err,
				sentUpdateFailure: updateFailure,
				sentRestartStatus: restartStatus,
			}
			select {
			case heartbeatRespCh <- res:
			case <-ctx.Done():
			}
		}

		send()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				send()
			case <-heartbeatKickCh:
				send()
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return nil
		case done := <-jobDoneCh:
			if done.err != nil {
				slog.Error("execute job failed", "job_execution_id", done.jobID, "error", done.err)
			}
			control.flushDeferred(runOrDeferUpdate, runOrDeferRestart, runOrDeferCacheWipe, runOrDeferJobHistoryWipe)
		case hbRes := <-heartbeatRespCh:
			if hbRes.err != nil {
				slog.Error("heartbeat failed", "error", hbRes.err)
			} else {
				ackHeartbeatState(hbRes.sentUpdateFailure, hbRes.sentRestartStatus)
				processHeartbeat(hbRes.resp)
			}
		case <-leaseTicker.C:
			if control.jobInProgress {
				continue
			}
			job, err := leaseJob(ctx, leaseClient, serverURL, agentID, getCapabilities())
			if err != nil {
				slog.Error("lease failed", "error", err)
				continue
			}
			if job == nil {
				continue
			}
			control.setJobInProgress(true)
			jobCaps := getCapabilities()
			go func(leased protocol.JobExecution, caps map[string]string) {
				jobDoneCh <- jobResult{
					jobID: leased.ID,
					err:   executeLeasedJob(ctx, jobClient, serverURL, agentID, workDir, caps, leased),
				}
			}(*job, jobCaps)
		}
	}
}

func selfUpdateWritabilityWarning() string {
	if reason := selfUpdateServiceModeReason(); reason != "" {
		return reason
	}
	exePath, err := os.Executable()
	if err != nil {
		return "cannot resolve executable path: " + err.Error()
	}
	if looksLikeGoRunBinary(exePath) {
		return "running via go run binary path; self-update is unavailable"
	}
	f, err := os.OpenFile(exePath, os.O_WRONLY, 0)
	if err != nil {
		return "binary path is not writable by current user (" + strings.TrimSpace(exePath) + "): " + err.Error()
	}
	_ = f.Close()
	return ""
}
