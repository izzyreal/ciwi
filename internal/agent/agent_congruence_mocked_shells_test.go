package agent

import "testing"

func TestRuntimeHostToolProbeAndValidationAlignedForAllSupportedShellsWithMock(t *testing.T) {
	orig := detectToolVersionInShellFn
	t.Cleanup(func() { detectToolVersionInShellFn = orig })

	detectToolVersionInShellFn = func(shell, cmd string, args ...string) string {
		switch normalizeShell(shell) {
		case shellPosix:
			if cmd == "tool-posix" {
				return "1.2.3"
			}
		case shellCmd:
			if cmd == "tool-cmd" {
				return "2.3.4"
			}
		case shellPowerShell:
			if cmd == "tool-ps" {
				return "3.4.5"
			}
		}
		return ""
	}

	cases := []struct {
		name       string
		shell      string
		tool       string
		constraint string
		want       string
	}{
		{name: "posix", shell: shellPosix, tool: "tool-posix", constraint: ">=1.0.0", want: "1.2.3"},
		{name: "cmd", shell: shellCmd, tool: "tool-cmd", constraint: ">=2.0.0", want: "2.3.4"},
		{name: "powershell", shell: shellPowerShell, tool: "tool-ps", constraint: ">=3.0.0", want: "3.4.5"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			required := map[string]string{"requires.tool." + tc.tool: tc.constraint}
			runtimeCaps := map[string]string{}
			enrichRuntimeHostToolCapabilities(runtimeCaps, required, tc.shell)
			if got := runtimeCaps["host.tool."+tc.tool]; got != tc.want {
				t.Fatalf("expected runtime host capability %q=%q, got %q", tc.tool, tc.want, got)
			}
			if err := validateHostToolRequirements(required, runtimeCaps); err != nil {
				t.Fatalf("expected host validation to pass, got %v", err)
			}
		})
	}
}
