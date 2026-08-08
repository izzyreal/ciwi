package presentation

import (
	"fmt"
	"net/url"
	"path"
	"sort"
	"strings"

	"github.com/izzyreal/ciwi/internal/domain"
)

type mutableTreeNode struct {
	name     string
	path     string
	files    []domain.JobArtifact
	children map[string]*mutableTreeNode
	total    int
	covered  int
	percent  float64
}

func presentArtifactTree(artifacts []domain.JobArtifact) []TreeNodeView {
	root := &mutableTreeNode{children: map[string]*mutableTreeNode{}}
	for _, artifact := range artifacts {
		artifact.Path = normalizeTreePath(artifact.Path)
		if artifact.Path == "" {
			artifact.Path = "(unnamed artifact)"
		}
		parts := strings.Split(artifact.Path, "/")
		node := root
		for _, segment := range parts[:len(parts)-1] {
			child := node.children[segment]
			if child == nil {
				childPath := segment
				if node.path != "" {
					childPath = node.path + "/" + segment
				}
				child = &mutableTreeNode{name: segment, path: childPath, children: map[string]*mutableTreeNode{}}
				node.children[segment] = child
			}
			node = child
		}
		artifact.Path = parts[len(parts)-1]
		node.files = append(node.files, artifact)
	}
	return presentArtifactChildren(root, 0)
}

func presentArtifactChildren(node *mutableTreeNode, depth int) []TreeNodeView {
	names := sortedTreeKeys(node.children)
	result := make([]TreeNodeView, 0, len(names)+len(node.files))
	for _, name := range names {
		child := node.children[name]
		result = append(result, TreeNodeView{
			Key: "dir:" + child.path, Label: child.name,
			ActionLabel: "Download .zip", ActionKind: "prefix", ActionPath: child.path,
			DefaultExpanded: depth == 0, Children: presentArtifactChildren(child, depth+1),
		})
	}
	sort.SliceStable(node.files, func(i, j int) bool { return node.files[i].Path < node.files[j].Path })
	for _, artifact := range node.files {
		fullPath := artifact.Path
		if node.path != "" {
			fullPath = node.path + "/" + artifact.Path
		}
		result = append(result, TreeNodeView{
			Key: "file:" + fullPath, Label: artifact.Path, Detail: formatBytes(artifact.SizeBytes),
			ActionLabel: "Download", ActionKind: "file", ActionPath: fullPath,
		})
	}
	return result
}

func presentTestTree(report *domain.JobTestReport, metadata domain.ExecutionMetadata) []TreeNodeView {
	if report == nil {
		return nil
	}
	suites := append([]domain.JobTestSuite(nil), report.Suites...)
	sort.SliceStable(suites, func(i, j int) bool {
		if suites[i].Failed != suites[j].Failed {
			return suites[i].Failed > suites[j].Failed
		}
		return suites[i].Name < suites[j].Name
	})
	result := make([]TreeNodeView, 0, len(suites))
	for suiteIndex, suite := range suites {
		label := strings.TrimSpace(suite.Name)
		if label == "" {
			label = DeclarativeDefaultLabel(suite.Format, fmt.Sprintf("Suite %d", suiteIndex+1))
		}
		packages := map[string][]domain.JobTestCase{}
		for _, testCase := range suite.Cases {
			packageName := DeclarativeDefaultLabel(testCase.Package, "(root)")
			packages[packageName] = append(packages[packageName], testCase)
		}
		packageNames := make([]string, 0, len(packages))
		for packageName := range packages {
			packageNames = append(packageNames, packageName)
		}
		sort.Strings(packageNames)
		children := make([]TreeNodeView, 0, len(packageNames))
		for _, packageName := range packageNames {
			cases := packages[packageName]
			sort.SliceStable(cases, func(i, j int) bool {
				left, right := testStatusRank(cases[i].Status), testStatusRank(cases[j].Status)
				if left != right {
					return left < right
				}
				return cases[i].Name < cases[j].Name
			})
			caseNodes := make([]TreeNodeView, 0, len(cases))
			passed, failed, skipped := 0, 0, 0
			for caseIndex, testCase := range cases {
				status := strings.ToLower(strings.TrimSpace(testCase.Status))
				switch status {
				case "pass":
					passed++
				case "fail":
					failed++
				case "skip":
					skipped++
				}
				detail := status
				if testCase.DurationSeconds >= 0 {
					detail = strings.TrimSpace(detail + " · " + fmt.Sprintf("%.3fs", testCase.DurationSeconds))
				}
				caseNodes = append(caseNodes, TreeNodeView{
					Key: fmt.Sprintf("case:%d:%s:%d", suiteIndex, packageName, caseIndex), Label: DeclarativeDefaultLabel(testCase.Name, "(unnamed test)"),
					Detail: detail, Tone: testStatusTone(status), Link: testCaseSourceURL(testCase, metadata),
					FilterValues: []string{"all", status},
				})
			}
			packageKey := fmt.Sprintf("suite:%d:package:%s", suiteIndex, packageName)
			children = append(children, TreeNodeView{
				Key: packageKey, Label: packageName,
				Detail: fmt.Sprintf("%d total · %d passed · %d failed · %d skipped", len(cases), passed, failed, skipped),
				Tone:   reportTone(len(cases), failed), Children: caseNodes,
			})
		}
		suiteKey := fmt.Sprintf("suite:%d:%s", suiteIndex, label)
		result = append(result, TreeNodeView{
			Key: suiteKey, Label: label,
			Detail: strings.TrimSpace(strings.TrimSpace(suite.Format) + " · " + formatTestCounts(suite.Total, suite.Passed, suite.Failed, suite.Skipped)),
			Tone:   reportTone(suite.Total, suite.Failed), DefaultExpanded: true, Children: children,
		})
	}
	return result
}

func presentCoverageTree(files []domain.JobCoverageFile) []TreeNodeView {
	root := &mutableTreeNode{children: map[string]*mutableTreeNode{}}
	for _, file := range files {
		filePath := normalizeTreePath(file.Path)
		if filePath == "" {
			continue
		}
		total, covered := coverageFileTotals(file)
		parts := strings.Split(filePath, "/")
		node := root
		for index, segment := range parts {
			child := node.children[segment]
			if child == nil {
				childPath := strings.Join(parts[:index+1], "/")
				child = &mutableTreeNode{name: segment, path: childPath, children: map[string]*mutableTreeNode{}}
				node.children[segment] = child
			}
			child.total += total
			child.covered += covered
			if index == len(parts)-1 {
				child.percent = file.Percent
			}
			node = child
		}
	}
	return presentCoverageChildren(root, 0)
}

func presentCoverageChildren(node *mutableTreeNode, depth int) []TreeNodeView {
	names := sortedTreeKeys(node.children)
	result := make([]TreeNodeView, 0, len(names))
	for _, name := range names {
		child := node.children[name]
		detail := fmt.Sprintf("%.2f%% · %d/%d", coveragePercent(child.covered, child.total, child.percent), child.covered, child.total)
		children := presentCoverageChildren(child, depth+1)
		entry := TreeNodeView{Key: "coverage:" + child.path, Label: child.name, Detail: detail, Children: children}
		if len(children) > 0 {
			entry.DefaultExpanded = depth == 0
		}
		result = append(result, entry)
	}
	return result
}

func sortedTreeKeys(values map[string]*mutableTreeNode) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func normalizeTreePath(value string) string {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
	value = strings.TrimPrefix(path.Clean("/"+value), "/")
	if value == "." {
		return ""
	}
	return value
}

func testStatusRank(status string) int {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "fail":
		return 0
	case "skip":
		return 1
	case "pass":
		return 2
	default:
		return 3
	}
}

func testStatusTone(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "fail":
		return "danger"
	case "pass":
		return "success"
	case "skip":
		return "warning"
	default:
		return "muted"
	}
}

func testCaseSourceURL(testCase domain.JobTestCase, metadata domain.ExecutionMetadata) string {
	repository := metadata.Value(domain.ExecutionMetadataPipelineSourceRepo)
	ref := metadata.Value(domain.ExecutionMetadataPipelineSourceRefResolved)
	if ref == "" {
		ref = metadata.Value(domain.ExecutionMetadataPipelineSourceRef)
	}
	host, repositoryPath := parseRepository(repository)
	context := testSourceContext{host: host, repositoryPath: repositoryPath, ref: ref}
	filePath := relativeTestSourcePath(testCase, context)
	if filePath == "" {
		if externalContext, externalPath, ok := externalTestSource(testCase, context); ok {
			context, filePath = externalContext, externalPath
		}
	}
	if blobURL := testBlobURL(context, filePath, testCase.Line); blobURL != "" {
		return blobURL
	}
	if strings.TrimSpace(testCase.Name) == "" || context.host == "" || context.repositoryPath == "" {
		externalContext, _, ok := parseExternalSourcePath(testCase.Package)
		if !ok || strings.TrimSpace(testCase.Name) == "" {
			return ""
		}
		if externalContext.host == context.host && externalContext.repositoryPath == context.repositoryPath && context.ref != "" {
			externalContext.ref = context.ref
		}
		context = externalContext
	}
	packagePath := relativeTestPackagePath(testCase.Package, context)
	query := `"` + strings.TrimSpace(testCase.Name) + `"`
	if packagePath != "" {
		query += " path:" + packagePath
	}
	switch context.host {
	case "github.com":
		searchURL := "https://github.com/" + context.repositoryPath + "/search?q=" + url.QueryEscape(query) + "&type=code"
		if context.ref != "" {
			searchURL += "&ref=" + url.QueryEscape(context.ref)
		}
		return searchURL
	case "gitlab.com":
		return "https://gitlab.com/" + context.repositoryPath + "/-/search?search=" + url.QueryEscape(query) + "&scope=blobs"
	default:
		return ""
	}
}

type testSourceContext struct {
	host           string
	repositoryPath string
	ref            string
}

func relativeTestSourcePath(testCase domain.JobTestCase, context testSourceContext) string {
	filePath := normalizeTreePath(testCase.File)
	if filePath == "" || context.repositoryPath == "" {
		return ""
	}
	prefixes := repositoryPathPrefixes(context.host, context.repositoryPath)
	matchedPrefix := false
	for _, prefix := range prefixes {
		if strings.HasPrefix(filePath, prefix) {
			filePath = strings.TrimPrefix(filePath, prefix)
			matchedPrefix = true
			break
		}
	}
	if !strings.Contains(filePath, "/") {
		if packagePath := relativeTestPackagePath(testCase.Package, context); packagePath != "" {
			filePath = packagePath + "/" + filePath
		}
	} else if !matchedPrefix && strings.HasPrefix(filePath, "Users/") {
		// Local absolute paths cannot be translated safely; fall back to code search.
		return ""
	}
	return normalizeTreePath(filePath)
}

func relativeTestPackagePath(packageName string, context testSourceContext) string {
	packagePath := normalizeTreePath(packageName)
	for _, prefix := range repositoryPathPrefixes(context.host, context.repositoryPath) {
		if strings.HasPrefix(packagePath, prefix) {
			return normalizeTreePath(strings.TrimPrefix(packagePath, prefix))
		}
	}
	return ""
}

func repositoryPathPrefixes(host, repositoryPath string) []string {
	return []string{host + "/" + repositoryPath + "/", repositoryPath + "/", "github.com/" + repositoryPath + "/", "gitlab.com/" + repositoryPath + "/"}
}

func externalTestSource(testCase domain.JobTestCase, primary testSourceContext) (testSourceContext, string, bool) {
	if context, subPath, ok := parseExternalSourcePath(testCase.File); ok && subPath != "" {
		if context.host == primary.host && context.repositoryPath == primary.repositoryPath && primary.ref != "" {
			context.ref = primary.ref
		}
		return context, subPath, true
	}
	context, packagePath, ok := parseExternalSourcePath(testCase.Package)
	if !ok {
		return testSourceContext{}, "", false
	}
	if context.host == primary.host && context.repositoryPath == primary.repositoryPath && primary.ref != "" {
		context.ref = primary.ref
	}
	filePath := normalizeTreePath(testCase.File)
	if filePath == "" {
		return testSourceContext{}, "", false
	}
	if !strings.Contains(filePath, "/") && packagePath != "" {
		filePath = packagePath + "/" + filePath
	}
	return context, filePath, true
}

func parseExternalSourcePath(raw string) (testSourceContext, string, bool) {
	parts := strings.Split(normalizeTreePath(raw), "/")
	if len(parts) < 4 || (parts[0] != "github.com" && parts[0] != "gitlab.com") {
		return testSourceContext{}, "", false
	}
	return testSourceContext{host: parts[0], repositoryPath: parts[1] + "/" + parts[2], ref: "HEAD"}, strings.Join(parts[3:], "/"), true
}

func testBlobURL(context testSourceContext, filePath string, line int) string {
	if context.host == "" || context.repositoryPath == "" || context.ref == "" || filePath == "" {
		return ""
	}
	fragment := ""
	if line > 0 {
		fragment = fmt.Sprintf("#L%d", line)
	}
	switch context.host {
	case "github.com":
		return "https://github.com/" + context.repositoryPath + "/blob/" + url.PathEscape(context.ref) + "/" + escapeURLPath(filePath) + fragment
	case "gitlab.com":
		return "https://gitlab.com/" + context.repositoryPath + "/-/blob/" + url.PathEscape(context.ref) + "/" + escapeURLPath(filePath) + fragment
	default:
		return ""
	}
}

func parseRepository(repository string) (string, string) {
	repository = strings.TrimSuffix(strings.TrimSpace(repository), ".git")
	if strings.HasPrefix(repository, "git@") {
		parts := strings.SplitN(strings.TrimPrefix(repository, "git@"), ":", 2)
		if len(parts) == 2 {
			return strings.ToLower(parts[0]), strings.Trim(parts[1], "/")
		}
	}
	parsed, err := url.Parse(repository)
	if err != nil || parsed.Host == "" {
		return "", ""
	}
	return strings.ToLower(parsed.Hostname()), strings.Trim(strings.TrimSuffix(parsed.Path, ".git"), "/")
}

func escapeURLPath(value string) string {
	parts := strings.Split(value, "/")
	for index := range parts {
		parts[index] = url.PathEscape(parts[index])
	}
	return strings.Join(parts, "/")
}
