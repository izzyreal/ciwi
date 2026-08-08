package uidsl

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// ActionCatalogDocument describes interaction semantics shared by all UI
// renderers. It intentionally excludes transport, timeout, and visual policy.
type ActionCatalogDocument struct {
	APIVersion string       `yaml:"apiVersion" json:"apiVersion"`
	Kind       string       `yaml:"kind" json:"kind"`
	Actions    []ActionSpec `yaml:"actions" json:"actions"`
}

type ActionSpec struct {
	Command          string `yaml:"command" json:"command"`
	Class            string `yaml:"class" json:"class"`
	Scope            string `yaml:"scope,omitempty" json:"scope,omitempty"`
	Pending          string `yaml:"pending,omitempty" json:"pending,omitempty"`
	Persistence      string `yaml:"persistence,omitempty" json:"persistence,omitempty"`
	Navigation       string `yaml:"navigation,omitempty" json:"navigation,omitempty"`
	RefreshOnSuccess bool   `yaml:"refreshOnSuccess,omitempty" json:"refreshOnSuccess,omitempty"`
}

const (
	ActionClassLocal    = "local"
	ActionClassQuery    = "query"
	ActionClassMutation = "mutation"

	ActionPersistenceNone    = "none"
	ActionPersistenceSafe    = "safe"
	ActionPersistenceReceipt = "receipt-only"
	ActionNavigationCancel   = "cancel"
	ActionNavigationContinue = "continue"
)

var actionScopeBindingPattern = regexp.MustCompile(`\{\{\s*([a-zA-Z][a-zA-Z0-9]*)\s*\}\}`)

func ParseActionCatalog(payload []byte) (*ActionCatalogDocument, error) {
	var document ActionCatalogDocument
	if err := decodeStrict(payload, &document); err != nil {
		return nil, err
	}
	if err := document.Validate(); err != nil {
		return nil, err
	}
	return &document, nil
}

func (d *ActionCatalogDocument) Validate() error {
	if d == nil {
		return errors.New("action catalog is nil")
	}
	if d.APIVersion != APIVersion {
		return fmt.Errorf("unsupported apiVersion %q", d.APIVersion)
	}
	if d.Kind != "ActionCatalog" {
		return fmt.Errorf("kind must be ActionCatalog, got %q", d.Kind)
	}
	seen := map[string]bool{}
	for index := range d.Actions {
		spec := &d.Actions[index]
		if !commands[spec.Command] {
			return fmt.Errorf("actions[%d].command %q is not supported", index, spec.Command)
		}
		if seen[spec.Command] {
			return fmt.Errorf("duplicate action command %q", spec.Command)
		}
		seen[spec.Command] = true
		switch spec.Class {
		case ActionClassLocal:
			if spec.Persistence == "" {
				spec.Persistence = ActionPersistenceNone
			}
		case ActionClassQuery:
			if spec.Navigation == "" {
				spec.Navigation = ActionNavigationCancel
			}
			if spec.Persistence == "" {
				spec.Persistence = ActionPersistenceNone
			}
		case ActionClassMutation:
			if strings.TrimSpace(spec.Scope) == "" {
				return fmt.Errorf("actions[%d].scope is required for mutation %q", index, spec.Command)
			}
			if spec.Navigation == "" {
				spec.Navigation = ActionNavigationContinue
			}
			if spec.Persistence == "" {
				spec.Persistence = ActionPersistenceSafe
			}
		default:
			return fmt.Errorf("actions[%d].class %q is not supported", index, spec.Class)
		}
		if spec.RefreshOnSuccess && spec.Class != ActionClassMutation {
			return fmt.Errorf("actions[%d].refreshOnSuccess requires a mutation action", index)
		}
		if spec.Persistence != ActionPersistenceNone && spec.Persistence != ActionPersistenceSafe && spec.Persistence != ActionPersistenceReceipt {
			return fmt.Errorf("actions[%d].persistence %q is not supported", index, spec.Persistence)
		}
		if spec.Navigation != "" && spec.Navigation != ActionNavigationCancel && spec.Navigation != ActionNavigationContinue {
			return fmt.Errorf("actions[%d].navigation %q is not supported", index, spec.Navigation)
		}
	}
	missing := make([]string, 0)
	for command := range commands {
		if !seen[command] {
			missing = append(missing, command)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return fmt.Errorf("action catalog is missing commands: %s", strings.Join(missing, ", "))
	}
	return nil
}

func (d *ActionCatalogDocument) Spec(command string) (ActionSpec, bool) {
	if d == nil {
		return ActionSpec{}, false
	}
	for _, spec := range d.Actions {
		if spec.Command == command {
			return spec, true
		}
	}
	return ActionSpec{}, false
}

func (s ActionSpec) ResolveScope(arguments map[string]string) string {
	scope := actionScopeBindingPattern.ReplaceAllStringFunc(s.Scope, func(match string) string {
		parts := actionScopeBindingPattern.FindStringSubmatch(match)
		if len(parts) != 2 {
			return ""
		}
		return strings.TrimSpace(arguments[parts[1]])
	})
	if strings.TrimSpace(scope) == "" {
		return s.Command
	}
	return scope
}
