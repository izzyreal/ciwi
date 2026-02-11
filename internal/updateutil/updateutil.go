package updateutil

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

func ExpectedAssetName(goos, goarch string) string {
	switch {
	case goos == "linux" && goarch == "amd64":
		return "ciwi-linux-amd64"
	case goos == "linux" && goarch == "arm64":
		return "ciwi-linux-arm64"
	case goos == "darwin" && goarch == "amd64":
		return "ciwi-darwin-amd64"
	case goos == "darwin" && goarch == "arm64":
		return "ciwi-darwin-arm64"
	case goos == "windows" && goarch == "amd64":
		return "ciwi-windows-amd64.exe"
	case goos == "windows" && goarch == "arm64":
		return "ciwi-windows-arm64.exe"
	}
	return ""
}

func IsVersionNewer(latest, current string) bool {
	l := parseSemver(strings.TrimSpace(latest))
	c := parseSemver(strings.TrimSpace(current))
	if l.valid && c.valid {
		if l.major != c.major {
			return l.major > c.major
		}
		if l.minor != c.minor {
			return l.minor > c.minor
		}
		return l.patch > c.patch
	}
	return strings.TrimPrefix(latest, "v") != strings.TrimPrefix(current, "v")
}

func CurrentVersion() string {
	v := strings.TrimSpace(envOrDefault("CIWI_VERSION", "dev"))
	if v == "" {
		return "dev"
	}
	return v
}

func VerifyFileSHA256(path, assetName, checksumContent string) error {
	want := ""
	for _, line := range strings.Split(checksumContent, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		name := strings.TrimPrefix(fields[len(fields)-1], "*")
		if name == assetName {
			want = strings.ToLower(fields[0])
			break
		}
	}
	if want == "" {
		return fmt.Errorf("no checksum entry for %s", assetName)
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	got := fmt.Sprintf("%x", h.Sum(nil))
	if got != want {
		return fmt.Errorf("sha256 mismatch: got=%s want=%s", got, want)
	}
	return nil
}

func ExeExt() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}

func CopyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return nil
}

func LooksLikeGoRunBinary(path string) bool {
	p := filepath.ToSlash(strings.ToLower(path))
	return strings.Contains(p, "/go-build") || strings.Contains(p, "/temp/")
}

type semver struct {
	major int
	minor int
	patch int
	valid bool
}

func parseSemver(v string) semver {
	v = strings.TrimPrefix(v, "v")
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return semver{}
	}
	maj, e1 := strconv.Atoi(parts[0])
	min, e2 := strconv.Atoi(parts[1])
	pat, e3 := strconv.Atoi(parts[2])
	if e1 != nil || e2 != nil || e3 != nil {
		return semver{}
	}
	return semver{major: maj, minor: min, patch: pat, valid: true}
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
