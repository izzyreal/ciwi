package uidsl

import (
	"fmt"
	"strings"
)

type TypographyDocument struct {
	APIVersion string     `yaml:"apiVersion" json:"apiVersion"`
	Kind       string     `yaml:"kind" json:"kind"`
	Typography Typography `yaml:"typography" json:"typography"`
}

type Typography struct {
	Families map[string]TypographyFamily `yaml:"families" json:"families"`
	Weights  map[string]TypographyWeight `yaml:"weights" json:"weights"`
	Roles    map[string]TypographyRole   `yaml:"roles" json:"roles"`
}

type TypographyFamily struct {
	Web    string `yaml:"web" json:"web"`
	Native string `yaml:"native" json:"native"`
}

type TypographyWeight struct {
	Web    int `yaml:"web" json:"web"`
	Native int `yaml:"native" json:"native"`
}

type TypographyRole struct {
	Family     string  `yaml:"family" json:"family"`
	Size       float32 `yaml:"size" json:"size"`
	Weight     string  `yaml:"weight" json:"weight"`
	LineHeight float32 `yaml:"lineHeight,omitempty" json:"lineHeight,omitempty"`
}

var requiredTypographyRoles = []string{
	"body", "title", "job-title", "heading", "subtitle", "control", "badge",
	"detail", "detail-small", "empty-state", "table-header", "code", "code-inline",
	"output-summary", "output-meta", "output-label", "output-code",
}

func ParseTypography(payload []byte) (*TypographyDocument, error) {
	var document TypographyDocument
	if err := decodeStrict(payload, &document); err != nil {
		return nil, err
	}
	if err := document.Validate(); err != nil {
		return nil, err
	}
	return &document, nil
}

func (d *TypographyDocument) Validate() error {
	if d == nil {
		return fmt.Errorf("typography document is nil")
	}
	if d.APIVersion != APIVersion {
		return fmt.Errorf("unsupported apiVersion %q", d.APIVersion)
	}
	if d.Kind != "Typography" {
		return fmt.Errorf("kind must be Typography, got %q", d.Kind)
	}
	for _, family := range []string{"body", "mono"} {
		definition, ok := d.Typography.Families[family]
		if !ok || strings.TrimSpace(definition.Web) == "" || strings.TrimSpace(definition.Native) == "" {
			return fmt.Errorf("typography family %q must define web and native names", family)
		}
	}
	for name, family := range d.Typography.Families {
		if !identifierPattern.MatchString(name) || strings.TrimSpace(family.Web) == "" || strings.TrimSpace(family.Native) == "" {
			return fmt.Errorf("typography family %q must define web and native names", name)
		}
	}
	for name, weight := range d.Typography.Weights {
		if !identifierPattern.MatchString(name) || weight.Web < 1 || weight.Web > 1000 || weight.Native < 1 || weight.Native > 1000 {
			return fmt.Errorf("typography weight %q must define web and native values from 1 to 1000", name)
		}
	}
	for _, weight := range []string{"regular", "medium", "strong", "title"} {
		if _, ok := d.Typography.Weights[weight]; !ok {
			return fmt.Errorf("typography weight %q is required", weight)
		}
	}
	for name, role := range d.Typography.Roles {
		if !identifierPattern.MatchString(name) {
			return fmt.Errorf("typography role %q is invalid", name)
		}
		if _, ok := d.Typography.Families[role.Family]; !ok {
			return fmt.Errorf("typography role %q references unknown family %q", name, role.Family)
		}
		if _, ok := d.Typography.Weights[role.Weight]; !ok {
			return fmt.Errorf("typography role %q references unknown weight %q", name, role.Weight)
		}
		if role.Size <= 0 || role.Size > 256 {
			return fmt.Errorf("typography role %q size must be greater than zero and at most 256", name)
		}
		if role.LineHeight < 0 || role.LineHeight > 4 {
			return fmt.Errorf("typography role %q lineHeight must be from 0 to 4", name)
		}
	}
	for _, role := range requiredTypographyRoles {
		if _, ok := d.Typography.Roles[role]; !ok {
			return fmt.Errorf("typography role %q is required", role)
		}
	}
	return nil
}
