package agent

import (
	"encoding/json"
	"strings"

	"github.com/izzyreal/ciwi/internal/protocol"
)

type goTestEvent struct {
	Action  string  `json:"Action"`
	Package string  `json:"Package"`
	Test    string  `json:"Test"`
	Elapsed float64 `json:"Elapsed"`
	Output  string  `json:"Output"`
}

func parseGoTestJSONSuite(name string, lines []string) protocol.TestSuiteReport {
	type caseKey struct {
		pkg  string
		test string
	}
	type caseState struct {
		pkg      string
		name     string
		status   string
		elapsed  float64
		outputSB strings.Builder
	}

	order := make([]caseKey, 0)
	cases := make(map[caseKey]*caseState)

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || !strings.HasPrefix(line, "{") {
			continue
		}
		var ev goTestEvent
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			continue
		}
		if strings.TrimSpace(ev.Test) == "" {
			continue
		}
		key := caseKey{pkg: ev.Package, test: ev.Test}
		st, ok := cases[key]
		if !ok {
			st = &caseState{pkg: ev.Package, name: ev.Test}
			cases[key] = st
			order = append(order, key)
		}
		if ev.Output != "" {
			st.outputSB.WriteString(ev.Output)
		}
		switch ev.Action {
		case "pass", "fail", "skip":
			st.status = ev.Action
			if ev.Elapsed > 0 {
				st.elapsed = ev.Elapsed
			}
		}
	}

	suite := protocol.TestSuiteReport{
		Name:   name,
		Format: "go-test-json",
		Cases:  make([]protocol.TestCase, 0, len(order)),
	}
	for _, key := range order {
		st := cases[key]
		status := st.status
		if status == "" {
			status = "unknown"
		}
		tc := protocol.TestCase{
			Package:         st.pkg,
			Name:            st.name,
			Status:          status,
			DurationSeconds: st.elapsed,
			Output:          st.outputSB.String(),
		}
		suite.Cases = append(suite.Cases, tc)
		suite.Total++
		switch status {
		case "pass":
			suite.Passed++
		case "fail":
			suite.Failed++
		case "skip":
			suite.Skipped++
		}
	}
	return suite
}
