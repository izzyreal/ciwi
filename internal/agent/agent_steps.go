package agent

import (
	"fmt"
	"strings"

	"github.com/izzyreal/ciwi/internal/protocol"
)

type stepMarkerMeta struct {
	index          int
	total          int
	name           string
	yamlLiteral    string
	kind           string
	testName       string
	testFormat     string
	testReport     string
	coverageFormat string
	coverageReport string
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

func jobExecutionEventStep(meta stepMarkerMeta, yamlLiteral, script string) *protocol.JobStepPlanItem {
	return &protocol.JobStepPlanItem{
		Index:          meta.index,
		Total:          meta.total,
		Name:           meta.name,
		Kind:           meta.kind,
		YAMLLiteral:    yamlLiteral,
		Script:         script,
		TestName:       meta.testName,
		TestFormat:     meta.testFormat,
		TestReport:     meta.testReport,
		CoverageFormat: meta.coverageFormat,
		CoverageReport: meta.coverageReport,
	}
}
