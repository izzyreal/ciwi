package agent

import (
	"context"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"time"
)

var versionPattern = regexp.MustCompile(`([0-9]+(?:\.[0-9]+){1,3})`)

func detectAgentCapabilities() map[string]string {
	caps := map[string]string{
		"executor": "shell",
		"os":       runtime.GOOS,
		"arch":     runtime.GOARCH,
	}
	for tool, version := range detectToolVersions() {
		if strings.TrimSpace(version) == "" {
			continue
		}
		caps["tool."+tool] = version
	}
	return caps
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
		{name: "gcc", cmd: "gcc", args: []string{"--version"}},
		{name: "clang", cmd: "clang", args: []string{"--version"}},
		{name: "xcodebuild", cmd: "xcodebuild", args: []string{"-version"}},
		{name: "msvc", cmd: "cl", args: []string{}},
	}
	out := map[string]string{}
	for _, t := range tools {
		if v := detectToolVersion(t.cmd, t.args...); v != "" {
			out[t.name] = v
		}
	}
	return out
}

func detectToolVersion(cmd string, args ...string) string {
	if _, err := exec.LookPath(cmd); err != nil {
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
