package linuxupdater

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/izzyreal/ciwi/internal/store"
)

func TestRunApplyStagedWritesFailedStateOnApplyError(t *testing.T) {
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "ciwi.db")
	t.Setenv("CIWI_DB_PATH", dbPath)

	oldGeteuid := geteuidFn
	oldExecutable := executablePathFn
	oldApply := applyStagedUpdateFn
	t.Cleanup(func() {
		geteuidFn = oldGeteuid
		executablePathFn = oldExecutable
		applyStagedUpdateFn = oldApply
	})

	geteuidFn = func() int { return 0 }
	executablePathFn = func() (string, error) { return filepath.Join(tmp, "ciwi"), nil }
	applyStagedUpdateFn = func(manifestPath, targetPath, serviceName, systemctlPath string) (stagedManifest, error) {
		return stagedManifest{}, os.ErrPermission
	}

	err := RunApplyStaged([]string{"--manifest", filepath.Join(tmp, "pending.json"), "--service", "ciwi.service"})
	if err == nil {
		t.Fatalf("expected RunApplyStaged to fail")
	}

	st, openErr := store.Open(dbPath)
	if openErr != nil {
		t.Fatalf("open state db: %v", openErr)
	}
	defer st.Close()
	status, found, getErr := st.GetAppState("update_last_apply_status")
	if getErr != nil {
		t.Fatalf("get update_last_apply_status: %v", getErr)
	}
	if !found || status != "failed" {
		t.Fatalf("expected failed apply status, found=%v value=%q", found, status)
	}
	msg, found, getErr := st.GetAppState("update_message")
	if getErr != nil {
		t.Fatalf("get update_message: %v", getErr)
	}
	if !found || !strings.Contains(msg, "linux updater failed") {
		t.Fatalf("expected failure message, found=%v value=%q", found, msg)
	}
}

func TestRunApplyStagedWritesSuccessState(t *testing.T) {
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "ciwi.db")
	t.Setenv("CIWI_DB_PATH", dbPath)

	oldGeteuid := geteuidFn
	oldExecutable := executablePathFn
	oldApply := applyStagedUpdateFn
	t.Cleanup(func() {
		geteuidFn = oldGeteuid
		executablePathFn = oldExecutable
		applyStagedUpdateFn = oldApply
	})

	geteuidFn = func() int { return 0 }
	executablePathFn = func() (string, error) { return filepath.Join(tmp, "ciwi"), nil }
	applyStagedUpdateFn = func(manifestPath, targetPath, serviceName, systemctlPath string) (stagedManifest, error) {
		return stagedManifest{
			TargetVersion: "v1.2.3",
			AssetName:     "ciwi-linux-amd64",
		}, nil
	}

	if err := RunApplyStaged([]string{"--manifest", filepath.Join(tmp, "pending.json"), "--service", "ciwi.service"}); err != nil {
		t.Fatalf("RunApplyStaged: %v", err)
	}

	st, openErr := store.Open(dbPath)
	if openErr != nil {
		t.Fatalf("open state db: %v", openErr)
	}
	defer st.Close()
	status, found, getErr := st.GetAppState("update_last_apply_status")
	if getErr != nil {
		t.Fatalf("get update_last_apply_status: %v", getErr)
	}
	if !found || status != "success" {
		t.Fatalf("expected success apply status, found=%v value=%q", found, status)
	}
	latest, found, getErr := st.GetAppState("update_latest_version")
	if getErr != nil {
		t.Fatalf("get update_latest_version: %v", getErr)
	}
	if !found || latest != "v1.2.3" {
		t.Fatalf("expected latest version persisted, found=%v value=%q", found, latest)
	}
}
