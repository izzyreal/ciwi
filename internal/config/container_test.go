package config

import "testing"

func TestContainerConfiguration(t *testing.T) {
	for _, tc := range []struct {
		name  string
		runs  map[string]string
		valid bool
	}{
		{"existing image", map[string]string{"container_image": "local-image"}, true},
		{"build", map[string]string{"container_build_context": "packaging/linux", "container_platform": "linux/amd64", "container_memory": "4G", "container_cpus": "4"}, true},
		{"both sources", map[string]string{"container_image": "x", "container_build_context": "."}, false},
		{"unknown runtime", map[string]string{"container_image": "x", "container_runtime": "podman"}, false},
		{"Apple device", map[string]string{"container_image": "x", "container_runtime": "apple", "container_devices": "/dev/snd"}, false},
		{"auto device", map[string]string{"container_image": "x", "container_runtime": "auto", "container_devices": "/dev/snd"}, true},
		{"escaping context", map[string]string{"container_build_context": "../outside"}, false},
		{"escaping Dockerfile", map[string]string{"container_build_context": ".", "container_build_file": "../Dockerfile"}, false},
		{"invalid resources", map[string]string{"container_image": "x", "container_memory": "-1", "container_cpus": "0"}, false},
		{"host only", map[string]string{"container_runtime": "apple"}, false},
		{"non-posix", map[string]string{"container_image": "x", "shell": "powershell"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if errors := validateContainerConfig(tc.runs); (len(errors) == 0) != tc.valid {
				t.Fatalf("errors: %v", errors)
			}
		})
	}
}
