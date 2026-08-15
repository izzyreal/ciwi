//go:build darwin || ios || linux || windows

package gio

import (
	"context"
	"errors"
	"time"
)

type nativeSessionConnector func(context.Context, nativeConnectionSettings, string) (*nativeSession, error)

type nativeRetryScheduler interface {
	Arm(time.Duration)
	Stop()
	C() <-chan time.Time
}

type nativeTimerScheduler struct {
	timer *time.Timer
	wake  <-chan time.Time
}

func (s *nativeTimerScheduler) Arm(delay time.Duration) {
	if s.timer == nil {
		s.timer = time.NewTimer(delay)
	} else {
		if !s.timer.Stop() {
			select {
			case <-s.timer.C:
			default:
			}
		}
		s.timer.Reset(delay)
	}
	s.wake = s.timer.C
}

func (s *nativeTimerScheduler) Stop() {
	if s.timer != nil && !s.timer.Stop() {
		select {
		case <-s.timer.C:
		default:
		}
	}
	s.wake = nil
}

func (s *nativeTimerScheduler) C() <-chan time.Time { return s.wake }

// nativeSessionSupervisor owns connection attempts, accepted session lifetime,
// retry timing, and generation invalidation. Its methods are called only by the
// native controller; connector goroutines communicate through Results.
type nativeSessionSupervisor struct {
	ctx       context.Context
	version   string
	connect   nativeSessionConnector
	scheduler nativeRetryScheduler
	results   chan nativeConnectResult

	generation uint64
	lifetime   context.Context
	cancel     context.CancelFunc
	session    *nativeSession
	pending    nativeConnectionSettings
	suspended  bool
	backoff    time.Duration
}

func newNativeSessionSupervisor(ctx context.Context, version string, connector nativeSessionConnector, scheduler nativeRetryScheduler) *nativeSessionSupervisor {
	if connector == nil {
		connector = connectConfiguredNativeSession
	}
	if scheduler == nil {
		scheduler = &nativeTimerScheduler{}
	}
	return &nativeSessionSupervisor{
		ctx: ctx, version: version, connect: connector, scheduler: scheduler,
		results: make(chan nativeConnectResult, 1), backoff: time.Second,
	}
}

func (s *nativeSessionSupervisor) Results() <-chan nativeConnectResult { return s.results }
func (s *nativeSessionSupervisor) Retry() <-chan time.Time             { return s.scheduler.C() }
func (s *nativeSessionSupervisor) Generation() uint64                  { return s.generation }
func (s *nativeSessionSupervisor) Context() context.Context            { return s.lifetime }
func (s *nativeSessionSupervisor) Backoff() time.Duration              { return s.backoff }

func (s *nativeSessionSupervisor) ResetBackoff() { s.backoff = time.Second }

func (s *nativeSessionSupervisor) AdvanceBackoff() {
	s.backoff = nextReconnectDelay(s.backoff)
}

func (s *nativeSessionSupervisor) Start(settings nativeConnectionSettings) {
	if s.suspended {
		return
	}
	s.stopCurrent()
	s.pending = cloneNativeConnectionSettings(settings)
	s.generation++
	generation := s.generation
	attemptCtx, cancel := context.WithCancel(s.ctx)
	s.lifetime, s.cancel = attemptCtx, cancel
	settings = cloneNativeConnectionSettings(settings)
	go func() {
		connected, err := s.connect(attemptCtx, settings, s.version)
		result := nativeConnectResult{generation: generation, session: connected, err: err}
		select {
		case s.results <- result:
		case <-attemptCtx.Done():
			if connected != nil {
				connected.close()
			}
		case <-s.ctx.Done():
			if connected != nil {
				connected.close()
			}
		}
	}()
}

func (s *nativeSessionSupervisor) Schedule(settings nativeConnectionSettings, delay time.Duration) {
	s.stopCurrent()
	s.pending = cloneNativeConnectionSettings(settings)
	s.scheduler.Arm(delay)
}

func (s *nativeSessionSupervisor) RetryNow() {
	s.scheduler.Stop()
	s.Start(s.pending)
}

func (s *nativeSessionSupervisor) Suspend() {
	s.suspended = true
	s.scheduler.Stop()
	s.stopCurrent()
}

func (s *nativeSessionSupervisor) Resume(settings nativeConnectionSettings) {
	s.suspended = false
	s.ResetBackoff()
	s.Start(settings)
}

func (s *nativeSessionSupervisor) Disconnect() {
	s.scheduler.Stop()
	s.stopCurrent()
}

func (s *nativeSessionSupervisor) Accept(result nativeConnectResult) (*nativeSession, error, bool) {
	if result.generation != s.generation || s.suspended {
		if result.session != nil {
			result.session.close()
		}
		return nil, nil, false
	}
	if result.err != nil {
		if s.cancel != nil {
			s.cancel()
		}
		s.cancel, s.lifetime = nil, nil
		return nil, result.err, true
	}
	if result.session == nil {
		if s.cancel != nil {
			s.cancel()
		}
		s.cancel, s.lifetime = nil, nil
		return nil, errors.New("native connector returned an empty session"), true
	}
	s.session = result.session
	return result.session, nil, true
}

func (s *nativeSessionSupervisor) Close() {
	s.suspended = true
	s.scheduler.Stop()
	s.stopCurrent()
}

func (s *nativeSessionSupervisor) stopCurrent() {
	if s.cancel != nil {
		s.cancel()
	}
	s.cancel, s.lifetime = nil, nil
	if s.session != nil {
		s.session.close()
		s.session = nil
	}
	s.generation++
}

func cloneNativeConnectionSettings(settings nativeConnectionSettings) nativeConnectionSettings {
	settings.SSH.PrivateKey = append([]byte(nil), settings.SSH.PrivateKey...)
	return settings
}
