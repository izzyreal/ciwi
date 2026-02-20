package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/izzyreal/ciwi/internal/agent"
	"github.com/izzyreal/ciwi/internal/darwinupdater"
	"github.com/izzyreal/ciwi/internal/linuxupdater"
	"github.com/izzyreal/ciwi/internal/server"
	"github.com/izzyreal/ciwi/internal/updatehelper"
)

type commandRunners struct {
	runServer                 func(context.Context) error
	runAgent                  func(context.Context) error
	runAllInOne               func(context.Context) error
	runUpdateHelper           func([]string) error
	runApplyStagedUpdate      func([]string) error
	runApplyStagedAgentUpdate func([]string) error
}

func defaultCommandRunners() commandRunners {
	return commandRunners{
		runServer:                 server.Run,
		runAgent:                  agent.Run,
		runAllInOne:               runAllInOne,
		runUpdateHelper:           updatehelper.Run,
		runApplyStagedUpdate:      linuxupdater.RunApplyStaged,
		runApplyStagedAgentUpdate: darwinupdater.RunApplyStagedAgent,
	}
}

func main() {
	initLogging()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if exitCode := runWith(os.Args, os.Stderr, ctx, defaultCommandRunners()); exitCode != 0 {
		os.Exit(exitCode)
	}
}

func initLogging() {
	level := new(slog.LevelVar)
	switch strings.ToLower(strings.TrimSpace(os.Getenv("CIWI_LOG_LEVEL"))) {
	case "debug":
		level.Set(slog.LevelDebug)
	case "warn", "warning":
		level.Set(slog.LevelWarn)
	case "error":
		level.Set(slog.LevelError)
	default:
		level.Set(slog.LevelInfo)
	}
	handler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level:     level,
		AddSource: false,
	})
	slog.SetDefault(slog.New(handler))
}

func runAllInOne(ctx context.Context) error {
	return runAllInOneWith(ctx, server.Run, agent.Run)
}

func runAllInOneWith(ctx context.Context, runServer func(context.Context) error, runAgent func(context.Context) error) error {
	errCh := make(chan error, 2)

	go func() {
		errCh <- runServer(ctx)
	}()
	go func() {
		errCh <- runAgent(ctx)
	}()

	return <-errCh
}

func runWith(args []string, stderr io.Writer, ctx context.Context, runners commandRunners) int {
	if len(args) < 2 {
		usageTo(stderr)
		return 2
	}

	var err error
	switch args[1] {
	case "server":
		err = runners.runServer(ctx)
	case "agent":
		err = runners.runAgent(ctx)
	case "all-in-one":
		err = runners.runAllInOne(ctx)
	case "update-helper":
		err = runners.runUpdateHelper(args[2:])
	case "apply-staged-update":
		err = runners.runApplyStagedUpdate(args[2:])
	case "apply-staged-agent-update":
		err = runners.runApplyStagedAgentUpdate(args[2:])
	case "help", "-h", "--help":
		usageTo(stderr)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown command: %s\n\n", args[1])
		usageTo(stderr)
		return 2
	}
	if err != nil {
		fmt.Fprintf(stderr, "ciwi: %v\n", err)
		return 1
	}
	return 0
}

func usage() {
	usageTo(os.Stderr)
}

func usageTo(w io.Writer) {
	fmt.Fprintf(w, `ciwi - lightweight CI/CD

Usage:
  ciwi <command>

Commands:
  server      Run the scheduler/API server
  agent       Run an execution agent
  all-in-one  Run server and agent in one process (dev mode)
  update-helper  Internal mode used by self-update
  apply-staged-update  Internal mode used by Linux server updater
  apply-staged-agent-update  Internal mode used by macOS agent updater
  help        Show this help
`)
}
