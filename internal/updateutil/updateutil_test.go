package updateutil

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExpectedAssetName(t *testing.T) {
	cases := []struct {
		goos, goarch string
		want         string
	}{
		{"linux", "amd64", "ciwi-linux-amd64"},
		{"linux", "arm64", "ciwi-linux-arm64"},
		{"darwin", "amd64", "ciwi-darwin-amd64"},
		{"darwin", "arm64", "ciwi-darwin-arm64"},
		{"windows", "amd64", "ciwi-windows-amd64.exe"},
		{"windows", "arm64", "ciwi-windows-arm64.exe"},
		{"plan9", "amd64", ""},
	}
	for _, tc := range cases {
		if got := ExpectedAssetName(tc.goos, tc.goarch); got != tc.want {
			t.Fatalf("ExpectedAssetName(%q,%q)=%q want %q", tc.goos, tc.goarch, got, tc.want)
		}
	}
}

func TestVersionComparisons(t *testing.T) {
	if !IsVersionNewer("v1.2.0", "v1.1.9") {
		t.Fatalf("expected newer semver to compare true")
	}
	if IsVersionNewer("v1.2.0", "v1.2.0") {
		t.Fatalf("equal semver must not be newer")
	}
	if IsVersionDifferent("1.2.0", "v1.2.0") {
		t.Fatalf("normalized equal semver must not differ")
	}
	if !IsVersionDifferent("main-abc", "main-def") {
		t.Fatalf("fallback non-semver compare should detect difference")
	}
}

func TestVerifyFileSHA256(t *testing.T) {
	p := filepath.Join(t.TempDir(), "asset.bin")
	content := []byte("asset-content")
	if err := os.WriteFile(p, content, 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	sum := sha256.Sum256(content)
	hash := hex.EncodeToString(sum[:])

	okChecksums := strings.Join([]string{
		"# comment",
		fmt.Sprintf("%s  other.bin", strings.Repeat("0", 64)),
		fmt.Sprintf("%s  *asset.bin", hash),
	}, "\n")
	if err := VerifyFileSHA256(p, "asset.bin", okChecksums); err != nil {
		t.Fatalf("expected checksum verify success, got %v", err)
	}

	if err := VerifyFileSHA256(p, "missing.bin", okChecksums); err == nil {
		t.Fatalf("expected missing checksum entry error")
	}

	badChecksums := fmt.Sprintf("%s  asset.bin\n", strings.Repeat("a", 64))
	if err := VerifyFileSHA256(p, "asset.bin", badChecksums); err == nil {
		t.Fatalf("expected checksum mismatch error")
	}
}

func TestCopyFileAndLooksLikeGoRunBinary(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.bin")
	dst := filepath.Join(dir, "dst.bin")
	if err := os.WriteFile(src, []byte("abc123"), 0o644); err != nil {
		t.Fatalf("write src: %v", err)
	}
	if err := CopyFile(src, dst, 0o600); err != nil {
		t.Fatalf("copy file: %v", err)
	}
	raw, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read dst: %v", err)
	}
	if string(raw) != "abc123" {
		t.Fatalf("unexpected dst contents: %q", string(raw))
	}

	if !LooksLikeGoRunBinary("/tmp/go-build1234/b001/exe/main") {
		t.Fatalf("expected go-build path to be detected")
	}
	if !LooksLikeGoRunBinary("/var/folders/x/TEMP/runner") {
		t.Fatalf("expected /temp/ path match")
	}
	if LooksLikeGoRunBinary("/usr/local/bin/ciwi") {
		t.Fatalf("normal installed path should not match go-run")
	}
}
