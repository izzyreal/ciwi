package webui

import (
	"encoding/json"
	"net/http"

	sharedUI "github.com/izzyreal/ciwi/ui"
)

func serveScreenContract(w http.ResponseWriter, name string) {
	screen, err := sharedUI.LoadScreen(name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	serveJSON(w, screen)
}

func serveThemeContracts(w http.ResponseWriter) {
	themes, err := sharedUI.LoadThemes()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	serveJSON(w, themes)
}

func serveActionContract(w http.ResponseWriter) {
	catalog, err := sharedUI.LoadActionCatalog()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	serveJSON(w, catalog)
}

func serveJSON(w http.ResponseWriter, value any) {
	payload, err := json.Marshal(value)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}
