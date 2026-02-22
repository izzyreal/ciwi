package agent

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/izzyreal/ciwi/internal/protocol"
)

const (
	executorScript  = "script"
	shellPosix      = "posix"
	shellCmd        = "cmd"
	shellPowerShell = "powershell"
)

func executeLeasedJob(ctx context.Context, client *http.Client, serverURL, agentID, workDir string, agentCapabilities map[string]string, job protocol.JobExecution) error {
	slog.Info("job execution started",
		"job_execution_id", job.ID,
		"agent_id", agentID,
		"timeout_seconds", job.TimeoutSeconds,
		"has_source", job.Source != nil && strings.TrimSpace(job.Source.Repo) != "",
	)
	if err := reportJobStatus(ctx, client, serverURL, job.ID, protocol.JobExecutionStatusUpdateRequest{
		AgentID:      agentID,
		Status:       protocol.JobExecutionStatusRunning,
		CurrentStep:  "Preparing execution",
		TimestampUTC: time.Now().UTC(),
	}); err != nil {
		return fmt.Errorf("report running status: %w", err)
	}

	workspaceDir := workspaceDirForJob(workDir, job)
	if err := os.RemoveAll(workspaceDir); err != nil {
		return reportFailure(ctx, client, serverURL, agentID, job, nil, fmt.Sprintf("prepare workdir: %v", err), "")
	}
	if err := os.MkdirAll(workspaceDir, 0o755); err != nil {
		return reportFailure(ctx, client, serverURL, agentID, job, nil, fmt.Sprintf("create workdir: %v", err), "")
	}

	runCtx := ctx
	cancel := func() {}
	if job.TimeoutSeconds > 0 {
		runCtx, cancel = context.WithTimeout(ctx, time.Duration(job.TimeoutSeconds)*time.Second)
	}
	defer cancel()
	runCtx, cancelRun := context.WithCancel(runCtx)
	defer cancelRun()

	var output syncBuffer
	fmt.Fprintf(&output, "[meta] agent=%s os=%s arch=%s\n", agentID, runtime.GOOS, runtime.GOARCH)
	fmt.Fprintf(&output, "[meta] job_execution_id=%s timeout_seconds=%d\n", job.ID, job.TimeoutSeconds)
	stopControlMonitor := monitorServerTerminalJobState(runCtx, client, serverURL, agentID, job.ID, &output, cancelRun)
	defer stopControlMonitor()

	fmt.Fprintf(&output, "[meta] workspace=%s\n", workspaceDir)
	execDir := workspaceDir
	if job.Source != nil && strings.TrimSpace(job.Source.Repo) != "" {
		sourceDir := filepath.Join(workspaceDir, "src")
		checkoutStart := time.Now()
		fmt.Fprintf(&output, "[checkout] repo=%s ref=%s\n", job.Source.Repo, job.Source.Ref)
		if err := reportJobStatus(ctx, client, serverURL, job.ID, protocol.JobExecutionStatusUpdateRequest{
			AgentID:      agentID,
			Status:       protocol.JobExecutionStatusRunning,
			Output:       redactSensitive(trimOutput(output.String()), job.SensitiveValues),
			CurrentStep:  "Checking out source",
			TimestampUTC: time.Now().UTC(),
		}); err != nil {
			return fmt.Errorf("report checkout status: %w", err)
		}
		checkoutOutput, checkoutErr := checkoutSource(runCtx, sourceDir, *job.Source)
		output.WriteString(checkoutOutput)
		fmt.Fprintf(&output, "[checkout] duration=%s\n", time.Since(checkoutStart).Round(time.Millisecond))
		if checkoutErr != nil {
			exitCode := exitCodeFromErr(checkoutErr)
			failMsg := "checkout failed: " + checkoutErr.Error()
			trimmedOutput := redactSensitive(trimOutput(output.String()), job.SensitiveValues)
			if reportErr := reportFailure(ctx, client, serverURL, agentID, job, exitCode, failMsg, trimmedOutput); reportErr != nil {
				return reportErr
			}
			slog.Error("job failed during checkout", "job_execution_id", job.ID, "error", failMsg)
			return nil
		}
		execDir = sourceDir
	}
	depJobIDs := dependencyArtifactJobIDs(job.Env)
	if len(depJobIDs) > 0 {
		fmt.Fprintf(&output, "[dep-artifacts] source_jobs=%s\n", strings.Join(depJobIDs, ","))
		for _, depJobID := range depJobIDs {
			note, depErr := downloadDependencyArtifacts(runCtx, client, serverURL, depJobID, execDir)
			if note != "" {
				output.WriteString(note)
				if !strings.HasSuffix(note, "\n") {
					output.WriteString("\n")
				}
			}
			if depErr != nil {
				exitCode := exitCodeFromErr(depErr)
				failMsg := "dependency artifact download failed: " + depErr.Error()
				trimmedOutput := redactSensitive(trimOutput(output.String()), job.SensitiveValues)
				if reportErr := reportFailure(ctx, client, serverURL, agentID, job, exitCode, failMsg, trimmedOutput); reportErr != nil {
					return reportErr
				}
				slog.Error("job failed during dependency artifact download", "job_execution_id", job.ID, "error", failMsg)
				return nil
			}
		}
	}
	cacheEnv, cacheLogs, resolvedCaches := resolveJobCacheEnvDetailed(workDir, execDir, job)
	for _, line := range cacheLogs {
		fmt.Fprintf(&output, "[cache] %s\n", line)
	}
	cacheStats := collectJobCacheStats(resolvedCaches)
	refreshCacheStats := func() []protocol.JobCacheStats {
		return collectJobCacheStats(resolvedCaches)
	}
	probeContainer := runtimeProbeContainerName(job.ID, job.Metadata)
	probeContainerImage := runtimeProbeContainerImageFromMetadata(job.Metadata)
	probeContainerWorkdir := runtimeExecContainerWorkdirFromMetadata(job.Metadata)
	if strings.TrimSpace(probeContainerWorkdir) == "" {
		probeContainerWorkdir = "/workspace"
	}
	probeContainerUser := runtimeExecContainerUserFromMetadata(job.Metadata)
	if strings.TrimSpace(probeContainerUser) == "" {
		probeContainerUser = defaultContainerUserSpec()
	}
	probeContainerDevices := runtimeExecContainerDevicesFromMetadata(job.Metadata)
	probeContainerGroups := runtimeExecContainerGroupsFromMetadata(job.Metadata)
	requireContainerTools := len(containerToolRequirements(job.RequiredCapabilities)) > 0
	if requireContainerTools && strings.TrimSpace(probeContainerImage) == "" {
		err := fmt.Errorf("container tool requirements require runs_on.container_image")
		fmt.Fprintf(&output, "[runtime] %v\n", err)
		trimmedOutput := redactSensitive(trimOutput(output.String()), job.SensitiveValues)
		if reportErr := reportTerminalJobStatusWithRetry(client, serverURL, job.ID, protocol.JobExecutionStatusUpdateRequest{
			AgentID:      agentID,
			Status:       protocol.JobExecutionStatusFailed,
			Error:        err.Error(),
			Output:       trimmedOutput,
			CacheStats:   cacheStats,
			TimestampUTC: time.Now().UTC(),
		}); reportErr != nil {
			return reportErr
		}
		slog.Error("job failed", "job_execution_id", job.ID, "error", err.Error())
		return nil
	}
	containerExec := strings.TrimSpace(probeContainerImage) != ""
	execContainer := (*executionContainerContext)(nil)
	if containerExec {
		mounts := []runtimeContainerMount{
			{hostPath: execDir, containerPath: probeContainerWorkdir},
		}
		cacheMountSeen := map[string]struct{}{}
		for _, hostPath := range cacheEnv {
			hostPath = strings.TrimSpace(hostPath)
			if hostPath == "" {
				continue
			}
			if _, ok := cacheMountSeen[hostPath]; ok {
				continue
			}
			cacheMountSeen[hostPath] = struct{}{}
			mounts = append(mounts, runtimeContainerMount{
				hostPath:      hostPath,
				containerPath: hostPath,
			})
		}
		startErr := startRuntimeContainer(runCtx, runtimeContainerConfig{
			name:    probeContainer,
			image:   probeContainerImage,
			workdir: probeContainerWorkdir,
			user:    probeContainerUser,
			mounts:  mounts,
			devices: probeContainerDevices,
			groups:  probeContainerGroups,
		})
		if startErr != nil {
			fmt.Fprintf(&output, "[runtime] %v\n", startErr)
			trimmedOutput := redactSensitive(trimOutput(output.String()), job.SensitiveValues)
			if reportErr := reportTerminalJobStatusWithRetry(client, serverURL, job.ID, protocol.JobExecutionStatusUpdateRequest{
				AgentID:      agentID,
				Status:       protocol.JobExecutionStatusFailed,
				Error:        startErr.Error(),
				Output:       trimmedOutput,
				CacheStats:   cacheStats,
				TimestampUTC: time.Now().UTC(),
			}); reportErr != nil {
				return reportErr
			}
			slog.Error("job failed", "job_execution_id", job.ID, "error", startErr.Error())
			return nil
		}
		mountSpecs := make([]string, 0, len(mounts))
		for _, m := range mounts {
			hostPath := strings.TrimSpace(m.hostPath)
			containerPath := strings.TrimSpace(m.containerPath)
			if hostPath == "" || containerPath == "" {
				continue
			}
			mountSpecs = append(mountSpecs, hostPath+"->"+containerPath)
		}
		mountSummary := "none"
		if len(mountSpecs) > 0 {
			mountSummary = strings.Join(mountSpecs, ", ")
		}
		userSummary := strings.TrimSpace(probeContainerUser)
		if userSummary == "" {
			userSummary = "default"
		}
		deviceSummary := "none"
		if len(probeContainerDevices) > 0 {
			deviceSummary = strings.Join(probeContainerDevices, ", ")
		}
		groupSummary := "none"
		if len(probeContainerGroups) > 0 {
			groupSummary = strings.Join(probeContainerGroups, ", ")
		}
		fmt.Fprintf(&output, "[runtime] started execution container %s from %s (workdir=%s user=%s mounts=%s devices=%s groups=%s)\n", probeContainer, probeContainerImage, probeContainerWorkdir, userSummary, mountSummary, deviceSummary, groupSummary)
		defer cleanupRuntimeProbeContainer(context.Background(), probeContainer)
		execContainer = &executionContainerContext{
			name:    probeContainer,
			workdir: probeContainerWorkdir,
		}
	}
	if ensureErr := validateProbeContainerReady(runCtx, probeContainer, probeContainerImage); ensureErr != nil {
		fmt.Fprintf(&output, "[runtime] %v\n", ensureErr)
		trimmedOutput := redactSensitive(trimOutput(output.String()), job.SensitiveValues)
		if reportErr := reportTerminalJobStatusWithRetry(client, serverURL, job.ID, protocol.JobExecutionStatusUpdateRequest{
			AgentID:      agentID,
			Status:       protocol.JobExecutionStatusFailed,
			Error:        ensureErr.Error(),
			Output:       trimmedOutput,
			CacheStats:   cacheStats,
			TimestampUTC: time.Now().UTC(),
		}); reportErr != nil {
			return reportErr
		}
		slog.Error("job failed", "job_execution_id", job.ID, "error", ensureErr.Error())
		return nil
	}
	shell, err := resolveJobShell(job.RequiredCapabilities)
	if err != nil {
		return reportFailure(ctx, client, serverURL, agentID, job, nil, fmt.Sprintf("resolve job shell: %v", err), "")
	}

	runtimeCaps := collectRuntimeCapabilities(agentCapabilities, probeContainer)
	enrichRuntimeHostToolCapabilities(runtimeCaps, job.RequiredCapabilities, shell)
	if summary := runtimeProbeSummary(runtimeCaps); summary != "" {
		fmt.Fprintf(&output, "%s\n", summary)
	}
	if err := validateHostToolRequirements(job.RequiredCapabilities, runtimeCaps); err != nil {
		fmt.Fprintf(&output, "[runtime] %v\n", err)
		trimmedOutput := redactSensitive(trimOutput(output.String()), job.SensitiveValues)
		if reportErr := reportTerminalJobStatusWithRetry(client, serverURL, job.ID, protocol.JobExecutionStatusUpdateRequest{
			AgentID:             agentID,
			Status:              protocol.JobExecutionStatusFailed,
			Error:               err.Error(),
			Output:              trimmedOutput,
			CacheStats:          cacheStats,
			RuntimeCapabilities: runtimeCaps,
			TimestampUTC:        time.Now().UTC(),
		}); reportErr != nil {
			return reportErr
		}
		slog.Error("job failed", "job_execution_id", job.ID, "error", err.Error())
		return nil
	}
	if err := validateContainerToolRequirements(job.RequiredCapabilities, runtimeCaps); err != nil {
		fmt.Fprintf(&output, "[runtime] %v\n", err)
		trimmedOutput := redactSensitive(trimOutput(output.String()), job.SensitiveValues)
		if reportErr := reportTerminalJobStatusWithRetry(client, serverURL, job.ID, protocol.JobExecutionStatusUpdateRequest{
			AgentID:             agentID,
			Status:              protocol.JobExecutionStatusFailed,
			Error:               err.Error(),
			Output:              trimmedOutput,
			CacheStats:          cacheStats,
			RuntimeCapabilities: runtimeCaps,
			TimestampUTC:        time.Now().UTC(),
		}); reportErr != nil {
			return reportErr
		}
		slog.Error("job failed", "job_execution_id", job.ID, "error", err.Error())
		return nil
	}
	if err := reportJobStatus(ctx, client, serverURL, job.ID, protocol.JobExecutionStatusUpdateRequest{
		AgentID:             agentID,
		Status:              protocol.JobExecutionStatusRunning,
		Output:              redactSensitive(trimOutput(output.String()), job.SensitiveValues),
		CurrentStep:         "Preparing execution",
		RuntimeCapabilities: runtimeCaps,
		TimestampUTC:        time.Now().UTC(),
	}); err != nil {
		return fmt.Errorf("report runtime capabilities: %w", err)
	}

	traceShell := boolEnv("CIWI_AGENT_TRACE_SHELL", true)
	verboseGo := boolEnv("CIWI_AGENT_GO_BUILD_VERBOSE", true)
	if job.Metadata != nil {
		// Keep ad-hoc runs readable and avoid leaking shell traces for secret-backed jobs.
		if job.Metadata["has_secrets"] == "1" || job.Metadata["adhoc"] == "1" {
			traceShell = false
		}
	}

	fmt.Fprintf(&output, "[run] shell_trace=%t go_build_verbose=%t\n", traceShell, verboseGo)
	fmt.Fprintf(&output, "[run] shell=%s\n", shell)

	runEnv := []string(nil)
	if execContainer != nil {
		runEnv = withGoVerbose(mergeEnv(mergeEnv([]string{}, job.Env), cacheEnv), verboseGo)
	} else {
		runEnv = withGoVerbose(mergeEnv(mergeEnv(os.Environ(), job.Env), cacheEnv), verboseGo)
	}
	scriptSteps := stepPlanToScriptSteps(job.StepPlan)
	collectedSuites := make([]protocol.TestSuiteReport, 0, len(scriptSteps))
	var collectedCoverage *protocol.CoverageReport
	if len(scriptSteps) > 0 {
		fmt.Fprintf(&output, "[run] mode=stepwise steps=%d\n", len(scriptSteps))
	}
	runStart := time.Now()
	if len(scriptSteps) == 0 {
		err = runJobScript(runCtx, client, serverURL, agentID, job.ID, shell, execDir, job.Script, execContainer, runEnv, &output, "Running job script", job.SensitiveValues, traceShell)
	} else {
		for _, step := range scriptSteps {
			currentStep := formatCurrentStep(step.meta)
			slog.Info("job step started", "job_execution_id", job.ID, "current_step", currentStep)
			if step.meta.kind == "dryrun_skip" {
				fmt.Fprintf(&output, "[dry-run] skipped step: %s\n", strings.TrimSpace(step.meta.name))
			}
			events := []protocol.JobExecutionEvent{
				{
					Type: protocol.JobExecutionEventTypeStepStarted,
					Step: &protocol.JobStepPlanItem{
						Index:          step.meta.index,
						Total:          step.meta.total,
						Name:           step.meta.name,
						Kind:           step.meta.kind,
						TestName:       step.meta.testName,
						TestFormat:     step.meta.testFormat,
						TestReport:     step.meta.testReport,
						CoverageFormat: step.meta.coverageFormat,
						CoverageReport: step.meta.coverageReport,
					},
					TimestampUTC: time.Now().UTC(),
				},
			}
			if err := reportJobStatus(ctx, client, serverURL, job.ID, protocol.JobExecutionStatusUpdateRequest{
				AgentID:      agentID,
				Status:       protocol.JobExecutionStatusRunning,
				Output:       redactSensitive(trimOutput(output.String()), job.SensitiveValues),
				CurrentStep:  currentStep,
				Events:       events,
				TimestampUTC: time.Now().UTC(),
			}); err != nil {
				return fmt.Errorf("report step status: %w", err)
			}
			if step.meta.kind == "dryrun_skip" {
				continue
			}
			stepRunEnv := runEnv
			if len(step.env) > 0 {
				stepRunEnv = mergeEnv(runEnv, step.env)
			}
			stepErr := runJobScript(runCtx, client, serverURL, agentID, job.ID, shell, execDir, step.script, execContainer, stepRunEnv, &output, currentStep, job.SensitiveValues, traceShell)
			if step.meta.kind == "test" && strings.TrimSpace(step.meta.testReport) != "" {
				suite, parseErr := parseStepTestSuiteFromFile(execDir, step.meta)
				if parseErr != nil {
					fmt.Fprintf(&output, "[tests] parse_failed suite=%s path=%s err=%v\n", step.meta.testName, step.meta.testReport, parseErr)
					if stepErr == nil {
						stepErr = parseErr
					}
				} else {
					collectedSuites = append(collectedSuites, suite)
				}
			}
			if step.meta.kind == "test" && strings.TrimSpace(step.meta.coverageReport) != "" {
				coverage, parseErr := parseStepCoverageFromFile(execDir, step.meta)
				if parseErr != nil {
					fmt.Fprintf(&output, "[coverage] parse_failed suite=%s path=%s err=%v\n", step.meta.testName, step.meta.coverageReport, parseErr)
					if stepErr == nil {
						stepErr = parseErr
					}
				} else if coverage != nil {
					collectedCoverage = coverage
					fmt.Fprintf(&output, "[coverage] format=%s coverage=%.2f%%\n", coverage.Format, coverage.Percent)
				}
			}
			if stepErr != nil {
				scriptLiteral := strings.TrimSpace(step.script)
				if scriptLiteral != "" {
					fmt.Fprintf(&output, "[run] failed step script literal:\n%s\n", scriptLiteral)
				}
				if code := exitCodeFromErr(stepErr); code != nil {
					fmt.Fprintf(&output, "[run] step failed: %s (exit=%d)\n", currentStep, *code)
				} else {
					fmt.Fprintf(&output, "[run] step failed: %s (%v)\n", currentStep, stepErr)
				}
				err = fmt.Errorf("%s: %w", currentStep, stepErr)
				break
			}
		}
	}
	duration := time.Since(runStart).Round(time.Millisecond)
	fmt.Fprintf(&output, "\n[run] duration=%s\n", duration)

	if len(job.ArtifactGlobs) > 0 {
		fmt.Fprintf(&output, "[artifacts] collecting...\n")
		note, uploadErr := collectAndUploadArtifacts(ctx, client, serverURL, agentID, job.ID, execDir, job.ArtifactGlobs, func(msg string) {
			fmt.Fprintf(&output, "%s\n", msg)
		})
		if note != "" {
			output.WriteString(note)
			if !strings.HasSuffix(note, "\n") {
				output.WriteString("\n")
			}
		}
		if uploadErr != nil {
			fmt.Fprintf(&output, "[artifacts] upload_failed=%v\n", uploadErr)
			trimmedOutput := redactSensitive(trimOutput(output.String()), job.SensitiveValues)
			failMsg := "artifact upload failed: " + uploadErr.Error()
			cacheStats = refreshCacheStats()
			if reportErr := reportTerminalJobStatusWithRetry(client, serverURL, job.ID, protocol.JobExecutionStatusUpdateRequest{
				AgentID:             agentID,
				Status:              protocol.JobExecutionStatusFailed,
				Error:               failMsg,
				Output:              trimmedOutput,
				CacheStats:          cacheStats,
				RuntimeCapabilities: runtimeCaps,
				TimestampUTC:        time.Now().UTC(),
			}); reportErr != nil {
				return reportErr
			}
			slog.Error("job failed", "job_execution_id", job.ID, "error", failMsg)
			return nil
		}
	}

	trimmedOutput := redactSensitive(trimOutput(output.String()), job.SensitiveValues)
	testReport := protocol.JobExecutionTestReport{Suites: collectedSuites, Coverage: collectedCoverage}
	for _, s := range collectedSuites {
		testReport.Total += s.Total
		testReport.Passed += s.Passed
		testReport.Failed += s.Failed
		testReport.Skipped += s.Skipped
	}
	if testReport.Total > 0 || testReport.Coverage != nil {
		if err := uploadTestReport(ctx, client, serverURL, agentID, job.ID, testReport); err != nil {
			fmt.Fprintf(&output, "[tests] upload_failed=%v\n", err)
		} else {
			fmt.Fprintf(&output, "%s\n", testReportSummary(testReport))
		}
		if err == nil && testReport.Failed > 0 {
			err = fmt.Errorf("test report contains failures: failed=%d", testReport.Failed)
		}
		trimmedOutput = redactSensitive(trimOutput(output.String()), job.SensitiveValues)
	}

	if err == nil {
		exitCode := 0
		cacheStats = refreshCacheStats()
		if reportErr := reportTerminalJobStatusWithRetry(client, serverURL, job.ID, protocol.JobExecutionStatusUpdateRequest{
			AgentID:             agentID,
			Status:              protocol.JobExecutionStatusSucceeded,
			ExitCode:            &exitCode,
			Output:              trimmedOutput,
			CacheStats:          cacheStats,
			RuntimeCapabilities: runtimeCaps,
			CurrentStep:         "",
			TimestampUTC:        time.Now().UTC(),
		}); reportErr != nil {
			return fmt.Errorf("report succeeded status: %w", reportErr)
		}
		slog.Info("job succeeded", "job_execution_id", job.ID)
		return nil
	}

	exitCode := exitCodeFromErr(err)
	failMsg := err.Error()
	if runCtx.Err() == context.DeadlineExceeded {
		failMsg = "job timed out"
	}
	cacheStats = refreshCacheStats()
	if reportErr := reportTerminalJobStatusWithRetry(client, serverURL, job.ID, protocol.JobExecutionStatusUpdateRequest{
		AgentID:             agentID,
		Status:              protocol.JobExecutionStatusFailed,
		ExitCode:            exitCode,
		Error:               failMsg,
		Output:              trimmedOutput,
		CacheStats:          cacheStats,
		RuntimeCapabilities: runtimeCaps,
		CurrentStep:         "",
		TimestampUTC:        time.Now().UTC(),
	}); reportErr != nil {
		return reportErr
	}
	slog.Error("job failed", "job_execution_id", job.ID, "exit_code", exitCode, "error", failMsg)
	return nil
}

func monitorServerTerminalJobState(
	ctx context.Context,
	client *http.Client,
	serverURL, agentID, jobID string,
	output *syncBuffer,
	cancel context.CancelFunc,
) func() {
	ticker := time.NewTicker(500 * time.Millisecond)
	done := make(chan struct{})
	stopCh := make(chan struct{})

	go func() {
		defer close(done)
		check := func() bool {
			state, err := getJobExecutionState(ctx, client, serverURL, jobID)
			if err != nil {
				return false
			}
			status := protocol.NormalizeJobExecutionStatus(state.Status)
			if !protocol.IsTerminalJobExecutionStatus(status) {
				return false
			}
			msg := "job marked " + status + " on server"
			if reason := strings.TrimSpace(state.Error); reason != "" {
				msg += ": " + reason
			}
			if output != nil {
				_, _ = output.WriteString("[control] " + msg + "\n")
			}
			slog.Warn("server marked job terminal during execution; cancelling local process", "job_execution_id", jobID, "agent_id", agentID, "status", status)
			cancel()
			return true
		}
		if check() {
			return
		}
		for {
			select {
			case <-stopCh:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				if check() {
					return
				}
			}
		}
	}()

	return func() {
		ticker.Stop()
		close(stopCh)
		<-done
	}
}

func parseStepTestSuiteFromFile(execDir string, meta stepMarkerMeta) (protocol.TestSuiteReport, error) {
	path := strings.TrimSpace(meta.testReport)
	if path == "" {
		return protocol.TestSuiteReport{}, fmt.Errorf("test report path is required")
	}
	format := strings.TrimSpace(meta.testFormat)
	if format == "" {
		format = "go-test-json"
	}
	if format == "junit" {
		format = "junit-xml"
	}
	suiteName := strings.TrimSpace(meta.testName)
	if suiteName == "" {
		suiteName = strings.TrimSpace(meta.name)
	}

	full := filepath.Join(execDir, filepath.FromSlash(path))
	raw, err := os.ReadFile(full)
	if err != nil {
		return protocol.TestSuiteReport{}, fmt.Errorf("read report %q: %w", path, err)
	}
	lines := strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n")
	var suite protocol.TestSuiteReport
	switch format {
	case "go-test-json":
		suite = parseGoTestJSONSuite(suiteName, lines)
	case "junit-xml":
		suite = parseJUnitXMLSuite(suiteName, lines)
	default:
		return protocol.TestSuiteReport{}, fmt.Errorf("unsupported test format %q", format)
	}
	if suite.Format == "" {
		suite.Format = format
	}
	if strings.TrimSpace(suite.Name) == "" {
		suite.Name = suiteName
	}
	return suite, nil
}

type jobScriptStep struct {
	meta   stepMarkerMeta
	script string
	env    map[string]string
}

func stepPlanToScriptSteps(plan []protocol.JobStepPlanItem) []jobScriptStep {
	if len(plan) == 0 {
		return nil
	}
	steps := make([]jobScriptStep, 0, len(plan))
	total := len(plan)
	for i, step := range plan {
		script := step.Script
		kind := strings.TrimSpace(step.Kind)
		if strings.TrimSpace(script) == "" && kind != "dryrun_skip" {
			continue
		}
		index := step.Index
		if index <= 0 {
			index = i + 1
		}
		itemTotal := step.Total
		if itemTotal <= 0 {
			itemTotal = total
		}
		name := strings.TrimSpace(step.Name)
		if name == "" {
			name = fmt.Sprintf("step_%d", index)
		}
		steps = append(steps, jobScriptStep{
			meta: stepMarkerMeta{
				index:          index,
				total:          itemTotal,
				name:           name,
				kind:           kind,
				testName:       strings.TrimSpace(step.TestName),
				testFormat:     strings.TrimSpace(step.TestFormat),
				testReport:     strings.TrimSpace(step.TestReport),
				coverageFormat: strings.TrimSpace(step.CoverageFormat),
				coverageReport: strings.TrimSpace(step.CoverageReport),
			},
			script: script,
			env:    cloneMap(step.Env),
		})
	}
	if len(steps) == 0 {
		return nil
	}
	return steps
}
