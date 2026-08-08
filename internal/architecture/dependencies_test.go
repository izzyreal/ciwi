package architecture

import (
	"fmt"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

const modulePath = "github.com/izzyreal/ciwi"

func TestLayerImportBoundaries(t *testing.T) {
	root := repositoryRoot(t)
	graph := loadRepositoryImportGraph(t, root)
	cases := []struct {
		packagePrefix string
		forbidden     []string
	}{
		{modulePath + "/internal/domain", []string{modulePath + "/internal/"}},
		{modulePath + "/internal/application", []string{
			modulePath + "/internal/adapters/",
			modulePath + "/internal/protocol",
			modulePath + "/internal/server",
			modulePath + "/internal/store",
			modulePath + "/internal/presentation",
		}},
		{modulePath + "/internal/presentation", []string{
			modulePath + "/internal/adapters/",
			modulePath + "/internal/protocol",
			modulePath + "/internal/server",
			modulePath + "/internal/store",
		}},
		{modulePath + "/pkg/uidsl", []string{
			modulePath + "/internal/",
			modulePath + "/pkg/cnp",
		}},
		{modulePath + "/ui", []string{
			modulePath + "/internal/",
			modulePath + "/pkg/cnp",
		}},
	}
	for _, tc := range cases {
		t.Run(strings.TrimPrefix(tc.packagePrefix, modulePath+"/"), func(t *testing.T) {
			for _, packagePath := range packagesAtOrBelow(graph, tc.packagePrefix) {
				assertNoForbiddenTransitiveImports(t, graph, packagePath, tc.forbidden)
			}
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

func loadRepositoryImportGraph(t *testing.T, root string) map[string][]string {
	t.Helper()
	imports := make(map[string]map[string]bool)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path != root && (entry.Name() == ".git" || entry.Name() == "vendor" || strings.HasPrefix(entry.Name(), ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}
		relativeDirectory, err := filepath.Rel(root, filepath.Dir(path))
		if err != nil {
			return err
		}
		packagePath := modulePath
		if relativeDirectory != "." {
			packagePath += "/" + filepath.ToSlash(relativeDirectory)
		}
		if imports[packagePath] == nil {
			imports[packagePath] = make(map[string]bool)
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, imported := range file.Imports {
			value, err := strconv.Unquote(imported.Path.Value)
			if err != nil {
				return err
			}
			if value == modulePath || strings.HasPrefix(value, modulePath+"/") {
				imports[packagePath][value] = true
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	graph := make(map[string][]string, len(imports))
	for packagePath, packageImports := range imports {
		for imported := range packageImports {
			graph[packagePath] = append(graph[packagePath], imported)
		}
		sort.Strings(graph[packagePath])
	}
	return graph
}

func packagesAtOrBelow(graph map[string][]string, prefix string) []string {
	packages := make([]string, 0)
	for packagePath := range graph {
		if packagePath == prefix || strings.HasPrefix(packagePath, prefix+"/") {
			packages = append(packages, packagePath)
		}
	}
	sort.Strings(packages)
	return packages
}

func assertNoForbiddenTransitiveImports(t *testing.T, graph map[string][]string, start string, forbidden []string) {
	t.Helper()
	type visit struct {
		packagePath string
		chain       []string
	}
	queue := []visit{{packagePath: start, chain: []string{start}}}
	seen := map[string]bool{start: true}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for _, imported := range graph[current.packagePath] {
			chain := append(append([]string(nil), current.chain...), imported)
			for _, prefix := range forbidden {
				if strings.HasPrefix(imported, prefix) {
					t.Errorf("forbidden dependency from %s via %s", start, formatImportChain(chain))
					return
				}
			}
			if !seen[imported] {
				seen[imported] = true
				queue = append(queue, visit{packagePath: imported, chain: chain})
			}
		}
	}
}

func formatImportChain(chain []string) string {
	short := make([]string, len(chain))
	for i, packagePath := range chain {
		short[i] = strings.TrimPrefix(packagePath, modulePath+"/")
	}
	return fmt.Sprintf("[%s]", strings.Join(short, " -> "))
}
