package uidsl

import "fmt"

type ControlsDocument struct {
	APIVersion string   `yaml:"apiVersion" json:"apiVersion"`
	Kind       string   `yaml:"kind" json:"kind"`
	Controls   Controls `yaml:"controls" json:"controls"`
}

type Controls struct {
	Button ButtonControl `yaml:"button" json:"button"`
	Select SelectControl `yaml:"select" json:"select"`
}

type ButtonControl struct {
	IconPosition string  `yaml:"iconPosition" json:"iconPosition"`
	IconSize     float32 `yaml:"iconSize" json:"iconSize"`
	IconGap      float32 `yaml:"iconGap" json:"iconGap"`
}

type SelectControl struct {
	ChevronPosition         string  `yaml:"chevronPosition" json:"chevronPosition"`
	ChevronSize             float32 `yaml:"chevronSize" json:"chevronSize"`
	ChevronGap              float32 `yaml:"chevronGap" json:"chevronGap"`
	MinimumHeight           float32 `yaml:"minimumHeight" json:"minimumHeight"`
	MenuGap                 float32 `yaml:"menuGap" json:"menuGap"`
	MenuPadding             float32 `yaml:"menuPadding" json:"menuPadding"`
	MenuItemGap             float32 `yaml:"menuItemGap" json:"menuItemGap"`
	MenuMinimumWidth        float32 `yaml:"menuMinimumWidth" json:"menuMinimumWidth"`
	MenuMinimumHeight       float32 `yaml:"menuMinimumHeight" json:"menuMinimumHeight"`
	MenuMaximumHeight       float32 `yaml:"menuMaximumHeight" json:"menuMaximumHeight"`
	ViewportInset           float32 `yaml:"viewportInset" json:"viewportInset"`
	OptionGap               float32 `yaml:"optionGap" json:"optionGap"`
	OptionPaddingX          float32 `yaml:"optionPaddingX" json:"optionPaddingX"`
	OptionPaddingY          float32 `yaml:"optionPaddingY" json:"optionPaddingY"`
	OptionMinimumHeight     float32 `yaml:"optionMinimumHeight" json:"optionMinimumHeight"`
	SelectionIndicatorWidth float32 `yaml:"selectionIndicatorWidth" json:"selectionIndicatorWidth"`
}

func ParseControls(payload []byte) (*ControlsDocument, error) {
	var document ControlsDocument
	if err := decodeStrict(payload, &document); err != nil {
		return nil, err
	}
	if err := document.Validate(); err != nil {
		return nil, err
	}
	return &document, nil
}

func (d *ControlsDocument) Validate() error {
	if d == nil {
		return fmt.Errorf("controls document is nil")
	}
	if d.APIVersion != APIVersion {
		return fmt.Errorf("unsupported apiVersion %q", d.APIVersion)
	}
	if d.Kind != "Controls" {
		return fmt.Errorf("kind must be Controls, got %q", d.Kind)
	}
	if err := validateIconPosition("button iconPosition", d.Controls.Button.IconPosition); err != nil {
		return err
	}
	if err := validateIconPosition("select chevronPosition", d.Controls.Select.ChevronPosition); err != nil {
		return err
	}
	positive := map[string]float32{
		"button iconSize":                d.Controls.Button.IconSize,
		"select chevronSize":             d.Controls.Select.ChevronSize,
		"select minimumHeight":           d.Controls.Select.MinimumHeight,
		"select menuMinimumWidth":        d.Controls.Select.MenuMinimumWidth,
		"select menuMinimumHeight":       d.Controls.Select.MenuMinimumHeight,
		"select menuMaximumHeight":       d.Controls.Select.MenuMaximumHeight,
		"select optionMinimumHeight":     d.Controls.Select.OptionMinimumHeight,
		"select selectionIndicatorWidth": d.Controls.Select.SelectionIndicatorWidth,
	}
	for name, value := range positive {
		if value <= 0 {
			return fmt.Errorf("%s must be greater than zero", name)
		}
	}
	nonNegative := map[string]float32{
		"button iconGap":        d.Controls.Button.IconGap,
		"select chevronGap":     d.Controls.Select.ChevronGap,
		"select menuGap":        d.Controls.Select.MenuGap,
		"select menuPadding":    d.Controls.Select.MenuPadding,
		"select menuItemGap":    d.Controls.Select.MenuItemGap,
		"select viewportInset":  d.Controls.Select.ViewportInset,
		"select optionGap":      d.Controls.Select.OptionGap,
		"select optionPaddingX": d.Controls.Select.OptionPaddingX,
		"select optionPaddingY": d.Controls.Select.OptionPaddingY,
	}
	for name, value := range nonNegative {
		if value < 0 {
			return fmt.Errorf("%s must be non-negative", name)
		}
	}
	if d.Controls.Select.MenuMaximumHeight < d.Controls.Select.MenuMinimumHeight {
		return fmt.Errorf("select menuMaximumHeight must be at least menuMinimumHeight")
	}
	return nil
}

func validateIconPosition(name, value string) error {
	if value != "leading" && value != "trailing" {
		return fmt.Errorf("%s must be leading or trailing", name)
	}
	return nil
}
