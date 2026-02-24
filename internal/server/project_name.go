package server

import (
	"strings"

	"github.com/izzyreal/ciwi/internal/protocol"
)

func displayProjectName(raw string) string {
	name := strings.TrimSpace(raw)
	if name == "" {
		return ""
	}
	if i := strings.Index(name, "@"); i > 0 {
		return strings.TrimSpace(name[:i])
	}
	return name
}

func applyDisplayProjectNames(projects []protocol.ProjectSummary) {
	for i := range projects {
		projects[i].Name = displayProjectName(projects[i].Name)
	}
}

func applyDisplayProjectNameDetail(detail *protocol.ProjectDetail) {
	if detail == nil {
		return
	}
	detail.Name = displayProjectName(detail.Name)
}
