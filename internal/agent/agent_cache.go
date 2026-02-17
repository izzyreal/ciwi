package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/izzyreal/ciwi/internal/protocol"
)

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
		cacheDir, source, err := resolveSingleJobCacheDir(cacheRoot, spec)
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
		msg := fmt.Sprintf("id=%s env=%s source=%s dir=%s", cacheID, envName, source, filepath.ToSlash(cacheDir))
		logs = append(logs, msg)
	}
	if len(cacheEnv) == 0 {
		return nil, logs
	}
	return cacheEnv, logs
}

func resolveSingleJobCacheDir(cacheRoot string, spec protocol.JobCacheSpec) (string, string, error) {
	cacheID := sanitizeCacheSegment(spec.ID)
	cacheBase := filepath.Join(cacheRoot, cacheID)
	targetExists := isDir(cacheBase)
	if err := os.MkdirAll(cacheBase, 0o755); err != nil {
		return "", "", fmt.Errorf("create cache base: %w", err)
	}

	selected := cacheBase
	source := "miss"
	if targetExists {
		source = "hit"
	}

	if err := os.MkdirAll(selected, 0o755); err != nil {
		return "", source, fmt.Errorf("create cache dir: %w", err)
	}
	_ = touchCacheDir(selected)
	return selected, source, nil
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
