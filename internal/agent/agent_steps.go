package agent

import (
	"fmt"
	"strings"

	"github.com/izzyreal/ciwi/internal/protocol"
)

type stepMarkerMeta struct {
	index          int
	total          int
	displayIndex   int
	displayTotal   int
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
	index := meta.displayIndex
	if index <= 0 {
		index = meta.index
	}
	total := meta.displayTotal
	if total <= 0 {
		total = meta.total
	}
	name := strings.TrimSpace(meta.name)
	if name != "" {
		name = strings.ReplaceAll(name, "_", " ")
	}
	if total > 0 {
		if name == "" {
			return fmt.Sprintf("Step %d/%d", index, total)
		}
		return fmt.Sprintf("Step %d/%d: %s", index, total, name)
	}
	if name == "" {
		return fmt.Sprintf("Step %d", index)
	}
	return fmt.Sprintf("Step %d: %s", index, name)
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
