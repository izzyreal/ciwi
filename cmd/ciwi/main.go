package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/izzyreal/ciwi/internal/agent"
	"github.com/izzyreal/ciwi/internal/server"
	"github.com/izzyreal/ciwi/internal/updatehelper"
)

func main() {
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
  help        Show this help
`)
}
