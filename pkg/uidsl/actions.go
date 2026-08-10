package uidsl

import (
	"errors"
	"fmt"
	"regexp"
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
		if !identifierPattern.MatchString(spec.Command) {
			return fmt.Errorf("actions[%d].command %q is malformed", index, spec.Command)
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
	return nil
}

// ValidateScreenActions ensures that every command referenced by a screen is
// defined by the authoritative action catalog. ParseScreen validates syntax;
// bundle owners call this method to validate cross-document references.
func (d *ScreenDocument) ValidateScreenActions(catalog *ActionCatalogDocument) error {
	if d == nil {
		return errors.New("screen document is nil")
	}
	if catalog == nil {
		return errors.New("action catalog is nil")
	}
	var visit func(Node, string) error
	visit = func(node Node, path string) error {
		validate := func(actions []Action, actionPath string) error {
			for index, action := range actions {
				if _, ok := catalog.Spec(action.Command); !ok {
					return fmt.Errorf("%s[%d].command %q is not defined in the action catalog", actionPath, index, action.Command)
				}
			}
			return nil
		}
		if err := validate(node.Actions, path+".actions"); err != nil {
			return err
		}
		if node.GraphView != nil && node.GraphView.Root != nil {
			if err := validate(node.GraphView.Root.Actions, path+".graphView.root.actions"); err != nil {
				return err
			}
			for index, child := range node.GraphView.Details {
				if err := visit(child, fmt.Sprintf("%s.graphView.details[%d]", path, index)); err != nil {
					return err
				}
			}
		}
		if node.Disclosure != nil {
			for index, child := range node.Disclosure.Summary {
				if err := visit(child, fmt.Sprintf("%s.disclosure.summary[%d]", path, index)); err != nil {
					return err
				}
			}
		}
		for index, child := range node.Children {
			if err := visit(child, fmt.Sprintf("%s.children[%d]", path, index)); err != nil {
				return err
			}
		}
		return nil
	}
	return visit(d.Screen.Root, "screen.root")
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
