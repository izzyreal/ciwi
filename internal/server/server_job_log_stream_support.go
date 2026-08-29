package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/izzyreal/ciwi/internal/application"
)

const jobLogHeartbeatInterval = 15 * time.Second

func jobLogChangeAffects(change application.Change, jobID string) bool {
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
	if !foundTopic || len(change.JobExecutionIDs) == 0 {
		return foundTopic
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

func writeJobLogSSEError(w http.ResponseWriter, flusher http.Flusher, err error) {
	payload, marshalErr := json.Marshal(map[string]string{"message": err.Error()})
	if marshalErr == nil {
		_ = writeSSE(w, flusher, "stream-error", "", payload)
	}
}
