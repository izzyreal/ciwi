package updateutil

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/izzyreal/ciwi/internal/version"
	"golang.org/x/mod/semver"
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
	l, lok := normalizeSemver(latest)
	c, cok := normalizeSemver(current)
	if lok && cok {
		return semver.Compare(l, c) > 0
	}
	return strings.TrimPrefix(latest, "v") != strings.TrimPrefix(current, "v")
}

func IsVersionDifferent(target, current string) bool {
	t, tok := normalizeSemver(target)
	c, cok := normalizeSemver(current)
	if tok && cok {
		return semver.Compare(t, c) != 0
	}
	return strings.TrimPrefix(target, "v") != strings.TrimPrefix(current, "v")
}

func CurrentVersion() string {
	return version.Current()
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

func normalizeSemver(v string) (string, bool) {
	v = strings.TrimSpace(v)
	if v == "" {
		return "", false
	}
	if !strings.HasPrefix(v, "v") {
		v = "v" + v
	}
	if !semver.IsValid(v) {
		return "", false
	}
	return v, true
}
