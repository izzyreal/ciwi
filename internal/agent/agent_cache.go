package agent

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/izzyreal/ciwi/internal/protocol"
)

type cachePolicy struct {
	pull bool
	push bool
}

func resolveJobCacheEnv(workDir, execDir string, job protocol.JobExecution, agentCapabilities map[string]string) (map[string]string, []string) {
	if len(job.Caches) == 0 {
		return nil, nil
	}
	cacheRoot := filepath.Join(workDir, "cache")
	if err := os.MkdirAll(cacheRoot, 0o755); err != nil {
		return nil, []string{fmt.Sprintf("warning=create_cache_root_failed err=%v", err)}
	}
	cacheEnv := map[string]string{}
	logs := make([]string, 0, len(job.Caches)*2)
	for _, spec := range job.Caches {
		cacheID := strings.TrimSpace(spec.ID)
		envName := strings.TrimSpace(spec.Env)
		if cacheID == "" || envName == "" {
			continue
		}
		cacheDir, keyName, source, pruneCount, err := resolveSingleJobCacheDir(cacheRoot, execDir, job.Env, spec, agentCapabilities)
		if err != nil {
			fallback := filepath.Join(execDir, ".ciwi-cache", sanitizeCacheSegment(cacheID))
			if mkErr := os.MkdirAll(fallback, 0o755); mkErr != nil {
				logs = append(logs, fmt.Sprintf("id=%s env=%s warning=cache_disabled err=%v fallback_err=%v", cacheID, envName, err, mkErr))
				continue
			}
			cacheDir = fallback
			source = "fallback"
			logs = append(logs, fmt.Sprintf("id=%s env=%s source=%s warning=%v", cacheID, envName, source, err))
		}
			cacheEnv[envName] = cacheEnvPath(cacheDir)
		msg := fmt.Sprintf("id=%s env=%s key=%s source=%s dir=%s", cacheID, envName, keyName, source, filepath.ToSlash(cacheDir))
		if pruneCount > 0 {
			msg += fmt.Sprintf(" pruned=%d", pruneCount)
		}
		logs = append(logs, msg)
	}
	if len(cacheEnv) == 0 {
		return nil, logs
	}
	return cacheEnv, logs
}

func resolveSingleJobCacheDir(cacheRoot, execDir string, jobEnv map[string]string, spec protocol.JobCacheSpec, agentCapabilities map[string]string) (string, string, string, int, error) {
	cacheID := sanitizeCacheSegment(spec.ID)
	cacheBase := filepath.Join(cacheRoot, cacheID)
	if err := os.MkdirAll(cacheBase, 0o755); err != nil {
		return "", "", "", 0, fmt.Errorf("create cache base: %w", err)
	}

	keyName, err := computeJobCacheKey(execDir, jobEnv, spec, agentCapabilities)
	if err != nil {
		return "", "", "", 0, err
	}

	policy := normalizeCachePolicy(spec.Policy)
	target := filepath.Join(cacheBase, keyName)
	selected := target
	source := "miss"

	if policy.pull {
		if isDir(target) {
			source = "hit"
		} else if restorePath, restoreName := findRestoreCache(cacheBase, spec.RestoreKeys); restorePath != "" {
			selected = restorePath
			source = "restore:" + restoreName
		}
	}
	if source == "miss" && !policy.push {
		selected = filepath.Join(execDir, ".ciwi-cache", cacheID)
		source = "miss-ephemeral"
	}

	if err := os.MkdirAll(selected, 0o755); err != nil {
		return "", keyName, source, 0, fmt.Errorf("create cache dir: %w", err)
	}
	_ = touchCacheDir(selected)
	pruned := 0
	if policy.push {
		count, err := pruneCacheEntries(cacheBase, spec, selected)
		if err != nil {
			return selected, keyName, source, 0, fmt.Errorf("prune cache: %w", err)
		}
		pruned = count
	}
	return selected, keyName, source, pruned, nil
}

func normalizeCachePolicy(raw string) cachePolicy {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "pull":
		return cachePolicy{pull: true}
	case "push":
		return cachePolicy{push: true}
	case "pull-push", "":
		return cachePolicy{pull: true, push: true}
	default:
		return cachePolicy{pull: true, push: true}
	}
}

func computeJobCacheKey(execDir string, jobEnv map[string]string, spec protocol.JobCacheSpec, agentCapabilities map[string]string) (string, error) {
	prefix := strings.TrimSpace(spec.Key.Prefix)
	if prefix == "" {
		prefix = strings.TrimSpace(spec.ID)
	}
	if prefix == "" {
		prefix = "cache"
	}
	prefix = sanitizeCacheSegment(prefix)

	filePatterns := dedupeSortedStrings(spec.Key.Files)
	runtimeTokens := dedupeSortedStrings(spec.Key.Runtime)
	toolTokens := dedupeSortedStrings(spec.Key.Tools)
	envTokens := dedupeSortedStrings(spec.Key.Env)

	files := map[string]string{}
	for _, pattern := range filePatterns {
		normalizedPattern := normalizeGlobPattern(pattern)
		matches, err := collectMatchingFiles(execDir, normalizedPattern)
		if err != nil {
			return "", fmt.Errorf("cache key files pattern %q: %w", normalizedPattern, err)
		}
		if len(matches) == 0 {
			files["pattern:"+normalizedPattern] = "<none>"
			continue
		}
		for _, rel := range matches {
			hash, err := hashFileSHA256(filepath.Join(execDir, filepath.FromSlash(rel)))
			if err != nil {
				return "", fmt.Errorf("hash cache key file %q: %w", rel, err)
			}
			files[rel] = hash
		}
	}

	runtimeVals := map[string]string{}
	for _, token := range runtimeTokens {
		switch strings.ToLower(strings.TrimSpace(token)) {
		case "os":
			if v := strings.TrimSpace(agentCapabilities["os"]); v != "" {
				runtimeVals["os"] = v
			} else {
				runtimeVals["os"] = runtime.GOOS
			}
		case "arch":
			if v := strings.TrimSpace(agentCapabilities["arch"]); v != "" {
				runtimeVals["arch"] = v
			} else {
				runtimeVals["arch"] = runtime.GOARCH
			}
		}
	}

	toolVals := map[string]string{}
	for _, tool := range toolTokens {
		name := strings.TrimSpace(tool)
		if name == "" {
			continue
		}
		toolVals[name] = strings.TrimSpace(agentCapabilities["tool."+name])
	}

	envVals := map[string]string{}
	for _, envName := range envTokens {
		name := strings.TrimSpace(envName)
		if name == "" {
			continue
		}
		if v, ok := jobEnv[name]; ok {
			envVals[name] = v
			continue
		}
		envVals[name] = os.Getenv(name)
	}

	payload := struct {
		Prefix  string            `json:"prefix"`
		Files   map[string]string `json:"files,omitempty"`
		Runtime map[string]string `json:"runtime,omitempty"`
		Tools   map[string]string `json:"tools,omitempty"`
		Env     map[string]string `json:"env,omitempty"`
	}{
		Prefix:  prefix,
		Files:   files,
		Runtime: runtimeVals,
		Tools:   toolVals,
		Env:     envVals,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal key payload: %w", err)
	}
	sum := sha256.Sum256(raw)
	return fmt.Sprintf("%s-%s", prefix, hex.EncodeToString(sum[:])[:20]), nil
}

func dedupeSortedStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, v := range in {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

func normalizeGlobPattern(v string) string {
	v = strings.ReplaceAll(strings.TrimSpace(v), "\\", "/")
	v = strings.TrimPrefix(v, "./")
	return pathCleanSlash(v)
}

func pathCleanSlash(v string) string {
	v = strings.ReplaceAll(v, "//", "/")
	return strings.TrimPrefix(v, "/")
}

func collectMatchingFiles(root, pattern string) ([]string, error) {
	if pattern == "" {
		return nil, nil
	}
	re, err := compileCacheGlob(pattern)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0)
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if re.MatchString(rel) {
			out = append(out, rel)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(out)
	return out, nil
}

func compileCacheGlob(pattern string) (*regexp.Regexp, error) {
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(pattern); i++ {
		ch := pattern[i]
		if ch == '*' {
			if i+1 < len(pattern) && pattern[i+1] == '*' {
				b.WriteString(".*")
				i++
				continue
			}
			b.WriteString("[^/]*")
			continue
		}
		if ch == '?' {
			b.WriteString("[^/]")
			continue
		}
		if strings.ContainsRune(`.+()[]{}^$|\`, rune(ch)) {
			b.WriteByte('\\')
		}
		b.WriteByte(ch)
	}
	b.WriteString("$")
	re, err := regexp.Compile(b.String())
	if err != nil {
		return nil, fmt.Errorf("invalid glob %q: %w", pattern, err)
	}
	return re, nil
}

func hashFileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func findRestoreCache(baseDir string, restoreKeys []string) (string, string) {
	entries, err := os.ReadDir(baseDir)
	if err != nil {
		return "", ""
	}
	for _, restore := range restoreKeys {
		prefix := sanitizeCacheSegment(strings.TrimSpace(restore))
		if prefix == "" {
			continue
		}
		bestPath := ""
		bestName := ""
		bestMod := time.Time{}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			name := entry.Name()
			if !strings.HasPrefix(name, prefix) {
				continue
			}
			info, err := entry.Info()
			if err != nil {
				continue
			}
			if bestPath == "" || info.ModTime().After(bestMod) {
				bestPath = filepath.Join(baseDir, name)
				bestName = name
				bestMod = info.ModTime()
			}
		}
		if bestPath != "" {
			return bestPath, bestName
		}
	}
	return "", ""
}

func pruneCacheEntries(baseDir string, spec protocol.JobCacheSpec, keepPath string) (int, error) {
	if spec.TTLDays <= 0 && spec.MaxSizeMB <= 0 {
		return 0, nil
	}
	type cacheEntry struct {
		path    string
		modTime time.Time
		size    int64
	}

	loadEntries := func() ([]cacheEntry, error) {
		items, err := os.ReadDir(baseDir)
		if err != nil {
			return nil, err
		}
		out := make([]cacheEntry, 0, len(items))
		for _, item := range items {
			if !item.IsDir() {
				continue
			}
			path := filepath.Join(baseDir, item.Name())
			if samePath(path, keepPath) {
				continue
			}
			info, err := item.Info()
			if err != nil {
				continue
			}
			size, err := dirSizeBytes(path)
			if err != nil {
				continue
			}
			out = append(out, cacheEntry{path: path, modTime: info.ModTime(), size: size})
		}
		return out, nil
	}

	pruned := 0
	entries, err := loadEntries()
	if err != nil {
		return 0, err
	}

	if spec.TTLDays > 0 {
		cutoff := time.Now().Add(-time.Duration(spec.TTLDays) * 24 * time.Hour)
		for _, entry := range entries {
			if entry.modTime.Before(cutoff) {
				if err := os.RemoveAll(entry.path); err != nil {
					return pruned, err
				}
				pruned++
			}
		}
		entries, err = loadEntries()
		if err != nil {
			return pruned, err
		}
	}

	if spec.MaxSizeMB > 0 {
		var total int64
		for _, entry := range entries {
			total += entry.size
		}
		limit := int64(spec.MaxSizeMB) * 1024 * 1024
		if total > limit {
			sort.Slice(entries, func(i, j int) bool {
				return entries[i].modTime.Before(entries[j].modTime)
			})
			for _, entry := range entries {
				if total <= limit {
					break
				}
				if err := os.RemoveAll(entry.path); err != nil {
					return pruned, err
				}
				total -= entry.size
				pruned++
			}
		}
	}

	return pruned, nil
}

func dirSizeBytes(root string) (int64, error) {
	var total int64
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		total += info.Size()
		return nil
	})
	return total, err
}

func touchCacheDir(dir string) error {
	marker := filepath.Join(dir, ".ciwi-cache-touch")
	if err := os.WriteFile(marker, []byte(time.Now().UTC().Format(time.RFC3339Nano)), 0o644); err != nil {
		return err
	}
	now := time.Now()
	return os.Chtimes(dir, now, now)
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func sanitizeCacheSegment(v string) string {
	v = strings.TrimSpace(strings.ToLower(v))
	if v == "" {
		return "cache"
	}
	var b strings.Builder
	for _, r := range v {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_' || r == '.':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	s := strings.Trim(b.String(), "-.")
	if s == "" {
		return "cache"
	}
	return s
}

func samePath(a, b string) bool {
	aa := filepath.Clean(a)
	bb := filepath.Clean(b)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(aa, bb)
	}
	return aa == bb
}

func cacheEnvPath(path string) string {
	return cacheEnvPathForGOOS(runtime.GOOS, path)
}

func cacheEnvPathForGOOS(goos, path string) string {
	if strings.EqualFold(strings.TrimSpace(goos), "windows") {
		return strings.ReplaceAll(path, "\\", "/")
	}
	return path
}
