package main

import (
	"context"
	"fmt"
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

func main() {
	initLogging()

	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var err error
	switch os.Args[1] {
	case "server":
		err = server.Run(ctx)
	case "agent":
		err = agent.Run(ctx)
	case "all-in-one":
		err = runAllInOne(ctx)
	case "update-helper":
		err = updatehelper.Run(os.Args[2:])
	case "apply-staged-update":
		err = linuxupdater.RunApplyStaged(os.Args[2:])
	case "apply-staged-agent-update":
		err = darwinupdater.RunApplyStagedAgent(os.Args[2:])
	case "help", "-h", "--help":
		usage()
		return
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "ciwi: %v\n", err)
		os.Exit(1)
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
	errCh := make(chan error, 2)

	go func() {
		errCh <- server.Run(ctx)
	}()
	go func() {
		errCh <- agent.Run(ctx)
	}()

	return <-errCh
}

func usage() {
	fmt.Fprintf(os.Stderr, `ciwi - lightweight CI/CD

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
