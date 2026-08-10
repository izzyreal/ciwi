package uidsl

import "fmt"

type ControlsDocument struct {
	APIVersion string   `yaml:"apiVersion" json:"apiVersion"`
	Kind       string   `yaml:"kind" json:"kind"`
	Controls   Controls `yaml:"controls" json:"controls"`
}

type Controls struct {
	Button     ButtonControl     `yaml:"button" json:"button"`
	Badge      BadgeControl      `yaml:"badge" json:"badge"`
	Input      InputControl      `yaml:"input" json:"input"`
	Select     SelectControl     `yaml:"select" json:"select"`
	Disclosure DisclosureControl `yaml:"disclosure" json:"disclosure"`
	Progress   ProgressControl   `yaml:"progress" json:"progress"`
}

// PlatformMetric allows the shared control contract to account for the
// different box and text layout semantics used by browsers and Gio without
// scattering renderer-specific constants through either implementation.
type PlatformMetric struct {
	Web    float32 `yaml:"web" json:"web"`
	Native float32 `yaml:"native" json:"native"`
}

type ButtonControl struct {
	IconPosition  string         `yaml:"iconPosition" json:"iconPosition"`
	MinimumHeight PlatformMetric `yaml:"minimumHeight" json:"minimumHeight"`
	PaddingX      PlatformMetric `yaml:"paddingX" json:"paddingX"`
	PaddingY      PlatformMetric `yaml:"paddingY" json:"paddingY"`
	IconSize      PlatformMetric `yaml:"iconSize" json:"iconSize"`
	IconGap       PlatformMetric `yaml:"iconGap" json:"iconGap"`
	IconOnlySize  PlatformMetric `yaml:"iconOnlySize" json:"iconOnlySize"`
}

type BadgeControl struct {
	PaddingX      float32 `yaml:"paddingX" json:"paddingX"`
	PaddingY      float32 `yaml:"paddingY" json:"paddingY"`
	TintOpacity   float32 `yaml:"tintOpacity" json:"tintOpacity"`
	BorderOpacity float32 `yaml:"borderOpacity" json:"borderOpacity"`
}

type InputControl struct {
	MinimumHeight PlatformMetric `yaml:"minimumHeight" json:"minimumHeight"`
	PaddingX      PlatformMetric `yaml:"paddingX" json:"paddingX"`
	PaddingY      PlatformMetric `yaml:"paddingY" json:"paddingY"`
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

type DisclosureControl struct {
	ChevronPosition string  `yaml:"chevronPosition" json:"chevronPosition"`
	ChevronSize     float32 `yaml:"chevronSize" json:"chevronSize"`
	ChevronGap      float32 `yaml:"chevronGap" json:"chevronGap"`
}

// ProgressControl defines renderer-independent semantic progress visuals.
type ProgressControl struct {
	TintOpacity float32 `yaml:"tintOpacity" json:"tintOpacity"`
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
	if err := validateIconPosition("disclosure chevronPosition", d.Controls.Disclosure.ChevronPosition); err != nil {
		return err
	}
	positive := map[string]float32{
		"disclosure chevronSize":         d.Controls.Disclosure.ChevronSize,
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
		"badge paddingX":        d.Controls.Badge.PaddingX,
		"badge paddingY":        d.Controls.Badge.PaddingY,
		"disclosure chevronGap": d.Controls.Disclosure.ChevronGap,
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
	for name, metric := range map[string]PlatformMetric{
		"button minimumHeight": d.Controls.Button.MinimumHeight,
		"button iconSize":      d.Controls.Button.IconSize,
		"button iconOnlySize":  d.Controls.Button.IconOnlySize,
		"input minimumHeight":  d.Controls.Input.MinimumHeight,
	} {
		if metric.Web <= 0 || metric.Native <= 0 {
			return fmt.Errorf("%s must define positive web and native values", name)
		}
	}
	for name, metric := range map[string]PlatformMetric{
		"button paddingX": d.Controls.Button.PaddingX,
		"button paddingY": d.Controls.Button.PaddingY,
		"button iconGap":  d.Controls.Button.IconGap,
		"input paddingX":  d.Controls.Input.PaddingX,
		"input paddingY":  d.Controls.Input.PaddingY,
	} {
		if metric.Web < 0 || metric.Native < 0 {
			return fmt.Errorf("%s must define non-negative web and native values", name)
		}
	}
	if d.Controls.Select.MenuMaximumHeight < d.Controls.Select.MenuMinimumHeight {
		return fmt.Errorf("select menuMaximumHeight must be at least menuMinimumHeight")
	}
	if opacity := d.Controls.Progress.TintOpacity; opacity <= 0 || opacity > 1 {
		return fmt.Errorf("progress tintOpacity must be greater than zero and at most one")
	}
	for name, opacity := range map[string]float32{
		"badge tintOpacity":   d.Controls.Badge.TintOpacity,
		"badge borderOpacity": d.Controls.Badge.BorderOpacity,
	} {
		if opacity <= 0 || opacity > 1 {
			return fmt.Errorf("%s must be greater than zero and at most one", name)
		}
	}
	return nil
}

func validateIconPosition(name, value string) error {
	if value != "leading" && value != "trailing" {
		return fmt.Errorf("%s must be leading or trailing", name)
	}
	return nil
}
