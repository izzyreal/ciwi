//go:build windows

package agent

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"golang.org/x/sys/windows/svc"
)

var (
	serviceStateMu sync.RWMutex
	serviceActive  bool
	serviceName    string
)

func runAsWindowsServiceIfNeeded(runFn func(context.Context) error) (bool, error) {
	isService, err := svc.IsWindowsService()
	if err != nil {
		return false, fmt.Errorf("detect windows service context: %w", err)
	}
	if !isService {
		return false, nil
	}

	name := strings.TrimSpace(envOrDefault("CIWI_WINDOWS_SERVICE_NAME", "ciwi-agent"))
	if name == "" {
		name = "ciwi-agent"
	}
	setWindowsServiceInfo(true, name)
	defer setWindowsServiceInfo(false, "")

	if err := svc.Run(name, &agentWindowsService{runFn: runFn}); err != nil {
		return true, fmt.Errorf("run windows service %q: %w", name, err)
	}
	return true, nil
}

func windowsServiceInfo() (bool, string) {
	serviceStateMu.RLock()
	defer serviceStateMu.RUnlock()
	return serviceActive, serviceName
}

func setWindowsServiceInfo(active bool, name string) {
	serviceStateMu.Lock()
	defer serviceStateMu.Unlock()
	serviceActive = active
	serviceName = name
}

type agentWindowsService struct {
	runFn func(context.Context) error
}

func (s *agentWindowsService) Execute(_ []string, req <-chan svc.ChangeRequest, changes chan<- svc.Status) (bool, uint32) {
	changes <- svc.Status{State: svc.StartPending}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- s.runFn(ctx)
	}()

	changes <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}

	for {
		select {
		case c := <-req:
			switch c.Cmd {
			case svc.Interrogate:
				changes <- c.CurrentStatus
			case svc.Stop, svc.Shutdown:
				changes <- svc.Status{State: svc.StopPending}
				cancel()
			}
		case err := <-errCh:
			if err != nil {
				slog.Error("windows service exited with error", "error", err)
				return false, 1
			}
			return false, 0
		}
	}
}
