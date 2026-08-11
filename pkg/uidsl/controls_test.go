package uidsl

import (
	"strings"
	"testing"
)

func TestParseControls(t *testing.T) {
	document, err := ParseControls([]byte(`
apiVersion: ciwi.ui/v1
kind: Controls
controls:
  viewport:
    compactMaximumWidth: 760
    condensedDisclosureMaximumWidth: 560
  button:
    iconPosition: leading
    minimumHeight: {web: 44, native: 44}
    paddingX: {web: 12, native: 12}
    paddingY: {web: 8, native: 8}
    iconSize: {web: 19, native: 19}
    iconGap: {web: 8, native: 8}
    iconOnlySize: {web: 34, native: 34}
  badge:
    paddingX: 9
    paddingY: 4
    tintOpacity: 0.12
    borderOpacity: 0.55
  input:
    minimumHeight: {web: 44, native: 44}
    paddingX: {web: 12, native: 12}
    paddingY: {web: 9, native: 8}
    placeholderColor: "#757575"
  select:
    chevronPosition: trailing
    chevronSize: 19
    chevronGap: 12
    minimumHeight: 44
    menuGap: 6
    menuPadding: 6
    menuItemGap: 2
    menuMinimumWidth: 120
    menuMinimumHeight: 120
    menuMaximumHeight: 420
    viewportInset: 8
    optionGap: 8
    optionPaddingX: 10
    optionPaddingY: 7
    optionMinimumHeight: 40
    selectionIndicatorWidth: 20
  disclosure:
    chevronPosition: trailing
    chevronSize: 20
    chevronGap: 8
  progress:
    tintOpacity: 0.18
`))
	if err != nil {
		t.Fatal(err)
	}
	if document.Controls.Button.IconPosition != "leading" || document.Controls.Select.ChevronPosition != "trailing" {
		t.Fatalf("controls = %#v", document.Controls)
	}
}

func TestControlsValidationRejectsInvalidVisualMetrics(t *testing.T) {
	document := validControlsDocument()
	document.Controls.Button.IconPosition = "center"
	if err := document.Validate(); err == nil || !strings.Contains(err.Error(), "iconPosition") {
		t.Fatalf("invalid position error = %v", err)
	}
	document = validControlsDocument()
	document.Controls.Select.MenuMaximumHeight = 80
	if err := document.Validate(); err == nil || !strings.Contains(err.Error(), "menuMaximumHeight") {
		t.Fatalf("invalid menu height error = %v", err)
	}
	document = validControlsDocument()
	document.Controls.Viewport.CondensedDisclosureMaximumWidth = 800
	if err := document.Validate(); err == nil || !strings.Contains(err.Error(), "condensedDisclosureMaximumWidth") {
		t.Fatalf("invalid viewport boundary error = %v", err)
	}
	document = validControlsDocument()
	document.Controls.Input.PlaceholderColor = "gray"
	if err := document.Validate(); err == nil || !strings.Contains(err.Error(), "placeholderColor") {
		t.Fatalf("invalid placeholder color error = %v", err)
	}
}

func validControlsDocument() *ControlsDocument {
	return &ControlsDocument{
		APIVersion: APIVersion,
		Kind:       "Controls",
		Controls: Controls{
			Viewport: ViewportControl{CompactMaximumWidth: 760, CondensedDisclosureMaximumWidth: 560},
			Button: ButtonControl{
				IconPosition:  "leading",
				MinimumHeight: PlatformMetric{Web: 44, Native: 44},
				PaddingX:      PlatformMetric{Web: 12, Native: 12},
				PaddingY:      PlatformMetric{Web: 8, Native: 8},
				IconSize:      PlatformMetric{Web: 19, Native: 19},
				IconGap:       PlatformMetric{Web: 8, Native: 8},
				IconOnlySize:  PlatformMetric{Web: 34, Native: 34},
			},
			Badge: BadgeControl{PaddingX: 9, PaddingY: 4, TintOpacity: 0.12, BorderOpacity: 0.55},
			Input: InputControl{
				MinimumHeight:    PlatformMetric{Web: 44, Native: 44},
				PaddingX:         PlatformMetric{Web: 12, Native: 12},
				PaddingY:         PlatformMetric{Web: 9, Native: 8},
				PlaceholderColor: "#757575",
			},
			Select: SelectControl{
				ChevronPosition: "trailing", ChevronSize: 19, ChevronGap: 12, MinimumHeight: 44,
				MenuGap: 6, MenuPadding: 6, MenuItemGap: 2, MenuMinimumWidth: 120,
				MenuMinimumHeight: 120, MenuMaximumHeight: 420, ViewportInset: 8,
				OptionGap: 8, OptionPaddingX: 10, OptionPaddingY: 7, OptionMinimumHeight: 40,
				SelectionIndicatorWidth: 20,
			},
			Disclosure: DisclosureControl{ChevronPosition: "trailing", ChevronSize: 20, ChevronGap: 8},
			Progress:   ProgressControl{TintOpacity: 0.18},
		},
	}
}
