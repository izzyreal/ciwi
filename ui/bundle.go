// Package ui owns the renderer-neutral screen and theme resources embedded in
// every ciwi UI client. The server does not send executable presentation code
// to native clients.
package ui

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"sync"

	"github.com/izzyreal/ciwi/pkg/uidsl"
)

//go:embed actions.yaml controls.yaml routes.yaml typography.yaml assets/* screens/*.yaml themes/*.yaml
var resources embed.FS

var (
	revisionOnce sync.Once
	revision     string
)

// Revision identifies the exact set of renderer-neutral resources embedded in
// this binary. Browser clients use it to cache immutable UI contracts safely.
func Revision() string {
	revisionOnce.Do(func() {
		hash := sha256.New()
		err := fs.WalkDir(resources, ".", func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				return nil
			}
			payload, err := resources.ReadFile(path)
			if err != nil {
				return err
			}
			_, _ = hash.Write([]byte(path))
			_, _ = hash.Write([]byte{0})
			_, _ = hash.Write(payload)
			_, _ = hash.Write([]byte{0})
			return nil
		})
		if err != nil {
			revision = "unavailable"
			return
		}
		revision = hex.EncodeToString(hash.Sum(nil))[:16]
	})
	return revision
}

func Read(path string) ([]byte, error) {
	clean := strings.TrimPrefix(path, "/")
	payload, err := resources.ReadFile(clean)
	if err != nil {
		return nil, fmt.Errorf("read embedded UI resource %q: %w", clean, err)
	}
	return payload, nil
}

func LoadScreen(name string) (*uidsl.ScreenDocument, error) {
	payload, err := Read("screens/" + name + ".yaml")
	if err != nil {
		return nil, err
	}
	document, err := uidsl.ParseScreen(payload)
	if err != nil {
		return nil, fmt.Errorf("parse screen %q: %w", name, err)
	}
	catalog, err := LoadActionCatalog()
	if err != nil {
		return nil, err
	}
	if err := document.ValidateScreenActions(catalog); err != nil {
		return nil, fmt.Errorf("validate screen %q actions: %w", name, err)
	}
	return document, nil
}

func LoadActionCatalog() (*uidsl.ActionCatalogDocument, error) {
	payload, err := Read("actions.yaml")
	if err != nil {
		return nil, err
	}
	catalog, err := uidsl.ParseActionCatalog(payload)
	if err != nil {
		return nil, fmt.Errorf("parse action catalog: %w", err)
	}
	return catalog, nil
}

func LoadRoutes() (*uidsl.RouteDocument, error) {
	payload, err := Read("routes.yaml")
	if err != nil {
		return nil, err
	}
	document, err := uidsl.ParseRoutes(payload)
	if err != nil {
		return nil, fmt.Errorf("parse route catalog: %w", err)
	}
	return document, nil
}

func LoadTypography() (*uidsl.TypographyDocument, error) {
	payload, err := Read("typography.yaml")
	if err != nil {
		return nil, err
	}
	document, err := uidsl.ParseTypography(payload)
	if err != nil {
		return nil, fmt.Errorf("parse typography: %w", err)
	}
	return document, nil
}

func LoadControls() (*uidsl.ControlsDocument, error) {
	payload, err := Read("controls.yaml")
	if err != nil {
		return nil, err
	}
	document, err := uidsl.ParseControls(payload)
	if err != nil {
		return nil, fmt.Errorf("parse controls: %w", err)
	}
	return document, nil
}

func LoadThemes() ([]*uidsl.ThemeDocument, error) {
	entries, err := fs.ReadDir(resources, "themes")
	if err != nil {
		return nil, fmt.Errorf("list embedded themes: %w", err)
	}
	themes := make([]*uidsl.ThemeDocument, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}
		payload, err := resources.ReadFile("themes/" + entry.Name())
		if err != nil {
			return nil, err
		}
		theme, err := uidsl.ParseTheme(payload)
		if err != nil {
			return nil, fmt.Errorf("parse theme %q: %w", entry.Name(), err)
		}
		themes = append(themes, theme)
	}
	sort.Slice(themes, func(i, j int) bool { return themes[i].Metadata.Name < themes[j].Metadata.Name })
	return themes, nil
}
