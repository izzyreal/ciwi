package server

import "net/http"

func serverInfoHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"name":        "ciwi",
		"api_version": 1,
		"version":     currentVersion(),
	})
}
