package agent

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type unavailableLogConsole struct{}

func (unavailableLogConsole) Write([]byte) (int, error) {
	return 0, errors.New("invalid Windows service stderr handle")
}

func TestAgentFileLoggingWithoutConsole(t *testing.T) {
	path := filepath.Join(t.TempDir(), "logs", "agent.log")
	logger, closer, err := newAgentFileLogger(path, unavailableLogConsole{}, "debug")
	if err != nil {
		t.Fatal(err)
	}
	logger.Debug("job step started", "job_execution_id", "job-123")
	logger.Error("execute job failed", "error", "connection reset")
	if err := closer.Close(); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"level=DEBUG", "job_execution_id=job-123", "execute job failed", "connection reset"} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("missing %q in log: %s", want, raw)
		}
	}
}

func TestAgentFileLoggingLevelAndConsole(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent.log")
	var console bytes.Buffer
	logger, closer, err := newAgentFileLogger(path, &console, "warn")
	if err != nil {
		t.Fatal(err)
	}
	logger.Info("hidden")
	logger.Warn("visible")
	if err := closer.Close(); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != console.String() || strings.Contains(string(raw), "hidden") || !strings.Contains(string(raw), "visible") {
		t.Fatalf("unexpected file/console logs: file=%q console=%q", raw, console.String())
	}
}

func TestAgentLogAppendAndRotation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent.log")
	if err := os.WriteFile(path, []byte("old\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	w, err := openAgentLog(path, 8, 3)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = w.Close() })
	if _, err := w.Write([]byte("one\n")); err != nil {
		t.Fatal(err)
	}
	if raw, err := os.ReadFile(path); err != nil || string(raw) != "old\none\n" {
		t.Fatalf("existing log not appended: %q, %v", raw, err)
	}
	for _, line := range []string{"second\n", "third\n", "fourth\n", "fifth\n"} {
		if _, err := w.Write([]byte(line)); err != nil {
			t.Fatal(err)
		}
	}
	for suffix, want := range map[string]string{"": "fifth\n", ".1": "fourth\n", ".2": "third\n", ".3": "second\n"} {
		if raw, err := os.ReadFile(path + suffix); err != nil || string(raw) != want {
			t.Errorf("log %q = %q, %v; want %q", suffix, raw, err, want)
		}
	}
	if _, err := os.Stat(path + ".4"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unexpected fourth backup: %v", err)
	}
}

func TestAgentFileLoggingInvalidPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := newAgentFileLogger(filepath.Join(path, "agent.log"), nil, ""); err == nil {
		t.Fatal("expected error when log directory is a file")
	}
}
