package architecture

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestLayerImportBoundaries(t *testing.T) {
	root := repositoryRoot(t)
	cases := []struct {
		directory string
		forbidden []string
	}{
		{"internal/domain", []string{"github.com/izzyreal/ciwi/internal/"}},
		{"internal/application", []string{
			"github.com/izzyreal/ciwi/internal/adapters/",
			"github.com/izzyreal/ciwi/internal/protocol",
			"github.com/izzyreal/ciwi/internal/server",
			"github.com/izzyreal/ciwi/internal/store",
			"github.com/izzyreal/ciwi/internal/presentation",
		}},
		{"internal/presentation", []string{
			"github.com/izzyreal/ciwi/internal/adapters/",
			"github.com/izzyreal/ciwi/internal/protocol",
			"github.com/izzyreal/ciwi/internal/server",
			"github.com/izzyreal/ciwi/internal/store",
		}},
		{"pkg/uidsl", []string{
			"github.com/izzyreal/ciwi/internal/",
			"github.com/izzyreal/ciwi/pkg/cnp",
		}},
		{"ui", []string{
			"github.com/izzyreal/ciwi/internal/",
			"github.com/izzyreal/ciwi/pkg/cnp",
		}},
	}
	for _, tc := range cases {
		t.Run(tc.directory, func(t *testing.T) {
			assertImportsDoNotContain(t, filepath.Join(root, tc.directory), tc.forbidden)
		})
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Clean(filepath.Join(dir, "..", ".."))
}

func assertImportsDoNotContain(t *testing.T, directory string, forbidden []string) {
	t.Helper()
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		path := filepath.Join(directory, entry.Name())
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imported := range file.Imports {
			value, err := strconv.Unquote(imported.Path.Value)
			if err != nil {
				t.Fatal(err)
			}
			for _, prefix := range forbidden {
				if strings.HasPrefix(value, prefix) {
					t.Errorf("%s imports forbidden package %s", path, value)
				}
			}
		}
	}
}
