package agent

import (
	"fmt"
	"strconv"
	"strings"
)

func extractCurrentStepFromOutput(output string) string {
	current := ""
	for _, line := range strings.Split(output, "\n") {
		meta, ok := parseStepMarkerLine(line)
		if !ok {
			continue
		}
		current = formatCurrentStep(meta)
	}
	return current
}

type stepMarkerMeta struct {
	index      int
	total      int
	name       string
	kind       string
	testName   string
	testFormat string
	testReport string
}

func parseStepMarkerLine(line string) (stepMarkerMeta, bool) {
	line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
	line = strings.Trim(line, `"'`)
	if !strings.HasPrefix(line, "__CIWI_STEP_BEGIN__") {
		return stepMarkerMeta{}, false
	}
	meta := stepMarkerMeta{}
	parts := strings.Fields(strings.TrimSpace(strings.TrimPrefix(line, "__CIWI_STEP_BEGIN__")))
	for _, part := range parts {
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			continue
		}
		switch strings.TrimSpace(kv[0]) {
		case "index":
			meta.index, _ = strconv.Atoi(strings.TrimSpace(kv[1]))
		case "total":
			meta.total, _ = strconv.Atoi(strings.TrimSpace(kv[1]))
		case "name":
			meta.name = strings.TrimSpace(kv[1])
		case "kind":
			meta.kind = strings.TrimSpace(kv[1])
		case "test_name":
			meta.testName = strings.TrimSpace(kv[1])
		case "test_format":
			meta.testFormat = strings.TrimSpace(kv[1])
		case "test_report":
			meta.testReport = strings.TrimSpace(kv[1])
		}
	}
	if meta.index <= 0 {
		return stepMarkerMeta{}, false
	}
	return meta, true
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
