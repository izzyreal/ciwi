package agent

import (
	"os/exec"
	"runtime"
	"testing"

	"github.com/izzyreal/ciwi/internal/requirements"
)

func TestPythonToolDetection(t *testing.T) {
	cases := []struct {
		name, python, python3 string
	}{
		{name: "neither"},
		{name: "python only", python: "2.7.18"},
		{name: "python3 only", python3: "3.12.8"},
		{name: "both", python: "3.11.9", python3: "3.12.8"},
		{name: "unparseable", python: "unavailable", python3: "unavailable"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			check := func(t *testing.T, caps map[string]string, prefix string) {
				t.Helper()
				for tool, version := range map[string]string{"python": tc.python, "python3": tc.python3} {
					want := version
					if want == "unavailable" {
						want = ""
					}
					got, exists := caps[prefix+tool]
					if got != want || exists != (want != "") {
						t.Errorf("%s%s = %q (present %v), want %q", prefix, tool, got, exists, want)
					}
				}
			}
			t.Run("host", func(t *testing.T) {
				origProbe, origLookPath := detectToolVersionInShellFn, lookPathFn
				t.Cleanup(func() {
					detectToolVersionInShellFn, lookPathFn = origProbe, origLookPath
				})
				lookPathFn = func(string) (string, error) { return "", exec.ErrNotFound }
				detectToolVersionInShellFn = func(shell, cmd string, args ...string) string {
					version, ok := map[string]string{"python": tc.python, "python3": tc.python3}[cmd]
					if !ok {
						return ""
					}
					if len(args) != 1 || args[0] != "--version" {
						t.Fatalf("unexpected %s probe arguments: %v", cmd, args)
					}
					return parseToolVersionOutput([]byte("Python " + version))
				}
				check(t, detectToolVersions(), "")
			})
			t.Run("container", func(t *testing.T) {
				if runtime.GOOS == "windows" {
					t.Skip("fake docker shell script tests are posix-only")
				}
				binDir, logPath := writeFakeDocker(t, `
if [ "$1" != "exec" ]; then exit 1; fi
case "$5" in
  "python --version") version="$CIWI_TEST_PYTHON_VERSION" ;;
  "python3 --version") version="$CIWI_TEST_PYTHON3_VERSION" ;;
  *) exit 1 ;;
esac
if [ -z "$version" ]; then exit 127; fi
echo "Python $version" >&2
`)
				t.Setenv("PATH", binDir)
				t.Setenv("CIWI_DOCKER_LOG", logPath)
				t.Setenv("CIWI_TEST_PYTHON_VERSION", tc.python)
				t.Setenv("CIWI_TEST_PYTHON3_VERSION", tc.python3)
				check(t, collectRuntimeCapabilities(nil, "python-test"), "container.tool.")
			})
		})
	}
}

func TestPythonVersionFromStderr(t *testing.T) {
	cmd, args := "/bin/sh", []string{"-c", "echo 'Python 3.12.8' >&2"}
	if runtime.GOOS == "windows" {
		cmd, args = "cmd.exe", []string{"/d", "/c", "echo Python 3.12.8 >&2"}
	}
	if got := detectToolVersionByPath(cmd, args...); got != "3.12.8" {
		t.Fatalf("version from stderr = %q, want 3.12.8", got)
	}
}

func TestPythonToolRequirements(t *testing.T) {
	for _, tool := range []string{"python", "python3"} {
		other := "python3"
		if tool == other {
			other = "python"
		}
		for _, tc := range []struct {
			name, command, version, constraint string
			matches                            bool
		}{
			{"sufficient", tool, "3.12.8", ">=3.10", true},
			{"too old", tool, "3.9.9", ">=3.10", false},
			{"python2", tool, "2.7.18", ">=3.10", false},
			{"presence", tool, "2.7.18", "*", true},
			{"missing", tool, "", "*", false},
			{"other command", other, "3.12.8", "*", false},
		} {
			t.Run(tool+"/"+tc.name, func(t *testing.T) {
				required := map[string]string{"requires.tool." + tool: tc.constraint}
				caps := map[string]string{"tool." + tc.command: tc.version}
				if got := requirements.MatchCapabilities(required, caps); got.Matches != tc.matches {
					t.Errorf("host match = %v, want %v: %v", got.Matches, tc.matches, got.Issues)
				}
				err := validateContainerToolRequirements(
					map[string]string{"requires.container.tool." + tool: tc.constraint},
					map[string]string{"container.tool." + tc.command: tc.version},
				)
				if (err == nil) != tc.matches {
					t.Errorf("container error = %v, want match %v", err, tc.matches)
				}
			})
		}
	}
}
