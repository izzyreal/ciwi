package webui

import (
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/izzyreal/ciwi/pkg/uidsl"
	sharedUI "github.com/izzyreal/ciwi/ui"
)

var browserThemeColorVariables = map[string][]string{
	"surface":            {"--surface"},
	"surface-raised":     {"--surface-raised"},
	"surface-subtle":     {"--surface-subtle"},
	"surface-glow":       {"--card-glow"},
	"text":               {"--ink"},
	"text-muted":         {"--muted"},
	"accent":             {"--accent"},
	"accent-strong":      {"--accent-strong"},
	"border":             {"--line"},
	"success":            {"--ok"},
	"warning":            {"--warn"},
	"awaiting-surface":   {"--awaiting-bg"},
	"awaiting-border":    {"--awaiting-line"},
	"awaiting-text":      {"--awaiting-ink"},
	"danger":             {"--bad"},
	"focus":              {"--focus"},
	"pill-background":    {"--pill-bg"},
	"pill-text":          {"--pill-ink"},
	"notice-background":  {"--snackbar-bg"},
	"notice-text":        {"--snackbar-ink"},
	"notice-border":      {"--snackbar-line"},
	"console-background": {"--console-bg"},
	"console-surface":    {"--console-surface"},
	"console-border":     {"--console-line"},
	"console-text":       {"--console-ink"},
	"console-muted":      {"--console-muted"},
	"console-accent":     {"--console-accent"},
	"console-success":    {"--console-green"},
}

var browserThemeDimensionVariables = map[string]string{
	"small": "--ciwi-space-small", "medium": "--ciwi-space-medium", "large": "--ciwi-space-large",
	"page": "--ciwi-page-max", "page-inset": "--ciwi-page-inset",
	"section-padding": "--ciwi-section-padding", "card-padding": "--ciwi-card-padding",
	"hero-padding": "--ciwi-hero-padding", "surface-radius": "--ciwi-surface-radius",
	"control-radius": "--ciwi-control-radius", "control-padding-x": "--ciwi-control-padding-x",
	"control-padding-y": "--ciwi-control-padding-y", "text-body": "--ciwi-text-body",
	"text-control": "--ciwi-text-control", "text-code": "--ciwi-text-code",
	"text-badge": "--ciwi-text-badge", "text-subtitle": "--ciwi-text-subtitle",
	"text-heading": "--ciwi-text-heading", "text-title": "--ciwi-text-title",
	"image-brand-width": "--ciwi-image-brand-width", "image-brand-height": "--ciwi-image-brand-height",
}

func serveThemeCSS(w http.ResponseWriter, r *http.Request) {
	themes, err := sharedUI.LoadThemes()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	cacheVersionedUIResource(w, r)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(browserThemeCSS(themes)))
}

func browserThemeCSS(themes []*uidsl.ThemeDocument) string {
	var css strings.Builder
	for _, document := range themes {
		if document == nil {
			continue
		}
		fmt.Fprintf(&css, ":root[data-ciwi-theme=%q]{\n", document.Metadata.Name)
		if document.Theme.Dark {
			css.WriteString("  color-scheme:dark;\n")
		} else {
			css.WriteString("  color-scheme:light;\n")
		}
		for _, token := range sortedThemeKeys(browserThemeColorVariables) {
			value := document.Theme.Colors[token]
			if value == "" {
				continue
			}
			for _, variable := range browserThemeColorVariables[token] {
				fmt.Fprintf(&css, "  %s:%s;\n", variable, value)
			}
		}
		for _, token := range sortedThemeKeys(browserThemeDimensionVariables) {
			if value := document.Theme.Dimensions[token]; value != "" {
				fmt.Fprintf(&css, "  %s:%spx;\n", browserThemeDimensionVariables[token], value)
			}
		}
		if gradient, ok := document.Theme.Gradients["page"]; ok {
			fmt.Fprintf(&css, "  --page-background:%s;\n", browserPageBackgroundCSS(document.Theme, gradient))
		}
		if gradient, ok := document.Theme.Gradients["hero"]; ok {
			fmt.Fprintf(&css, "  --chrome-card-bg:%s;\n", browserGradientCSS(gradient))
		}
		css.WriteString("}\n")
	}
	return css.String()
}

func browserPageBackgroundCSS(theme uidsl.Theme, gradient uidsl.Gradient) string {
	base := browserGradientCSS(gradient)
	glowA := theme.Colors["background-glow-a"]
	glowB := theme.Colors["background-glow-b"]
	if glowA == "" || glowB == "" {
		return base
	}
	return fmt.Sprintf(
		"radial-gradient(circle at 12%% -10%%,color-mix(in srgb,%s 86%%,transparent) 0%%,transparent 38%%),"+
			"radial-gradient(circle at 90%% 8%%,color-mix(in srgb,%s 82%%,transparent) 0%%,transparent 34%%),%s",
		glowA, glowB, base,
	)
}

func browserGradientCSS(gradient uidsl.Gradient) string {
	stops := make([]string, 0, len(gradient.Stops))
	for _, stop := range gradient.Stops {
		stops = append(stops, fmt.Sprintf("%s %d%%", stop.Color, stop.Position))
	}
	if gradient.Kind == "radial" {
		return "radial-gradient(circle," + strings.Join(stops, ",") + ")"
	}
	return fmt.Sprintf("linear-gradient(%ddeg,%s)", gradient.Angle, strings.Join(stops, ","))
}

func sortedThemeKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
