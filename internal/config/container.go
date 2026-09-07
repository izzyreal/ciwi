package config

import (
	"fmt"
	"path"
	"regexp"
	"strconv"
	"strings"
)

var containerMemoryPattern = regexp.MustCompile(`^[1-9][0-9]*[kKmMgGtT]?$`)

func validateContainerConfig(runs map[string]string) []string {
	var errors []string
	get := func(k string) string { return strings.TrimSpace(runs[k]) }
	image, build := get("container_image"), get("container_build_context")
	managed := image != "" || build != ""
	if image != "" && build != "" {
		errors = append(errors, "runs_on.container_image and container_build_context are mutually exclusive")
	}
	for _, key := range []string{"container_runtime", "container_platform", "container_build_file", "container_cpus", "container_memory", "container_shm_size"} {
		if get(key) != "" && !managed {
			errors = append(errors, "runs_on."+key+" requires container_image or container_build_context")
		}
	}
	if v := get("container_runtime"); v != "" && v != "auto" && v != "docker" && v != "apple" {
		errors = append(errors, "runs_on.container_runtime must be auto, docker, or apple")
	}
	if v := get("container_platform"); v != "" && v != "linux/amd64" && v != "linux/arm64" {
		errors = append(errors, "runs_on.container_platform must be linux/amd64 or linux/arm64")
	}
	if get("container_runtime") == "apple" && (get("container_devices") != "" || get("container_groups") != "") {
		errors = append(errors, "Apple Container does not support container_devices or container_groups")
	}
	if managed && get("shell") != "" && get("shell") != "posix" {
		errors = append(errors, "managed containers require a posix shell")
	}
	if get("container_build_file") != "" && build == "" {
		errors = append(errors, "runs_on.container_build_file requires container_build_context")
	}
	for _, key := range []string{"container_build_context", "container_build_file"} {
		if v := get(key); v != "" && (path.IsAbs(v) || path.Clean(v) == ".." || strings.HasPrefix(path.Clean(v), "../") || strings.ContainsAny(v, "\\:\x00")) {
			errors = append(errors, "runs_on."+key+" must be a relative path within its root")
		}
	}
	if v := get("container_cpus"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			errors = append(errors, "runs_on.container_cpus must be a positive integer")
		}
	}
	for _, key := range []string{"container_memory", "container_shm_size"} {
		if v := get(key); v != "" && !containerMemoryPattern.MatchString(v) {
			errors = append(errors, fmt.Sprintf("runs_on.%s must be positive bytes or an integer with K, M, G, or T suffix", key))
		}
	}
	return errors
}
