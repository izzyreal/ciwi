package agent

import (
	"fmt"
	"strings"
)

type stepMarkerMeta struct {
	index      int
	total      int
	name       string
	kind       string
	testName   string
	testFormat string
	testReport string
	metadata   map[string]string
}

func formatCurrentStep(meta stepMarkerMeta) string {
	name := strings.TrimSpace(meta.name)
	if name == "" {
		name = fmt.Sprintf("Step %d", meta.index)
	} else {
		name = strings.ReplaceAll(name, "_", " ")
	}
	if meta.total > 0 {
		return fmt.Sprintf("Step %d/%d: %s", meta.index, meta.total, name)
	}
	return fmt.Sprintf("Step %d: %s", meta.index, name)
}
