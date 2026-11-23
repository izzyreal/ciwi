package domain

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

var autoBumpVersionPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)

func NextAutoBumpVersion(raw, mode string) (string, error) {
	raw = strings.TrimSpace(raw)
	if !autoBumpVersionPattern.MatchString(raw) {
		return "", fmt.Errorf("invalid auto-bump version %q", raw)
	}
	parts := strings.Split(raw, ".")
	major, err := strconv.ParseUint(parts[0], 10, 64)
	if err != nil {
		return "", fmt.Errorf("invalid auto-bump major version: %w", err)
	}
	minor, err := strconv.ParseUint(parts[1], 10, 64)
	if err != nil {
		return "", fmt.Errorf("invalid auto-bump minor version: %w", err)
	}
	patch, err := strconv.ParseUint(parts[2], 10, 64)
	if err != nil {
		return "", fmt.Errorf("invalid auto-bump patch version: %w", err)
	}
	switch mode {
	case "patch":
		if patch == math.MaxUint64 {
			return "", fmt.Errorf("auto-bump patch version overflows")
		}
		patch++
	case "minor":
		if minor == math.MaxUint64 {
			return "", fmt.Errorf("auto-bump minor version overflows")
		}
		minor++
		patch = 0
	case "major":
		if major == math.MaxUint64 {
			return "", fmt.Errorf("auto-bump major version overflows")
		}
		major++
		minor, patch = 0, 0
	default:
		return "", fmt.Errorf("unsupported auto-bump mode %q", mode)
	}
	return fmt.Sprintf("%d.%d.%d", major, minor, patch), nil
}
