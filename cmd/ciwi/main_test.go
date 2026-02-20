package main

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"strings"
	"testing"
)

func TestRunWithCommandDispatchAndExitCodes(t *testing.T) {
	ctx := context.Background()

	type called struct {
		server, agent, allInOne, helper, staged, stagedAgent bool
	}
	mk := func(c *called, err error) commandRunners {
		return commandRunners{
			runServer: func(context.Context) error { c.server = true; return err },
			runAgent:  func(context.Context) error { c.agent = true; return err },
			runAllInOne: func(context.Context) error {
				c.allInOne = true
				return err
			},
			runUpdateHelper: func([]string) error { c.helper = true; return err },
			runApplyStagedUpdate: func([]string) error {
				c.staged = true
				return err
			},
			runApplyStagedAgentUpdate: func([]string) error {
				c.stagedAgent = true
				return err
			},
		}
	}

	t.Run("missing command", func(t *testing.T) {
		var out bytes.Buffer
		code := runWith([]string{"ciwi"}, &out, ctx, mk(&called{}, nil))
		if code != 2 {
			t.Fatalf("expected exit code 2, got %d", code)
		}
		if !strings.Contains(out.String(), "Usage:") {
			t.Fatalf("expected usage output, got %q", out.String())
		}
	})

	t.Run("unknown command", func(t *testing.T) {
		var out bytes.Buffer
		code := runWith([]string{"ciwi", "nope"}, &out, ctx, mk(&called{}, nil))
		if code != 2 {
			t.Fatalf("expected exit code 2, got %d", code)
		}
		if !strings.Contains(out.String(), "unknown command: nope") {
			t.Fatalf("expected unknown command output, got %q", out.String())
		}
	})

	t.Run("help", func(t *testing.T) {
		var out bytes.Buffer
		code := runWith([]string{"ciwi", "--help"}, &out, ctx, mk(&called{}, nil))
		if code != 0 {
			t.Fatalf("expected exit code 0, got %d", code)
		}
		if !strings.Contains(out.String(), "lightweight CI/CD") {
			t.Fatalf("expected help output, got %q", out.String())
		}
	})

	t.Run("server dispatch", func(t *testing.T) {
		var out bytes.Buffer
		c := &called{}
		code := runWith([]string{"ciwi", "server"}, &out, ctx, mk(c, nil))
		if code != 0 || !c.server || c.agent || c.allInOne || c.helper || c.staged || c.stagedAgent {
			t.Fatalf("unexpected dispatch result code=%d called=%+v", code, c)
		}
	})

	t.Run("agent dispatch", func(t *testing.T) {
		var out bytes.Buffer
		c := &called{}
		code := runWith([]string{"ciwi", "agent"}, &out, ctx, mk(c, nil))
		if code != 0 || !c.agent {
			t.Fatalf("unexpected dispatch result code=%d called=%+v", code, c)
		}
	})

	t.Run("all-in-one dispatch", func(t *testing.T) {
		var out bytes.Buffer
		c := &called{}
		code := runWith([]string{"ciwi", "all-in-one"}, &out, ctx, mk(c, nil))
		if code != 0 || !c.allInOne {
			t.Fatalf("unexpected dispatch result code=%d called=%+v", code, c)
		}
	})

	t.Run("internal modes dispatch", func(t *testing.T) {
		var out bytes.Buffer
		c := &called{}
		code := runWith([]string{"ciwi", "update-helper", "--x"}, &out, ctx, mk(c, nil))
		if code != 0 || !c.helper {
			t.Fatalf("unexpected update-helper dispatch result code=%d called=%+v", code, c)
		}

		c = &called{}
		code = runWith([]string{"ciwi", "apply-staged-update"}, &out, ctx, mk(c, nil))
		if code != 0 || !c.staged {
			t.Fatalf("unexpected apply-staged-update dispatch result code=%d called=%+v", code, c)
		}

		c = &called{}
		code = runWith([]string{"ciwi", "apply-staged-agent-update"}, &out, ctx, mk(c, nil))
		if code != 0 || !c.stagedAgent {
			t.Fatalf("unexpected apply-staged-agent-update dispatch result code=%d called=%+v", code, c)
		}
	})

	t.Run("command error maps to exit 1", func(t *testing.T) {
		var out bytes.Buffer
		code := runWith([]string{"ciwi", "server"}, &out, ctx, mk(&called{}, errors.New("boom")))
		if code != 1 {
			t.Fatalf("expected exit code 1, got %d", code)
		}
		if !strings.Contains(out.String(), "ciwi: boom") {
			t.Fatalf("expected error output, got %q", out.String())
		}
	})
}

func TestRunAllInOneWithReturnsFirstError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errExpected := errors.New("server failed")
	got := runAllInOneWith(
		ctx,
		func(context.Context) error { return errExpected },
		func(context.Context) error {
			<-ctx.Done()
			return ctx.Err()
		},
	)
	if !errors.Is(got, errExpected) {
		t.Fatalf("expected first returned error, got %v", got)
	}
}

func TestDefaultCommandRunnersAndHelpers(t *testing.T) {
	runners := defaultCommandRunners()
	if runners.runServer == nil || runners.runAgent == nil || runners.runAllInOne == nil ||
		runners.runUpdateHelper == nil || runners.runApplyStagedUpdate == nil || runners.runApplyStagedAgentUpdate == nil {
		t.Fatalf("expected all default command runners to be wired: %+v", runners)
	}

	t.Setenv("CIWI_LOG_LEVEL", "debug")
	initLogging()
	if slog.Default() == nil {
		t.Fatalf("expected slog default logger to be set")
	}
	t.Setenv("CIWI_LOG_LEVEL", "warn")
	initLogging()
	t.Setenv("CIWI_LOG_LEVEL", "error")
	initLogging()
	t.Setenv("CIWI_LOG_LEVEL", "unknown")
	initLogging()

	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stderr = w
	usage()
	_ = w.Close()
	os.Stderr = old
	out := make([]byte, 4096)
	n, _ := r.Read(out)
	_ = r.Close()
	if !strings.Contains(string(out[:n]), "Usage:") {
		t.Fatalf("expected usage output, got %q", string(out[:n]))
	}
}
