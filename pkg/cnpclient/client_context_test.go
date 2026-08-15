package cnpclient

import (
	"context"
	"errors"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/izzyreal/ciwi/pkg/cnp"
)

type cancellationTestSession struct{ stream *cancellationTestStream }

func (s *cancellationTestSession) OpenStream(context.Context) (cnp.Stream, error) {
	return s.stream, nil
}
func (*cancellationTestSession) AcceptStream(context.Context) (cnp.Stream, error) {
	return nil, errors.New("not implemented")
}
func (*cancellationTestSession) CloseWithError(error) error { return nil }

type cancellationTestStream struct {
	blockWrite     bool
	readDone       chan struct{}
	writeDone      chan struct{}
	writeStarted   chan struct{}
	readOnce       sync.Once
	writeOnce      sync.Once
	writeStartOnce sync.Once
	cancelRead     atomic.Int32
	cancelWrite    atomic.Int32
	closes         atomic.Int32
}

func newCancellationTestStream(blockWrite bool) *cancellationTestStream {
	return &cancellationTestStream{blockWrite: blockWrite, readDone: make(chan struct{}), writeDone: make(chan struct{}), writeStarted: make(chan struct{})}
}

func (s *cancellationTestStream) Read([]byte) (int, error) {
	<-s.readDone
	return 0, io.ErrClosedPipe
}

func (s *cancellationTestStream) Write(payload []byte) (int, error) {
	if !s.blockWrite {
		return len(payload), nil
	}
	s.writeStartOnce.Do(func() { close(s.writeStarted) })
	<-s.writeDone
	return 0, io.ErrClosedPipe
}

func (s *cancellationTestStream) Close() error {
	s.closes.Add(1)
	return nil
}

func (s *cancellationTestStream) CancelRead() {
	s.cancelRead.Add(1)
	s.readOnce.Do(func() { close(s.readDone) })
}

func (s *cancellationTestStream) CancelWrite() {
	s.cancelWrite.Add(1)
	s.writeOnce.Do(func() { close(s.writeDone) })
}

func TestHelloCancellationInterruptsResponseIO(t *testing.T) {
	stream := newCancellationTestStream(false)
	client := &Client{session: &cancellationTestSession{stream: stream}}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- client.hello(ctx, "test", "1") }()
	waitForCancellationRead(t, stream)
	cancel()
	assertContextCancellation(t, <-done, stream)
}

func TestCallCancellationInterruptsResponseIO(t *testing.T) {
	stream := newCancellationTestStream(false)
	client := &Client{session: &cancellationTestSession{stream: stream}}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := client.GetServerInfo(ctx)
		done <- err
	}()
	waitForCancellationRead(t, stream)
	cancel()
	assertContextCancellation(t, <-done, stream)
}

func TestWatchCancellationInterruptsInitialWrite(t *testing.T) {
	for _, test := range []struct {
		name string
		open func(*Client, context.Context) error
	}{
		{name: "changes", open: func(client *Client, ctx context.Context) error {
			_, _, err := client.WatchChanges(ctx)
			return err
		}},
		{name: "job output", open: func(client *Client, ctx context.Context) error {
			_, _, err := client.WatchJobOutput(ctx, "job-1", 0)
			return err
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			stream := newCancellationTestStream(true)
			client := &Client{session: &cancellationTestSession{stream: stream}}
			ctx, cancel := context.WithCancel(context.Background())
			done := make(chan error, 1)
			go func() { done <- test.open(client, ctx) }()
			waitForCancellationWrite(t, stream)
			cancel()
			assertContextCancellation(t, <-done, stream)
		})
	}
}

func waitForCancellationRead(t *testing.T, stream *cancellationTestStream) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for stream.closes.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if stream.closes.Load() == 0 {
		t.Fatal("request did not reach its response read")
	}
}

func waitForCancellationWrite(t *testing.T, stream *cancellationTestStream) {
	t.Helper()
	select {
	case <-stream.writeStarted:
	case <-time.After(time.Second):
		t.Fatal("request did not reach its initial write")
	}
}

func assertContextCancellation(t *testing.T, err error, stream *cancellationTestStream) {
	t.Helper()
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context cancellation", err)
	}
	if stream.cancelRead.Load() == 0 || stream.cancelWrite.Load() == 0 || stream.closes.Load() == 0 {
		t.Fatalf("interrupts: read=%d write=%d close=%d", stream.cancelRead.Load(), stream.cancelWrite.Load(), stream.closes.Load())
	}
}
