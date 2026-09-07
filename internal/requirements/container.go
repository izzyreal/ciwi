package requirements

import (
	"fmt"
	"strings"
)

const ContainerExecutionCapability = "container.execution"

// ContainerRequirementKeys are matched together against one available backend.
func IsContainerRuntimeRequirement(key string) bool {
	return key == "requires.container.runtime" || key == "requires.container.platform" || key == "requires.container.docker_options"
}

// SelectContainerRuntime is shared by scheduling and execution. Host constraints
// remain independent; platform always describes the Linux guest.
func SelectContainerRuntime(required, observed map[string]string) (string, string, error) {
	requested := strings.TrimSpace(required["requires.container.runtime"])
	if requested == "" {
		requested = "auto"
	}
	candidates := []string{"docker", "apple"}
	if observed["os"] == "darwin" && observed["arch"] == "arm64" {
		candidates = []string{"apple", "docker"}
	}
	if requested != "auto" {
		candidates = []string{requested}
	}
	var reasons []string
	for _, backend := range candidates {
		reason := ""
		platform := strings.TrimSpace(required["requires.container.platform"])
		if platform == "" {
			platform = observed["container.native."+backend]
		}
		switch {
		case backend != "docker" && backend != "apple":
			reason = "unknown runtime"
		case observed[ContainerExecutionCapability] != "1":
			reason = "agent needs managed container support"
		case observed["container.runtime."+backend] == "":
			reason = observed["container.error."+backend]
			if reason == "" {
				reason = "runtime unavailable"
			}
		case backend == "apple" && required["requires.container.docker_options"] == "1":
			reason = "device/group options require Docker"
		default:
			supported := false
			for _, p := range strings.Split(observed["container.platforms."+backend], ",") {
				if p == platform && p != "" {
					supported = true
				}
			}
			if !supported {
				reason = "unsupported platform " + platform
			}
		}
		if reason == "" {
			return backend, platform, nil
		}
		reasons = append(reasons, backend+": "+reason)
	}
	return "", "", fmt.Errorf("no eligible container runtime (%s)", strings.Join(reasons, "; "))
}
