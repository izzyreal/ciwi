package server

import (
	"net/http"
	"time"
)

type serverRestartResponse struct {
	Restarting bool   `json:"restarting"`
	Message    string `json:"message,omitempty"`
}

func (s *stateStore) serverRestartHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	_ = s.persistUpdateStatus(map[string]string{
		"update_message": "manual server restart requested",
	})
	writeJSON(w, http.StatusOK, serverRestartResponse{
		Restarting: true,
		Message:    "server restart requested",
	})
	go func(restartFn func()) {
		time.Sleep(250 * time.Millisecond)
		if restartFn != nil {
			restartFn()
		}
	}(s.restartServerFn)
}
