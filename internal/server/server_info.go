package server

import (
	"net/http"
	"os"
	"strings"
)

func serverInfoHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	host, _ := os.Hostname()
	host = strings.TrimSpace(host)
	writeJSON(w, http.StatusOK, serverInfoResponse{
		Name:       "ciwi",
		APIVersion: 1,
		Version:    currentVersion(),
		Hostname:   host,
	})
}
