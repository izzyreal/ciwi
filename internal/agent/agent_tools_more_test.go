package agent

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDetectToolVersionByPathParsesSemver(t *testing.T) {
	binDir := t.TempDir()
	tool := filepath.Join(binDir, "mytool")
	if runtime.GOOS == "windows" {
		tool += ".bat"
		if err := os.WriteFile(tool, []byte("@echo mytool version 1.2.3\r\n"), 0o755); err != nil {
			t.Fatalf("write fake tool: %v", err)
		}
	} else {
		if err := os.WriteFile(tool, []byte("#!/bin/sh\necho 'mytool version 1.2.3'\n"), 0o755); err != nil {
			t.Fatalf("write fake tool: %v", err)
		}
	}
	if got := detectToolVersionByPath(tool); got != "1.2.3" {
		t.Fatalf("expected parsed version 1.2.3, got %q", got)
	}
}

func TestDetectToolVersionParsesGoVersionOutput(t *testing.T) {
	binDir := t.TempDir()
	name := "go"
	path := filepath.Join(binDir, name)
	if runtime.GOOS == "windows" {
		path += ".bat"
		if err := os.WriteFile(path, []byte("@echo go version go1.26.0 windows/amd64\r\n"), 0o755); err != nil {
			t.Fatalf("write fake go: %v", err)
		}
	} else {
		if err := os.WriteFile(path, []byte("#!/bin/sh\necho 'go version go1.26.0 linux/amd64'\n"), 0o755); err != nil {
			t.Fatalf("write fake go: %v", err)
		}
	}
	t.Setenv("PATH", binDir)
	if got := detectToolVersion("go", "version"); got != "1.26.0" {
		t.Fatalf("expected go version 1.26.0, got %q", got)
	}
}

func TestDetectToolVersionByPathEdgeCases(t *testing.T) {
	if got := detectToolVersionByPath(""); got != "" {
		t.Fatalf("expected empty version for empty command path, got %q", got)
	}
	binDir := t.TempDir()
	tool := filepath.Join(binDir, "noversion")
	if runtime.GOOS == "windows" {
		tool += ".bat"
		if err := os.WriteFile(tool, []byte("@echo hello\r\n"), 0o755); err != nil {
			t.Fatalf("write fake tool: %v", err)
		}
	} else {
		if err := os.WriteFile(tool, []byte("#!/bin/sh\necho hello\n"), 0o755); err != nil {
			t.Fatalf("write fake tool: %v", err)
		}
	}
	if got := detectToolVersionByPath(tool); got != "" {
		t.Fatalf("expected empty version for output without semver, got %q", got)
	}
}

func TestDetectCodesignVersionViaWhatFallback(t *testing.T) {
	binDir := t.TempDir()
	pathSep := string(os.PathListSeparator)
	origPath := os.Getenv("PATH")
	setPath := binDir
	if strings.TrimSpace(origPath) != "" {
		setPath = binDir + pathSep + origPath
	}
	t.Setenv("PATH", setPath)

	makeTool := func(name, body string) {
		toolPath := filepath.Join(binDir, name)
		if runtime.GOOS == "windows" {
			toolPath += ".bat"
			if err := os.WriteFile(toolPath, []byte(body+"\r\n"), 0o755); err != nil {
				t.Fatalf("write fake %s: %v", name, err)
			}
			return
		}
		if err := os.WriteFile(toolPath, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
			t.Fatalf("write fake %s: %v", name, err)
		}
	}

	// Simulate codesign existing but not supporting --version.
	makeTool("codesign", "exit 1")
	// Simulate BSD `what` output for codesign.
	makeTool("what", "echo 'PROGRAM:codesign  PROJECT:codesign-69.100.1'")

	if got := detectCodesignVersion(); got == "" {
		t.Fatalf("expected codesign version from what fallback, got empty")
	}
}

func TestCandidateVSWherePathsIncludesLookupAndEnvRoots(t *testing.T) {
	binDir := t.TempDir()
	lookupPath := filepath.Join(binDir, "vswhere.exe")
	if runtime.GOOS == "windows" {
		if err := os.WriteFile(lookupPath, []byte("@echo C:\\VS\r\n"), 0o755); err != nil {
			t.Fatalf("write fake vswhere: %v", err)
		}
	} else {
		if err := os.WriteFile(lookupPath, []byte("#!/bin/sh\necho /VS\n"), 0o755); err != nil {
			t.Fatalf("write fake vswhere: %v", err)
		}
	}
	t.Setenv("PATH", binDir)
	t.Setenv("ProgramFiles(x86)", `C:\Program Files (x86)`)
	t.Setenv("ProgramW6432", `C:\Program Files`)
	t.Setenv("ProgramFiles", `C:\Program Files`)

	paths := candidateVSWherePaths()
	if len(paths) < 2 {
		t.Fatalf("expected candidateVSWherePaths to include lookup and env roots, got %v", paths)
	}
}

func TestFileExistsAndHasXorgDev(t *testing.T) {
	f := filepath.Join(t.TempDir(), "f")
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if !fileExists(f) {
		t.Fatalf("expected fileExists true for regular file")
	}
	if fileExists(filepath.Dir(f)) {
		t.Fatalf("expected fileExists false for directory")
	}
	t.Setenv("PATH", "")
	if hasXorgDev() {
		t.Fatalf("expected hasXorgDev false without pkg-config in PATH")
	}
}

func TestDetectAgentCapabilitiesBasicShape(t *testing.T) {
	t.Setenv("PATH", "")
	t.Setenv("INVOCATION_ID", "")
	caps := detectAgentCapabilities()
	if caps["executor"] != executorScript {
		t.Fatalf("unexpected executor capability: %v", caps)
	}
	if caps["os"] == "" || caps["arch"] == "" || caps["shells"] == "" {
		t.Fatalf("expected base capability keys, got %v", caps)
	}
	if caps["run_mode"] == "" {
		t.Fatalf("expected run_mode capability")
	}
}

func TestQueryVSWhereInstallPathParsesFirstNonEmptyLine(t *testing.T) {
	tmp := t.TempDir()
	vswhere := filepath.Join(tmp, "vswhere.exe")
	if runtime.GOOS == "windows" {
		if err := os.WriteFile(vswhere, []byte("@echo.\r\n@echo C:\\VS\\2022\\BuildTools\r\n"), 0o755); err != nil {
			t.Fatalf("write fake vswhere: %v", err)
		}
	} else {
		if err := os.WriteFile(vswhere, []byte("#!/bin/sh\necho\necho /opt/vs/2022/BuildTools\n"), 0o755); err != nil {
			t.Fatalf("write fake vswhere: %v", err)
		}
	}
	got := queryVSWhereInstallPath(vswhere)
	if strings.TrimSpace(got) == "" {
		t.Fatalf("expected parsed install path")
	}
}

func TestFindWindowsMSVCCompilerPathWithOverride(t *testing.T) {
	tmp := t.TempDir()
	cl := filepath.Join(tmp, "cl.exe")
	if err := os.WriteFile(cl, []byte("stub"), 0o644); err != nil {
		t.Fatalf("write override cl.exe: %v", err)
	}
	t.Setenv("CIWI_MSVC_CL_PATH", cl)
	if got := findWindowsMSVCCompilerPath(); got != cl {
		t.Fatalf("expected CIWI_MSVC_CL_PATH override, got %q", got)
	}
}

func TestFindWindowsMSVCCompilerPathViaVswhere(t *testing.T) {
	tmp := t.TempDir()
	installPath := filepath.Join(tmp, "VS", "2022", "BuildTools")
	cl := filepath.Join(installPath, "VC", "Tools", "MSVC", "14.39.33519", "bin", "Hostx64", "x64", "cl.exe")
	if err := os.MkdirAll(filepath.Dir(cl), 0o755); err != nil {
		t.Fatalf("mkdir cl path: %v", err)
	}
	if err := os.WriteFile(cl, []byte("stub"), 0o644); err != nil {
		t.Fatalf("write cl.exe: %v", err)
	}

	vswhere := filepath.Join(tmp, "vswhere.exe")
	if runtime.GOOS == "windows" {
		content := "@echo " + installPath + "\r\n"
		if err := os.WriteFile(vswhere, []byte(content), 0o755); err != nil {
			t.Fatalf("write fake vswhere: %v", err)
		}
	} else {
		content := "#!/bin/sh\necho '" + installPath + "'\n"
		if err := os.WriteFile(vswhere, []byte(content), 0o755); err != nil {
			t.Fatalf("write fake vswhere: %v", err)
		}
	}
	t.Setenv("PATH", tmp)
	t.Setenv("CIWI_MSVC_CL_PATH", "")
	if got := findWindowsMSVCCompilerPath(); got != cl {
		t.Fatalf("expected vswhere-discovered cl.exe %q, got %q", cl, got)
	}
}
