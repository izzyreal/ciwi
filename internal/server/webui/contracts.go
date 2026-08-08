package webui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"

	"github.com/izzyreal/ciwi/pkg/uidsl"
	sharedUI "github.com/izzyreal/ciwi/ui"
)

var (
	routeContractOnce sync.Once
	routeContract     *uidsl.RouteDocument
	routeContractErr  error
)

func loadRouteContract() (*uidsl.RouteDocument, error) {
	routeContractOnce.Do(func() { routeContract, routeContractErr = sharedUI.LoadRoutes() })
	return routeContract, routeContractErr
}

func serveScreenContract(w http.ResponseWriter, r *http.Request, name string) {
	screen, err := sharedUI.LoadScreen(name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	serveJSON(w, r, screen)
}

func serveThemeContracts(w http.ResponseWriter, r *http.Request) {
	themes, err := sharedUI.LoadThemes()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	serveJSON(w, r, themes)
}

func serveActionContract(w http.ResponseWriter, r *http.Request) {
	catalog, err := sharedUI.LoadActionCatalog()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	serveJSON(w, r, catalog)
}

func serveRouteContract(w http.ResponseWriter, r *http.Request) {
	routes, err := loadRouteContract()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	serveJSON(w, r, routes)
}

func serveTypographyContract(w http.ResponseWriter, r *http.Request) {
	typography, err := sharedUI.LoadTypography()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	serveJSON(w, r, typography)
}

func serveControlsContract(w http.ResponseWriter, r *http.Request) {
	controls, err := sharedUI.LoadControls()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	serveJSON(w, r, controls)
}

func serveTypographyCSS(w http.ResponseWriter, r *http.Request) {
	document, err := sharedUI.LoadTypography()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var css strings.Builder
	css.WriteString("@font-face{font-family:\"Ciwi Mono\";src:url(\"/ui/fonts/ciwi-mono-regular.ttf?v=3\") format(\"truetype\");font-style:normal;font-weight:400;font-display:swap}\n")
	css.WriteString("@font-face{font-family:\"Ciwi Mono\";src:url(\"/ui/fonts/ciwi-mono-medium.ttf?v=3\") format(\"truetype\");font-style:normal;font-weight:500;font-display:swap}\n")
	css.WriteString("@font-face{font-family:\"Ciwi Mono\";src:url(\"/ui/fonts/ciwi-mono-bold.ttf?v=3\") format(\"truetype\");font-style:normal;font-weight:700;font-display:swap}\n")
	css.WriteString(":root{\n")
	families := sortedMapKeys(document.Typography.Families)
	for _, name := range families {
		fmt.Fprintf(&css, "  --ciwi-font-%s:%s;\n", name, document.Typography.Families[name])
	}
	weights := sortedMapKeys(document.Typography.Weights)
	for _, name := range weights {
		fmt.Fprintf(&css, "  --ciwi-weight-%s:%d;\n", name, document.Typography.Weights[name].Web)
	}
	roles := sortedMapKeys(document.Typography.Roles)
	for _, name := range roles {
		role := document.Typography.Roles[name]
		fmt.Fprintf(&css, "  --ciwi-type-%s-family:var(--ciwi-font-%s);\n", name, role.Family)
		fmt.Fprintf(&css, "  --ciwi-type-%s-size:%gpx;\n", name, role.Size)
		fmt.Fprintf(&css, "  --ciwi-type-%s-weight:var(--ciwi-weight-%s);\n", name, role.Weight)
		if role.LineHeight > 0 {
			fmt.Fprintf(&css, "  --ciwi-type-%s-line-height:%g;\n", name, role.LineHeight)
		}
	}
	css.WriteString("}\n")
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	cacheVersionedUIResource(w, r)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(css.String()))
}

func sortedMapKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func serveJSON(w http.ResponseWriter, r *http.Request, value any) {
	payload, err := json.Marshal(value)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	cacheVersionedUIResource(w, r)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}
