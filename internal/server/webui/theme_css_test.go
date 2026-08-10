package webui

import (
	"strings"
	"testing"

	sharedUI "github.com/izzyreal/ciwi/ui"
)

func TestBrowserThemeCSSComesFromSharedDocuments(t *testing.T) {
	themes, err := sharedUI.LoadThemes()
	if err != nil {
		t.Fatal(err)
	}
	css := browserThemeCSS(themes)
	for _, theme := range themes {
		selector := `:root[data-ciwi-theme="` + theme.Metadata.Name + `"]`
		start := strings.Index(css, selector)
		if start < 0 {
			t.Errorf("missing selector for %q", theme.Metadata.Name)
			continue
		}
		block := css[start:]
		for token, variables := range browserThemeColorVariables {
			if theme.Theme.Colors[token] == "" {
				continue
			}
			for _, variable := range variables {
				if !strings.Contains(block, variable+":"+theme.Theme.Colors[token]) {
					t.Errorf("theme %q does not map %s to %s", theme.Metadata.Name, token, variable)
				}
			}
		}
		if gradient, ok := theme.Theme.Gradients["page"]; ok {
			if want := browserPageBackgroundCSS(theme.Theme, gradient); !strings.Contains(block, "--page-background:"+want) {
				t.Errorf("theme %q does not use its shared page gradient", theme.Metadata.Name)
			}
		}
		if gradient, ok := theme.Theme.Gradients["hero"]; ok {
			if want := browserGradientCSS(gradient); !strings.Contains(block, "--chrome-card-bg:"+want) {
				t.Errorf("theme %q does not use its shared hero gradient", theme.Metadata.Name)
			}
		}
	}
}
