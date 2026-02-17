package agent

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
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

func resolveJobCacheEnv(workDir, execDir string, job protocol.JobExecution) (map[string]string, []string) {
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
		cacheDir, keyName, source, pruneCount, err := resolveSingleJobCacheDir(cacheRoot, execDir, spec)
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

func resolveSingleJobCacheDir(cacheRoot, execDir string, spec protocol.JobCacheSpec) (string, string, string, int, error) {
	cacheID := sanitizeCacheSegment(spec.ID)
	cacheBase := filepath.Join(cacheRoot, cacheID)
	if err := os.MkdirAll(cacheBase, 0o755); err != nil {
		return "", "", "", 0, fmt.Errorf("create cache base: %w", err)
	}

	keyName := computeJobCacheKey(spec)

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

func computeJobCacheKey(spec protocol.JobCacheSpec) string {
	prefix := strings.TrimSpace(spec.Key.Prefix)
	if prefix == "" {
		prefix = strings.TrimSpace(spec.ID)
	}
	if prefix == "" {
		prefix = "cache"
	}
	return sanitizeCacheSegment(prefix)
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
