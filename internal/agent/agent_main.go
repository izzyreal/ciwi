package agent

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/izzyreal/ciwi/internal/protocol"
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

	client := &http.Client{Timeout: 10 * time.Minute}
	heartbeatTicker := time.NewTicker(protocol.AgentHeartbeatInterval)
	defer heartbeatTicker.Stop()
	leaseTicker := time.NewTicker(3 * time.Second)
	defer leaseTicker.Stop()
	capabilities := detectAgentCapabilities()
	pendingUpdateFailure := ""
	pendingRestartStatus := ""
	control := &deferredControl{}
	type jobResult struct {
		jobID string
		err   error
	}
	jobDoneCh := make(chan jobResult, 1)

	runOrDeferUpdate := func(target, repository, apiBase string) {
		control.requestUpdate(target, repository, apiBase, func() {
			slog.Info("server requested agent update; deferring until current job completes", "target_version", target)
		}, func(target, repository, apiBase string) {
			slog.Info("server requested agent update", "target_version", target)
			if err := selfUpdateAndRestart(ctx, target, repository, apiBase, os.Args[1:]); err != nil {
				slog.Error("agent self-update failed", "error", err)
				pendingUpdateFailure = err.Error()
			}
		})
	}
	runOrDeferRestart := func() {
		control.requestRestart(func() {
			pendingRestartStatus = "restart deferred: agent busy with active job"
			slog.Info("server requested agent restart; deferring until current job completes")
		}, func() {
			slog.Info("server requested agent restart")
			pendingRestartStatus = requestAgentRestart()
			if pendingRestartStatus != "" {
				if _, err := sendHeartbeat(ctx, client, serverURL, agentID, hostname, capabilities, pendingUpdateFailure, pendingRestartStatus); err != nil {
					slog.Error("heartbeat failed while reporting restart status", "error", err)
				} else {
					pendingUpdateFailure = ""
					pendingRestartStatus = ""
				}
			}
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

	if hb, err := sendHeartbeat(ctx, client, serverURL, agentID, hostname, capabilities, pendingUpdateFailure, pendingRestartStatus); err != nil {
		slog.Error("initial heartbeat failed", "error", err)
	} else {
		pendingUpdateFailure = ""
		pendingRestartStatus = ""
		if hb.RefreshToolsRequested {
			capabilities = detectAgentCapabilities()
			slog.Info("server requested tools refresh")
			if _, err := sendHeartbeat(ctx, client, serverURL, agentID, hostname, capabilities, pendingUpdateFailure, pendingRestartStatus); err != nil {
				slog.Error("heartbeat failed", "error", err)
			} else {
				pendingUpdateFailure = ""
				pendingRestartStatus = ""
			}
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

	for {
		select {
		case <-ctx.Done():
			return nil
		case done := <-jobDoneCh:
			if done.err != nil {
				slog.Error("execute job failed", "job_execution_id", done.jobID, "error", done.err)
			}
			control.flushDeferred(runOrDeferUpdate, runOrDeferRestart, runOrDeferCacheWipe, runOrDeferJobHistoryWipe)
		case <-heartbeatTicker.C:
			hb, err := sendHeartbeat(ctx, client, serverURL, agentID, hostname, capabilities, pendingUpdateFailure, pendingRestartStatus)
			if err != nil {
				slog.Error("heartbeat failed", "error", err)
			} else {
				pendingUpdateFailure = ""
				pendingRestartStatus = ""
				if hb.RefreshToolsRequested {
					capabilities = detectAgentCapabilities()
					slog.Info("server requested tools refresh")
					if _, err := sendHeartbeat(ctx, client, serverURL, agentID, hostname, capabilities, pendingUpdateFailure, pendingRestartStatus); err != nil {
						slog.Error("heartbeat failed", "error", err)
					} else {
						pendingUpdateFailure = ""
						pendingRestartStatus = ""
					}
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
		case <-leaseTicker.C:
			if control.jobInProgress {
				continue
			}
			job, err := leaseJob(ctx, client, serverURL, agentID, capabilities)
			if err != nil {
				slog.Error("lease failed", "error", err)
				continue
			}
			if job == nil {
				continue
			}
			control.setJobInProgress(true)
			jobCaps := cloneMap(capabilities)
			go func(leased protocol.JobExecution, caps map[string]string) {
				jobDoneCh <- jobResult{
					jobID: leased.ID,
					err:   executeLeasedJob(ctx, client, serverURL, agentID, workDir, caps, leased),
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
