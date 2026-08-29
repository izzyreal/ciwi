package config

import (
	"bytes"
	"fmt"
	"path"
	"strings"
)

// IncludeReader reads a repository-relative path from the same source revision
// as the root configuration file.
type IncludeReader func(repoPath string) ([]byte, error)

// ExpandIncludes recursively expands standalone "!include path" lines. The
// leading spaces from the directive are prefixed to every non-empty line in
// the included file so YAML fragments can be authored from column zero.
func ExpandIncludes(data []byte, sourcePath string, read IncludeReader) ([]byte, error) {
	if read == nil {
		return nil, fmt.Errorf("expand includes in %q: include reader is required", sourcePath)
	}
	root, err := canonicalRootPath(sourcePath)
	if err != nil {
		return nil, err
	}
	return expandIncludes(data, root, read, []string{root})
}

func expandIncludes(data []byte, sourcePath string, read IncludeReader, stack []string) ([]byte, error) {
	var out bytes.Buffer
	lineNumber := 0
	for offset := 0; offset < len(data); {
		lineNumber++
		relativeNewline := bytes.IndexByte(data[offset:], '\n')
		end := len(data)
		hasNewline := false
		if relativeNewline >= 0 {
			end = offset + relativeNewline
			hasNewline = true
		}

		line := data[offset:end]
		directiveLine := bytes.TrimSuffix(line, []byte{'\r'})
		indent, includePath, matched, err := parseIncludeDirective(directiveLine)
		if err != nil {
			return nil, fmt.Errorf("invalid include directive in %s:%d (include chain: %s): %w", sourcePath, lineNumber, strings.Join(stack, " -> "), err)
		}
		if !matched {
			out.Write(line)
			if hasNewline {
				out.WriteByte('\n')
			}
			offset = end
			if hasNewline {
				offset++
			}
			continue
		}

		resolved, err := resolveIncludePath(sourcePath, includePath)
		if err != nil {
			return nil, fmt.Errorf("invalid include in %s:%d (include chain: %s): %w", sourcePath, lineNumber, strings.Join(stack, " -> "), err)
		}
		if cycleAt := indexPath(stack, resolved); cycleAt >= 0 {
			chain := append(append([]string(nil), stack...), resolved)
			return nil, fmt.Errorf("include cycle in %s:%d: %s", sourcePath, lineNumber, strings.Join(chain, " -> "))
		}

		included, err := read(resolved)
		if err != nil {
			chain := append(append([]string(nil), stack...), resolved)
			return nil, fmt.Errorf("read include %q referenced by %s:%d (include chain: %s): %w", resolved, sourcePath, lineNumber, strings.Join(chain, " -> "), err)
		}
		expanded, err := expandIncludes(included, resolved, read, append(stack, resolved))
		if err != nil {
			return nil, err
		}
		writeIndented(&out, expanded, indent)
		if hasNewline && (len(expanded) == 0 || expanded[len(expanded)-1] != '\n') {
			out.WriteByte('\n')
		}

		offset = end
		if hasNewline {
			offset++
		}
	}
	return out.Bytes(), nil
}

func parseIncludeDirective(line []byte) (indent, includePath string, matched bool, err error) {
	indentLength := 0
	for indentLength < len(line) && line[indentLength] == ' ' {
		indentLength++
	}
	rest := string(line[indentLength:])
	const directive = "!include"
	if !strings.HasPrefix(rest, directive) {
		return "", "", false, nil
	}
	if len(rest) > len(directive) && rest[len(directive)] != ' ' && rest[len(directive)] != '\t' {
		return "", "", false, nil
	}
	fields := strings.Fields(rest)
	if len(fields) != 2 || fields[0] != directive {
		return "", "", true, fmt.Errorf("expected !include followed by one whitespace-free path")
	}
	return string(line[:indentLength]), fields[1], true, nil
}

func canonicalRootPath(sourcePath string) (string, error) {
	if strings.Contains(sourcePath, "\\") {
		return "", fmt.Errorf("config path %q must use forward slashes", sourcePath)
	}
	if sourcePath == "" || path.IsAbs(sourcePath) {
		return "", fmt.Errorf("config path %q must be repository-relative", sourcePath)
	}
	clean := path.Clean(sourcePath)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("config path %q escapes the repository", sourcePath)
	}
	return clean, nil
}

func resolveIncludePath(sourcePath, includePath string) (string, error) {
	if strings.Contains(includePath, "\\") {
		return "", fmt.Errorf("include path %q must use forward slashes", includePath)
	}
	if includePath == "" || path.IsAbs(includePath) {
		return "", fmt.Errorf("include path %q must be relative", includePath)
	}
	resolved := path.Clean(path.Join(path.Dir(sourcePath), includePath))
	if resolved == "." || resolved == ".." || strings.HasPrefix(resolved, "../") {
		return "", fmt.Errorf("include path %q escapes the repository", includePath)
	}
	return resolved, nil
}

func indexPath(paths []string, target string) int {
	for i, candidate := range paths {
		if candidate == target {
			return i
		}
	}
	return -1
}

func writeIndented(out *bytes.Buffer, data []byte, indent string) {
	for offset := 0; offset < len(data); {
		relativeNewline := bytes.IndexByte(data[offset:], '\n')
		end := len(data)
		hasNewline := false
		if relativeNewline >= 0 {
			end = offset + relativeNewline
			hasNewline = true
		}
		line := data[offset:end]
		if len(line) > 0 {
			out.WriteString(indent)
			out.Write(line)
		}
		if hasNewline {
			out.WriteByte('\n')
		}
		offset = end
		if hasNewline {
			offset++
		}
	}
}
