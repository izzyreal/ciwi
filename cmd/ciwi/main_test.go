package main

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"
)

func TestInitLoggingLevelFromEnv(t *testing.T) {
	cases := []struct {
		name    string
		env     string
		debugOn bool
		infoOn  bool
		warnOn  bool
		errorOn bool
	}{
		{name: "debug", env: "debug", debugOn: true, infoOn: true, warnOn: true, errorOn: true},
		{name: "warn", env: "warn", debugOn: false, infoOn: false, warnOn: true, errorOn: true},
		{name: "error", env: "error", debugOn: false, infoOn: false, warnOn: false, errorOn: true},
		{name: "default", env: "", debugOn: false, infoOn: true, warnOn: true, errorOn: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("CIWI_LOG_LEVEL", tc.env)
			initLogging()
			h := slog.Default().Handler()
			ctx := context.Background()
			if got := h.Enabled(ctx, slog.LevelDebug); got != tc.debugOn {
				t.Fatalf("debug enabled=%v want %v", got, tc.debugOn)
			}
			if got := h.Enabled(ctx, slog.LevelInfo); got != tc.infoOn {
				t.Fatalf("info enabled=%v want %v", got, tc.infoOn)
			}
			if got := h.Enabled(ctx, slog.LevelWarn); got != tc.warnOn {
				t.Fatalf("warn enabled=%v want %v", got, tc.warnOn)
			}
			if got := h.Enabled(ctx, slog.LevelError); got != tc.errorOn {
				t.Fatalf("error enabled=%v want %v", got, tc.errorOn)
			}
		})
	}
}

func TestUsageWritesExpectedText(t *testing.T) {
	out := captureStderr(t, usage)
	if !strings.Contains(out, "ciwi - lightweight CI/CD") {
		t.Fatalf("missing usage title, got: %q", out)
	}
	if !strings.Contains(out, "apply-staged-update") || !strings.Contains(out, "apply-staged-agent-update") {
		t.Fatalf("missing updater commands in usage: %q", out)
	}
}

func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stderr = w
	done := make(chan string, 1)
	go func() {
		raw, _ := io.ReadAll(r)
		done <- string(raw)
	}()
	fn()
	_ = w.Close()
	os.Stderr = orig
	return <-done
}
