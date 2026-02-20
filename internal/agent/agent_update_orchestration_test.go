package agent

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSelfUpdateAndRestartChecksumFailureStopsBeforeHelper(t *testing.T) {
	restore := stubSelfUpdateOrchestration(t)
	defer restore()

	calledStartHelper := false
	agentFetchReleaseAssetsForTagFn = func(context.Context, string, string, string, string, string) (githubReleaseAsset, githubReleaseAsset, error) {
		return githubReleaseAsset{Name: expectedAssetName(runtime.GOOS, runtime.GOARCH), URL: "https://example.invalid/asset"},
			githubReleaseAsset{Name: "ciwi-checksums.txt", URL: "https://example.invalid/sums"}, nil
	}
	agentDownloadUpdateAssetFn = func(context.Context, string, string) (string, error) {
		p := filepath.Join(t.TempDir(), "ciwi-new"+exeExt())
		if err := os.WriteFile(p, []byte("bin"), 0o755); err != nil {
			t.Fatalf("write staged binary: %v", err)
		}
		return p, nil
	}
	agentDownloadTextAssetFn = func(context.Context, string) (string, error) { return "dummy", nil }
	agentVerifyFileSHA256Fn = func(string, string, string) error { return errors.New("bad checksum") }
	agentStartUpdateHelperFn = func(string, string, string, int, []string, string) error {
		calledStartHelper = true
		return nil
	}

	err := selfUpdateAndRestart(context.Background(), "v999.9.1", "izzyreal/ciwi", "https://api.github.com", []string{"serve"})
	if err == nil || !strings.Contains(err.Error(), "checksum verification failed") {
		t.Fatalf("expected checksum verification failure, got %v", err)
	}
	if calledStartHelper {
		t.Fatalf("helper should not be started on checksum failure")
	}
}

func TestSelfUpdateAndRestartHelperStartFailure(t *testing.T) {
	restore := stubSelfUpdateOrchestration(t)
	defer restore()

	agentFetchReleaseAssetsForTagFn = func(context.Context, string, string, string, string, string) (githubReleaseAsset, githubReleaseAsset, error) {
		return githubReleaseAsset{Name: expectedAssetName(runtime.GOOS, runtime.GOARCH), URL: "https://example.invalid/asset"},
			githubReleaseAsset{}, nil
	}
	agentDownloadUpdateAssetFn = func(context.Context, string, string) (string, error) {
		p := filepath.Join(t.TempDir(), "ciwi-new"+exeExt())
		if err := os.WriteFile(p, []byte("bin"), 0o755); err != nil {
			t.Fatalf("write staged binary: %v", err)
		}
		return p, nil
	}
	agentStartUpdateHelperFn = func(string, string, string, int, []string, string) error {
		return errors.New("helper start failed")
	}

	err := selfUpdateAndRestart(context.Background(), "v999.9.2", "izzyreal/ciwi", "https://api.github.com", []string{"serve"})
	if err == nil || !strings.Contains(err.Error(), "start update helper: helper start failed") {
		t.Fatalf("expected helper start failure, got %v", err)
	}
}

func TestSelfUpdateAndRestartSuccessSchedulesExit(t *testing.T) {
	restore := stubSelfUpdateOrchestration(t)
	defer restore()

	exitScheduled := 0
	startArgs := []string{}
	agentFetchReleaseAssetsForTagFn = func(context.Context, string, string, string, string, string) (githubReleaseAsset, githubReleaseAsset, error) {
		return githubReleaseAsset{Name: expectedAssetName(runtime.GOOS, runtime.GOARCH), URL: "https://example.invalid/asset"},
			githubReleaseAsset{}, nil
	}
	agentDownloadUpdateAssetFn = func(context.Context, string, string) (string, error) {
		p := filepath.Join(t.TempDir(), "ciwi-new"+exeExt())
		if err := os.WriteFile(p, []byte("bin"), 0o755); err != nil {
			t.Fatalf("write staged binary: %v", err)
		}
		return p, nil
	}
	agentStartUpdateHelperFn = func(_ string, _ string, _ string, _ int, args []string, _ string) error {
		startArgs = append([]string{}, args...)
		return nil
	}
	agentScheduleExitAfterUpdateFn = func() { exitScheduled++ }

	err := selfUpdateAndRestart(context.Background(), "v999.9.3", "izzyreal/ciwi", "https://api.github.com", []string{"serve", "--foo"})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if exitScheduled != 1 {
		t.Fatalf("expected one scheduled exit, got %d", exitScheduled)
	}
	if len(startArgs) != 2 || startArgs[0] != "serve" || startArgs[1] != "--foo" {
		t.Fatalf("unexpected restart args passed to helper: %#v", startArgs)
	}
}

func TestSelfUpdateAndRestartDarwinStagingBranch(t *testing.T) {
	restore := stubSelfUpdateOrchestration(t)
	defer restore()

	staged := 0
	helperStarts := 0
	agentFetchReleaseAssetsForTagFn = func(context.Context, string, string, string, string, string) (githubReleaseAsset, githubReleaseAsset, error) {
		return githubReleaseAsset{Name: expectedAssetName(runtime.GOOS, runtime.GOARCH), URL: "https://example.invalid/asset"},
			githubReleaseAsset{}, nil
	}
	agentDownloadUpdateAssetFn = func(context.Context, string, string) (string, error) {
		p := filepath.Join(t.TempDir(), "ciwi-new"+exeExt())
		if err := os.WriteFile(p, []byte("bin"), 0o755); err != nil {
			t.Fatalf("write staged binary: %v", err)
		}
		return p, nil
	}
	agentHasDarwinUpdaterConfigFn = func() bool { return true }
	agentUpdateRuntimeGOOS = "darwin"
	agentStageDarwinUpdaterFn = func(string, string, string, string) error {
		staged++
		return nil
	}
	agentStartUpdateHelperFn = func(string, string, string, int, []string, string) error {
		helperStarts++
		return nil
	}

	err := selfUpdateAndRestart(context.Background(), "v999.9.4", "izzyreal/ciwi", "https://api.github.com", []string{"serve"})
	if err != nil {
		t.Fatalf("expected darwin staging success, got %v", err)
	}
	if staged != 1 {
		t.Fatalf("expected staging invocation once, got %d", staged)
	}
	if helperStarts != 0 {
		t.Fatalf("helper should not be started on darwin staging path")
	}
}

func stubSelfUpdateOrchestration(t *testing.T) func() {
	t.Helper()
	origExecutable := agentExecutablePathFn
	origRuntimeGOOS := agentUpdateRuntimeGOOS
	origAbs := agentAbsPathFn
	origReason := agentSelfUpdateServiceReasonFn
	origFetch := agentFetchReleaseAssetsForTagFn
	origDownloadAsset := agentDownloadUpdateAssetFn
	origDownloadText := agentDownloadTextAssetFn
	origVerify := agentVerifyFileSHA256Fn
	origHasDarwin := agentHasDarwinUpdaterConfigFn
	origStageDarwin := agentStageDarwinUpdaterFn
	origCopy := agentCopyFileFn
	origWinSvc := agentWindowsServiceInfoFn
	origStartHelper := agentStartUpdateHelperFn
	origPID := agentPIDFn
	origExit := agentScheduleExitAfterUpdateFn

	agentExecutablePathFn = func() (string, error) { return "/opt/ciwi/ciwi-agent", nil }
	agentUpdateRuntimeGOOS = runtime.GOOS
	agentAbsPathFn = func(path string) (string, error) { return path, nil }
	agentSelfUpdateServiceReasonFn = func() string { return "" }
	agentFetchReleaseAssetsForTagFn = func(context.Context, string, string, string, string, string) (githubReleaseAsset, githubReleaseAsset, error) {
		return githubReleaseAsset{}, githubReleaseAsset{}, errors.New("not stubbed")
	}
	agentDownloadUpdateAssetFn = func(context.Context, string, string) (string, error) { return "", errors.New("not stubbed") }
	agentDownloadTextAssetFn = func(context.Context, string) (string, error) { return "", errors.New("not stubbed") }
	agentVerifyFileSHA256Fn = func(string, string, string) error { return nil }
	agentHasDarwinUpdaterConfigFn = func() bool { return false }
	agentStageDarwinUpdaterFn = func(string, string, string, string) error { return nil }
	agentCopyFileFn = func(string, string, os.FileMode) error { return nil }
	agentWindowsServiceInfoFn = func() (bool, string) { return false, "" }
	agentStartUpdateHelperFn = func(string, string, string, int, []string, string) error { return nil }
	agentPIDFn = func() int { return 4242 }
	agentScheduleExitAfterUpdateFn = func() {}

	return func() {
		agentExecutablePathFn = origExecutable
		agentUpdateRuntimeGOOS = origRuntimeGOOS
		agentAbsPathFn = origAbs
		agentSelfUpdateServiceReasonFn = origReason
		agentFetchReleaseAssetsForTagFn = origFetch
		agentDownloadUpdateAssetFn = origDownloadAsset
		agentDownloadTextAssetFn = origDownloadText
		agentVerifyFileSHA256Fn = origVerify
		agentHasDarwinUpdaterConfigFn = origHasDarwin
		agentStageDarwinUpdaterFn = origStageDarwin
		agentCopyFileFn = origCopy
		agentWindowsServiceInfoFn = origWinSvc
		agentStartUpdateHelperFn = origStartHelper
		agentPIDFn = origPID
		agentScheduleExitAfterUpdateFn = origExit
	}
}
