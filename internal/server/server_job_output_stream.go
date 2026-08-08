package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/izzyreal/ciwi/internal/application"
)

const jobOutputHeartbeatInterval = 15 * time.Second

func (s *stateStore) jobOutputStreamHandler(w http.ResponseWriter, r *http.Request, jobID string) {
	afterEventID, err := jobOutputStreamCursor(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	if _, err := fmt.Fprint(w, "retry: 1000\n\n"); err != nil {
		return
	}
	flusher.Flush()

	changes := s.app().changes.Watch(r.Context())
	select {
	case _, ok := <-changes:
		if !ok {
			return
		}
	case <-r.Context().Done():
		return
	}
	heartbeat := time.NewTicker(jobOutputHeartbeatInterval)
	defer heartbeat.Stop()

	for {
		view, err := s.app().jobDetails.GetJobOutputView(r.Context(), jobID, afterEventID)
		if err != nil {
			writeJobOutputSSEError(w, flusher, err)
			return
		}
		response := jobOutputToResponse(view)
		payload, err := json.Marshal(response)
		if err != nil || writeSSE(w, flusher, "output", strconv.FormatInt(view.NextEventID, 10), payload) != nil {
			return
		}
		afterEventID = view.NextEventID
		if view.Terminal && !view.HasMore {
			_ = writeSSE(w, flusher, "complete", strconv.FormatInt(view.NextEventID, 10), []byte(`{"terminal":true}`))
			return
		}
		if view.HasMore {
			continue
		}

		for {
			select {
			case change, ok := <-changes:
				if !ok {
					return
				}
				if jobOutputChangeAffects(change, jobID) {
					goto changed
				}
			case <-heartbeat.C:
				if _, err := fmt.Fprint(w, ": keepalive\n\n"); err != nil {
					return
				}
				flusher.Flush()
			case <-r.Context().Done():
				return
			}
		}
	changed:
	}
}

func jobOutputStreamCursor(r *http.Request) (int64, error) {
	raw := strings.TrimSpace(r.Header.Get("Last-Event-ID"))
	if raw == "" {
		raw = strings.TrimSpace(r.URL.Query().Get("after_event_id"))
	}
	if raw == "" {
		return 0, nil
	}
	cursor, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || cursor < 0 {
		return 0, fmt.Errorf("after_event_id must be a non-negative integer")
	}
	return cursor, nil
}

func jobOutputChangeAffects(change application.Change, jobID string) bool {
	if change.Resync {
		return true
	}
	foundTopic := false
	for _, topic := range change.Topics {
		if topic == application.ChangeJobOutput {
			foundTopic = true
			break
		}
	}
	if !foundTopic {
		return false
	}
	if len(change.JobExecutionIDs) == 0 {
		return true
	}
	for _, changedJobID := range change.JobExecutionIDs {
		if changedJobID == jobID {
			return true
		}
	}
	return false
}

func writeSSE(w http.ResponseWriter, flusher http.Flusher, event, id string, payload []byte) error {
	if _, err := fmt.Fprintf(w, "id: %s\nevent: %s\ndata: %s\n\n", id, event, payload); err != nil {
		return err
	}
	flusher.Flush()
	return nil
}

func writeJobOutputSSEError(w http.ResponseWriter, flusher http.Flusher, err error) {
	payload, marshalErr := json.Marshal(map[string]string{"message": err.Error()})
	if marshalErr == nil {
		_ = writeSSE(w, flusher, "stream-error", "", payload)
	}
}
