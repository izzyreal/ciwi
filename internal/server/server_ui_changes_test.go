package server

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/izzyreal/ciwi/internal/application"
)

type streamingRecorder struct {
	header  http.Header
	mu      sync.Mutex
	body    bytes.Buffer
	flushed chan struct{}
}

func newStreamingRecorder() *streamingRecorder {
	return &streamingRecorder{header: http.Header{}, flushed: make(chan struct{}, 4)}
}
func (r *streamingRecorder) Header() http.Header { return r.header }
func (r *streamingRecorder) WriteHeader(int)     {}
func (r *streamingRecorder) Write(payload []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.body.Write(payload)
}
func (r *streamingRecorder) Flush() {
	select {
	case r.flushed <- struct{}{}:
	default:
	}
}
func (r *streamingRecorder) String() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.body.String()
}

func TestBrowserUIChangeStreamSendsResyncAndTopicInvalidations(t *testing.T) {
	state := &stateStore{}
	recorder := newStreamingRecorder()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		state.uiChangesHandler(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/ui/changes", nil).WithContext(ctx))
		close(done)
	}()
	waitFlush := func() {
		t.Helper()
		select {
		case <-recorder.flushed:
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for UI change event")
		}
	}
	waitFlush()
	state.app().changes.Publish(application.ChangeVault)
	waitFlush()
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("UI change stream did not stop with its request")
	}
	if contentType := recorder.Header().Get("Content-Type"); contentType != "text/event-stream" {
		t.Fatalf("content type = %q", contentType)
	}
	body := recorder.String()
	if !strings.Contains(body, `"resync_required":true`) || !strings.Contains(body, `"topics":["vault"]`) {
		t.Fatalf("stream body = %q", body)
	}
}
