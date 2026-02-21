package agent

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"
)

var versionPattern = regexp.MustCompile(`([0-9]+(?:\.[0-9]+){1,3})`)
var detectToolVersionInShellFn = detectToolVersionInShell

func detectAgentCapabilities() map[string]string {
	shells := supportedShellsForRuntime()
	runMode := "service"
	if selfUpdateServiceModeReason() != "" {
		runMode = "manual"
	}
	caps := map[string]string{
		"executor": executorScript,
		"shells":   strings.Join(shells, ","),
		"os":       runtime.GOOS,
		"arch":     runtime.GOARCH,
		"run_mode": runMode,
	}
	for tool, version := range detectToolVersions() {
		if strings.TrimSpace(version) == "" {
			continue
		}
		caps["tool."+tool] = version
	}
	return caps
}

func hasXorgDev() bool {
	if runtime.GOOS != "linux" {
		return false
	}
	if _, err := exec.LookPath("pkg-config"); err != nil {
		return false
	}
	modules := []string{"x11", "xext", "xrandr", "xi", "xcursor", "xinerama", "xfixes", "xscrnsaver"}
	for _, module := range modules {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		err := exec.CommandContext(ctx, "pkg-config", "--exists", module).Run()
		cancel()
		if err != nil {
			return false
		}
	}
	return true
}

func detectToolVersions() map[string]string {
	tools := []struct {
		name string
		cmd  string
		args []string
	}{
		{name: "git", cmd: "git", args: []string{"--version"}},
		{name: "go", cmd: "go", args: []string{"version"}},
		{name: "gh", cmd: "gh", args: []string{"--version"}},
		{name: "cmake", cmd: "cmake", args: []string{"--version"}},
		{name: "sccache", cmd: "sccache", args: []string{"--version"}},
		{name: "ccache", cmd: "ccache", args: []string{"--version"}},
		{name: "ninja", cmd: "ninja", args: []string{"--version"}},
		{name: "docker", cmd: "docker", args: []string{"--version"}},
		{name: "gcc", cmd: "gcc", args: []string{"--version"}},
		{name: "clang", cmd: "clang", args: []string{"--version"}},
		{name: "xcodebuild", cmd: "xcodebuild", args: []string{"-version"}},
		{name: "iscc", cmd: "iscc", args: []string{"/?"}},
		{name: "signtool", cmd: "signtool", args: []string{"/?"}},
		{name: "productsign", cmd: "productsign", args: []string{"--version"}},
		{name: "packagesbuild", cmd: "packagesbuild", args: []string{"--version"}},
		{name: "packagesutil", cmd: "packagesutil", args: []string{"version"}},
	}
	out := map[string]string{}
	for _, t := range tools {
		if v := detectToolVersion(t.cmd, t.args...); v != "" {
			out[t.name] = v
		}
	}
	if v := detectCodesignVersion(); v != "" {
		out["codesign"] = v
	}
	if v := detectXCRUNToolVersion("notarytool"); v != "" {
		out["notarytool"] = v
	}
	if v := detectXCRUNToolVersion("stapler"); v != "" {
		out["stapler"] = v
	}
	if fileExists("/usr/libexec/PlistBuddy") {
		out["plistbuddy"] = "1"
	}
	if v := detectMSVCVersion(); v != "" {
		out["msvc"] = v
	}
	if hasXorgDev() {
		out["xorg-dev"] = "1"
	}
	return out
}

func detectCodesignVersion() string {
	if v := detectToolVersion("codesign", "--version"); v != "" {
		return v
	}
	codesignPath, err := exec.LookPath("codesign")
	if err != nil || strings.TrimSpace(codesignPath) == "" {
		return ""
	}
	if v := detectToolVersion("what", codesignPath); v != "" {
		return v
	}
	return ""
}

func detectXCRUNToolVersion(tool string) string {
	tool = strings.TrimSpace(tool)
	if tool == "" || runtime.GOOS != "darwin" {
		return ""
	}
	if _, err := exec.LookPath("xcrun"); err != nil {
		return ""
	}
	if v := detectToolVersion("xcrun", tool, "--version"); v != "" {
		return v
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "xcrun", "--find", tool).CombinedOutput()
	if err != nil || strings.TrimSpace(string(out)) == "" {
		return ""
	}
	return "1"
}

func detectToolVersion(cmd string, args ...string) string {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return ""
	}
	for _, shell := range supportedShellsForRuntime() {
		if v := detectToolVersionInShellFn(shell, cmd, args...); v != "" {
			return v
		}
	}
	if _, err := exec.LookPath(cmd); err != nil {
		return ""
	}
	return detectToolVersionByPath(cmd, args...)
}

func detectToolVersionInShell(shell, cmd string, args ...string) string {
	script := toolProbeScriptForShell(shell, cmd, args)
	if strings.TrimSpace(script) == "" {
		return ""
	}
	bin, shellArgs, err := commandForScript(shell, script)
	if err != nil {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	c := exec.CommandContext(ctx, bin, shellArgs...)
	raw, err := c.CombinedOutput()
	if err != nil && len(raw) == 0 {
		return ""
	}
	text := strings.TrimSpace(string(raw))
	if text == "" {
		return ""
	}
	if strings.Contains(text, "go version go") {
		text = strings.ReplaceAll(text, "go version go", "go version ")
	}
	if m := versionPattern.FindStringSubmatch(text); len(m) >= 2 {
		return m[1]
	}
	return ""
}

func toolProbeScriptForShell(shell, cmd string, args []string) string {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return ""
	}
	norm := normalizeShell(shell)
	switch norm {
	case shellPosix:
		line := shellQuote(cmd)
		for _, arg := range args {
			line += " " + shellQuote(arg)
		}
		return "if command -v " + shellQuote(cmd) + " >/dev/null 2>&1; then " + line + " 2>&1; fi"
	case shellCmd:
		line := cmdQuote(cmd)
		for _, arg := range args {
			line += " " + cmdQuote(arg)
		}
		return "where " + cmdQuote(cmd) + " >NUL 2>NUL && " + line + " 2>&1"
	case shellPowerShell:
		quotedCmd := powershellQuote(cmd)
		line := "& " + quotedCmd
		for _, arg := range args {
			line += " " + powershellQuote(arg)
		}
		return "$c = Get-Command " + quotedCmd + " -ErrorAction SilentlyContinue; if ($c) { " + line + " 2>&1 }"
	default:
		return ""
	}
}

func cmdQuote(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return `""`
	}
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}

func powershellQuote(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

func detectToolVersionByPath(cmd string, args ...string) string {
	if strings.TrimSpace(cmd) == "" {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	c := exec.CommandContext(ctx, cmd, args...)
	raw, err := c.CombinedOutput()
	if err != nil && len(raw) == 0 {
		return ""
	}
	text := strings.TrimSpace(string(raw))
	if text == "" {
		return ""
	}
	if strings.Contains(text, "go version go") {
		text = strings.ReplaceAll(text, "go version go", "go version ")
	}
	if m := versionPattern.FindStringSubmatch(text); len(m) >= 2 {
		return m[1]
	}
	return ""
}

func detectMSVCVersion() string {
	if v := detectToolVersion("cl"); v != "" {
		return v
	}
	if runtime.GOOS != "windows" {
		return ""
	}
	if clPath := findWindowsMSVCCompilerPath(); clPath != "" {
		return detectToolVersionByPath(clPath)
	}
	return ""
}

func findWindowsMSVCCompilerPath() string {
	if override := strings.TrimSpace(os.Getenv("CIWI_MSVC_CL_PATH")); override != "" && fileExists(override) {
		return override
	}

	for _, vswherePath := range candidateVSWherePaths() {
		if !fileExists(vswherePath) {
			continue
		}
		installPath := queryVSWhereInstallPath(vswherePath)
		if installPath == "" {
			continue
		}
		if cl := findMSVCCompilerInInstallPath(installPath); cl != "" {
			return cl
		}
	}

	for _, root := range windowsProgramFilesRoots() {
		if root == "" {
			continue
		}
		base := filepath.Join(root, "Microsoft Visual Studio")
		matches, err := filepath.Glob(filepath.Join(base, "*", "*"))
		if err != nil {
			continue
		}
		sort.Strings(matches)
		for i := len(matches) - 1; i >= 0; i-- {
			if cl := findMSVCCompilerInInstallPath(matches[i]); cl != "" {
				return cl
			}
		}
	}

	return ""
}

func queryVSWhereInstallPath(vswherePath string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(
		ctx,
		vswherePath,
		"-latest",
		"-products", "*",
		"-requires", "Microsoft.VisualStudio.Component.VC.Tools.x86.x64",
		"-property", "installationPath",
	).CombinedOutput()
	if err != nil && len(out) == 0 {
		return ""
	}
	lines := strings.Split(strings.ReplaceAll(string(out), "\r\n", "\n"), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
	}
	return ""
}

func candidateVSWherePaths() []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, 4)
	add := func(p string) {
		p = strings.TrimSpace(p)
		if p == "" {
			return
		}
		if _, ok := seen[p]; ok {
			return
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	if p, err := exec.LookPath("vswhere.exe"); err == nil {
		add(p)
	}
	for _, root := range windowsProgramFilesRoots() {
		add(filepath.Join(root, "Microsoft Visual Studio", "Installer", "vswhere.exe"))
	}
	return out
}

func windowsProgramFilesRoots() []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, 4)
	add := func(v string) {
		v = strings.TrimSpace(v)
		if v == "" {
			return
		}
		if _, ok := seen[v]; ok {
			return
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	add(os.Getenv("ProgramFiles(x86)"))
	add(os.Getenv("ProgramW6432"))
	add(os.Getenv("ProgramFiles"))
	return out
}

func findMSVCCompilerInInstallPath(installPath string) string {
	installPath = strings.TrimSpace(installPath)
	if installPath == "" {
		return ""
	}

	// Prefer host/target combos that match common amd64/arm64 agents, then fall back to any cl.exe.
	patterns := []string{
		filepath.Join(installPath, "VC", "Tools", "MSVC", "*", "bin", "Hostx64", "x64", "cl.exe"),
		filepath.Join(installPath, "VC", "Tools", "MSVC", "*", "bin", "Hostarm64", "arm64", "cl.exe"),
		filepath.Join(installPath, "VC", "Tools", "MSVC", "*", "bin", "Hostx64", "arm64", "cl.exe"),
		filepath.Join(installPath, "VC", "Tools", "MSVC", "*", "bin", "Hostarm64", "x64", "cl.exe"),
		filepath.Join(installPath, "VC", "Tools", "MSVC", "*", "bin", "*", "*", "cl.exe"),
	}
	for _, pattern := range patterns {
		matches, err := filepath.Glob(pattern)
		if err != nil || len(matches) == 0 {
			continue
		}
		sort.Strings(matches)
		for i := len(matches) - 1; i >= 0; i-- {
			if fileExists(matches[i]) {
				return matches[i]
			}
		}
	}
	return ""
}

func fileExists(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}
