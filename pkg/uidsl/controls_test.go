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
  button: {iconPosition: leading, iconSize: 19, iconGap: 8}
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
}

func validControlsDocument() *ControlsDocument {
	return &ControlsDocument{
		APIVersion: APIVersion,
		Kind:       "Controls",
		Controls: Controls{
			Button: ButtonControl{IconPosition: "leading", IconSize: 19, IconGap: 8},
			Select: SelectControl{
				ChevronPosition: "trailing", ChevronSize: 19, ChevronGap: 12, MinimumHeight: 44,
				MenuGap: 6, MenuPadding: 6, MenuItemGap: 2, MenuMinimumWidth: 120,
				MenuMinimumHeight: 120, MenuMaximumHeight: 420, ViewportInset: 8,
				OptionGap: 8, OptionPaddingX: 10, OptionPaddingY: 7, OptionMinimumHeight: 40,
				SelectionIndicatorWidth: 20,
			},
		},
	}
}
