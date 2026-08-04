package uidsl

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

type ThemeDocument struct {
	APIVersion string   `yaml:"apiVersion" json:"apiVersion"`
	Kind       string   `yaml:"kind" json:"kind"`
	Metadata   Metadata `yaml:"metadata" json:"metadata"`
	Theme      Theme    `yaml:"theme" json:"theme"`
}

type Theme struct {
	Dark       bool                `yaml:"dark,omitempty" json:"dark,omitempty"`
	Colors     map[string]string   `yaml:"colors" json:"colors"`
	Gradients  map[string]Gradient `yaml:"gradients,omitempty" json:"gradients,omitempty"`
	Dimensions map[string]string   `yaml:"dimensions,omitempty" json:"dimensions,omitempty"`
}

type Gradient struct {
	Kind  string         `yaml:"kind" json:"kind"`
	Angle int            `yaml:"angle,omitempty" json:"angle,omitempty"`
	Stops []GradientStop `yaml:"stops" json:"stops"`
}

type GradientStop struct {
	Color    string `yaml:"color" json:"color"`
	Position int    `yaml:"position" json:"position"`
}

var colorPattern = regexp.MustCompile(`^#[0-9a-fA-F]{6}([0-9a-fA-F]{2})?$`)

var requiredColorTokens = []string{
	"background", "surface", "surface-subtle", "text", "text-muted",
	"accent", "accent-strong", "border", "success", "warning", "danger", "focus",
}

func ParseTheme(payload []byte) (*ThemeDocument, error) {
	var document ThemeDocument
	if err := decodeStrict(payload, &document); err != nil {
		return nil, err
	}
	if err := document.Validate(); err != nil {
		return nil, err
	}
	return &document, nil
}

func (d *ThemeDocument) Validate() error {
	if d == nil {
		return fmt.Errorf("theme document is nil")
	}
	if d.APIVersion != APIVersion {
		return fmt.Errorf("unsupported apiVersion %q", d.APIVersion)
	}
	if d.Kind != "Theme" {
		return fmt.Errorf("kind must be Theme, got %q", d.Kind)
	}
	if !identifierPattern.MatchString(d.Metadata.Name) {
		return fmt.Errorf("metadata.name %q is not a valid identifier", d.Metadata.Name)
	}
	for _, token := range requiredColorTokens {
		if !colorPattern.MatchString(d.Theme.Colors[token]) {
			return fmt.Errorf("theme color %q must be a six- or eight-digit hex color", token)
		}
	}
	for token, value := range d.Theme.Colors {
		if !colorPattern.MatchString(value) {
			return fmt.Errorf("theme color %q must be a six- or eight-digit hex color", token)
		}
	}
	for name, gradient := range d.Theme.Gradients {
		if gradient.Kind != "linear" && gradient.Kind != "radial" {
			return fmt.Errorf("gradient %q kind must be linear or radial", name)
		}
		if len(gradient.Stops) < 2 {
			return fmt.Errorf("gradient %q requires at least two stops", name)
		}
		previous := -1
		for i, stop := range gradient.Stops {
			if !colorPattern.MatchString(stop.Color) || stop.Position < 0 || stop.Position > 100 || stop.Position < previous {
				return fmt.Errorf("gradient %q stop %d is invalid", name, i)
			}
			previous = stop.Position
		}
	}
	for name, value := range d.Theme.Dimensions {
		parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 32)
		if err != nil || parsed < 0 {
			return fmt.Errorf("theme dimension %q must be a non-negative number", name)
		}
	}
	return nil
}
