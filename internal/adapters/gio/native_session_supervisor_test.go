//go:build darwin || linux || windows

package gio

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type supervisorAttempt struct {
	ctx     context.Context
	release chan struct{}
	session *nativeSession
}

type supervisorScheduler struct {
	mu      sync.Mutex
	wake    chan time.Time
	delay   time.Duration
	stopped bool
}

func newSupervisorScheduler() *supervisorScheduler {
	return &supervisorScheduler{wake: make(chan time.Time, 1)}
}

func (s *supervisorScheduler) Arm(delay time.Duration) {
	s.mu.Lock()
	s.delay, s.stopped = delay, false
	s.mu.Unlock()
}
func (s *supervisorScheduler) Stop() {
	s.mu.Lock()
	s.stopped = true
	s.mu.Unlock()
}
func (s *supervisorScheduler) C() <-chan time.Time { return s.wake }

func TestNativeSessionSupervisorRejectsPreSuspendSession(t *testing.T) {
	rootCtx, cancelRoot := context.WithCancel(context.Background())
	defer cancelRoot()
	attempts := make(chan supervisorAttempt, 2)
	var closed [2]atomic.Int32
	var call atomic.Int32
	connector := func(ctx context.Context, _ nativeConnectionSettings, _ string) (*nativeSession, error) {
		index := int(call.Add(1) - 1)
		release := make(chan struct{})
		session := &nativeSession{cancelWatch: func() { closed[index].Add(1) }}
		attempts <- supervisorAttempt{ctx: ctx, release: release, session: session}
		<-release // Model a transport that completes after cancellation raced it.
		return session, nil
	}
	supervisor := newNativeSessionSupervisor(rootCtx, "test", connector, newSupervisorScheduler())
	defer supervisor.Close()

	supervisor.Start(nativeConnectionSettings{Endpoint: "first"})
	first := receiveSupervisorAttempt(t, attempts)
	supervisor.Suspend()
	select {
	case <-first.ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("suspension did not cancel the active attempt")
	}

	supervisor.Resume(nativeConnectionSettings{Endpoint: "second"})
	second := receiveSupervisorAttempt(t, attempts)
	close(second.release)
	accepted := receiveCurrentSupervisorSession(t, supervisor)
	if accepted != second.session {
		t.Fatalf("accepted session = %p, want resumed session %p", accepted, second.session)
	}

	close(first.release)
	deadline := time.Now().Add(time.Second)
	for closed[0].Load() == 0 && time.Now().Before(deadline) {
		select {
		case result := <-supervisor.Results():
			supervisor.Accept(result)
		case <-time.After(time.Millisecond):
		}
	}
	if closed[0].Load() == 0 {
		t.Fatal("pre-suspend session was not closed")
	}
}

func TestNativeSessionSupervisorOwnsRetryAndBackoff(t *testing.T) {
	scheduler := newSupervisorScheduler()
	var closed atomic.Int32
	supervisor := newNativeSessionSupervisor(context.Background(), "test", func(context.Context, nativeConnectionSettings, string) (*nativeSession, error) {
		return &nativeSession{cancelWatch: func() { closed.Add(1) }}, nil
	}, scheduler)
	defer supervisor.Close()

	supervisor.Schedule(nativeConnectionSettings{Endpoint: "server"}, 3*time.Second)
	scheduler.mu.Lock()
	delay, stopped := scheduler.delay, scheduler.stopped
	scheduler.mu.Unlock()
	if delay != 3*time.Second || stopped {
		t.Fatalf("scheduled delay=%s stopped=%v", delay, stopped)
	}
	supervisor.AdvanceBackoff()
	if supervisor.Backoff() != 2*time.Second {
		t.Fatalf("backoff = %s", supervisor.Backoff())
	}
	supervisor.ResetBackoff()
	if supervisor.Backoff() != time.Second {
		t.Fatalf("reset backoff = %s", supervisor.Backoff())
	}
	if supervisor.Retry() == nil {
		t.Fatal("retry channel is unavailable")
	}
	supervisor.RetryNow()
	result := <-supervisor.Results()
	accepted, err, current := supervisor.Accept(result)
	if err != nil || !current || accepted == nil || supervisor.Context() == nil || supervisor.Generation() == 0 {
		t.Fatalf("accepted=%p err=%v current=%v context=%v generation=%d", accepted, err, current, supervisor.Context(), supervisor.Generation())
	}
	supervisor.Disconnect()
	if supervisor.Context() != nil || closed.Load() != 1 {
		t.Fatalf("disconnect context=%v closes=%d", supervisor.Context(), closed.Load())
	}
}

func TestNativeSessionSupervisorReportsCurrentFailureAndStaysSuspended(t *testing.T) {
	want := errors.New("unreachable")
	var calls atomic.Int32
	supervisor := newNativeSessionSupervisor(context.Background(), "test", func(context.Context, nativeConnectionSettings, string) (*nativeSession, error) {
		calls.Add(1)
		return nil, want
	}, newSupervisorScheduler())
	defer supervisor.Close()
	supervisor.Start(nativeConnectionSettings{})
	result := <-supervisor.Results()
	if session, err, current := supervisor.Accept(result); session != nil || !current || !errors.Is(err, want) {
		t.Fatalf("failure acceptance = %p, %v, %v", session, err, current)
	}
	if supervisor.Context() != nil {
		t.Fatal("failed attempt retained its lifetime")
	}
	supervisor.Suspend()
	supervisor.Start(nativeConnectionSettings{})
	time.Sleep(10 * time.Millisecond)
	if calls.Load() != 1 {
		t.Fatalf("suspended supervisor started %d attempts", calls.Load())
	}
}

func TestNativeTimerSchedulerArmsResetsAndStops(t *testing.T) {
	scheduler := &nativeTimerScheduler{}
	scheduler.Arm(time.Hour)
	if scheduler.C() == nil {
		t.Fatal("armed timer has no wake channel")
	}
	scheduler.Arm(0)
	select {
	case <-scheduler.C():
	case <-time.After(time.Second):
		t.Fatal("reset timer did not fire")
	}
	scheduler.Stop()
	if scheduler.C() != nil {
		t.Fatal("stopped timer retained wake channel")
	}
}

func receiveSupervisorAttempt(t *testing.T, attempts <-chan supervisorAttempt) supervisorAttempt {
	t.Helper()
	select {
	case attempt := <-attempts:
		return attempt
	case <-time.After(time.Second):
		t.Fatal("connection attempt did not start")
		return supervisorAttempt{}
	}
}

func receiveCurrentSupervisorSession(t *testing.T, supervisor *nativeSessionSupervisor) *nativeSession {
	t.Helper()
	deadline := time.After(time.Second)
	for {
		select {
		case result := <-supervisor.Results():
			session, err, current := supervisor.Accept(result)
			if current {
				if err != nil {
					t.Fatal(err)
				}
				return session
			}
		case <-deadline:
			t.Fatal("resumed session was not accepted")
			return nil
		}
	}
}
