package version

import "strings"

// Version is set at build time with:
// -ldflags "-X github.com/izzyreal/ciwi/internal/version.Version=vX.Y.Z"
var Version = "dev"

func Current() string {
	v := strings.TrimSpace(Version)
	if v == "" {
		return "dev"
	}
	return v
}
