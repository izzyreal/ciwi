package requirements

import "testing"

func TestContainerSelectionUsesOneBackendAndKeepsHostConstraints(t *testing.T) {
	caps := map[string]string{"os": "darwin", "arch": "arm64", "shells": "posix", "container.execution": "1", "container.runtime.apple": "1.2.2", "container.native.apple": "linux/arm64", "container.platforms.apple": "linux/arm64,linux/amd64", "container.runtime.docker": "28.0.0", "container.native.docker": "linux/amd64", "container.platforms.docker": "linux/amd64"}
	for _, tc := range []struct{ name, pin, platform, devices, want string }{
		{"native auto", "auto", "", "", "apple"},
		{"translated auto", "auto", "linux/amd64", "", "apple"},
		{"pin docker", "docker", "linux/amd64", "", "docker"},
		{"Docker options", "auto", "linux/amd64", "1", "docker"},
		{"incompatible pinned options", "apple", "linux/arm64", "1", ""},
		{"cannot combine backends", "auto", "linux/arm64", "1", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := map[string]string{"requires.container.runtime": tc.pin, "requires.container.platform": tc.platform, "requires.container.docker_options": tc.devices}
			got, _, err := SelectContainerRuntime(req, caps)
			if got != tc.want || (err != nil) != (tc.want == "") {
				t.Fatalf("got %q, %v", got, err)
			}
		})
	}
	req := map[string]string{"requires.container.runtime": "auto", "os": "linux"}
	if MatchCapabilities(req, caps).Matches {
		t.Fatal("container support must not override host OS constraint")
	}
	delete(req, "os")
	delete(caps, "container.runtime.apple")
	if got, _, err := SelectContainerRuntime(req, caps); got != "docker" || err != nil {
		t.Fatalf("unavailable Apple runtime: %s %v", got, err)
	}
	delete(caps, "container.execution")
	if MatchCapabilities(req, caps).Matches {
		t.Fatal("old agent must not lease new managed job")
	}
}

func TestContainerConstraintsWithoutRuntimeAreNotSilentlyIgnored(t *testing.T) {
	for _, key := range []string{"requires.container.platform", "requires.container.docker_options"} {
		if MatchCapabilities(map[string]string{key: "required"}, nil).Matches {
			t.Fatalf("ignored requirement %s", key)
		}
	}
}
