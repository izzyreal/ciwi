package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
)

type commandReceiptStatusResponse struct {
	Found       bool            `json:"found"`
	Key         string          `json:"key"`
	Operation   string          `json:"operation,omitempty"`
	Fingerprint string          `json:"fingerprint,omitempty"`
	Status      string          `json:"status,omitempty"`
	Result      json.RawMessage `json:"result,omitempty"`
}

func (s *stateStore) commandReceiptStatusHandler(w http.ResponseWriter, r *http.Request) {
	key := strings.TrimSpace(chi.URLParam(r, "key"))
	status, err := s.app().commandReceipts.Get(r.Context(), key)
	if err != nil {
		writeApplicationHTTPError(w, err)
		return
	}
	response := commandReceiptStatusResponse{
		Found: status.Found, Key: status.Key, Operation: status.Operation,
		Fingerprint: status.Fingerprint, Status: status.Status,
	}
	if len(status.Result) > 0 && json.Valid(status.Result) {
		response.Result = append(json.RawMessage(nil), status.Result...)
	}
	writeJSON(w, http.StatusOK, response)
}
