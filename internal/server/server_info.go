package server

import (
	"net/http"

	"github.com/izzyreal/ciwi/internal/application"
)

func serverInfoHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	queries := application.NewServerQueries(localServerInfoSource{})
	info, err := queries.GetServerInfo(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, serverInfoResponse{
		Name:       info.Name,
		APIVersion: info.APIVersion,
		Version:    info.Version,
		Hostname:   info.Hostname,
	})
}
