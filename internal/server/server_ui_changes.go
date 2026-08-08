package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type browserUIChange struct {
	ServerInstanceID string   `json:"server_instance_id"`
	Revision         uint64   `json:"revision"`
	Topics           []string `json:"topics"`
	JobExecutionIDs  []string `json:"job_execution_ids,omitempty"`
	OccurredUnixMS   int64    `json:"occurred_unix_ms"`
	ResyncRequired   bool     `json:"resync_required"`
}

func (s *stateStore) uiChangesHandler(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	changes := s.app().changes.Watch(r.Context())
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case change, ok := <-changes:
			if !ok {
				return
			}
			topics := make([]string, 0, len(change.Topics))
			for _, topic := range change.Topics {
				topics = append(topics, string(topic))
			}
			payload, err := json.Marshal(browserUIChange{
				ServerInstanceID: change.InstanceID, Revision: change.Revision, Topics: topics,
				JobExecutionIDs: append([]string(nil), change.JobExecutionIDs...),
				OccurredUnixMS:  change.OccurredAt.UnixMilli(), ResyncRequired: change.Resync,
			})
			if err != nil {
				return
			}
			if _, err := fmt.Fprintf(w, "data: %s\n\n", payload); err != nil {
				return
			}
			flusher.Flush()
		case <-heartbeat.C:
			if _, err := fmt.Fprint(w, ": keepalive\n\n"); err != nil {
				return
			}
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}
