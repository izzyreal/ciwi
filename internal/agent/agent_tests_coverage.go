package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/izzyreal/ciwi/internal/protocol"
)

func parseStepCoverageFromFile(execDir string, meta stepMarkerMeta) (*protocol.CoverageReport, error) {
	path := strings.TrimSpace(meta.coverageReport)
	if path == "" {
		return nil, nil
	}
	format := strings.TrimSpace(meta.coverageFormat)
	if format == "" {
		switch strings.ToLower(filepath.Ext(path)) {
		case ".info", ".lcov":
			format = "lcov"
		default:
			format = "go-coverprofile"
		}
	}

	full := filepath.Join(execDir, filepath.FromSlash(path))
	raw, err := os.ReadFile(full)
	if err != nil {
		return nil, fmt.Errorf("read coverage report %q: %w", path, err)
	}
	lines := strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n")

	switch format {
	case "go-coverprofile":
		return parseGoCoverprofileCoverage(lines)
	case "lcov":
		return parseLCOVCoverage(lines)
	default:
		return nil, fmt.Errorf("unsupported coverage format %q", format)
	}
}

func parseGoCoverprofileCoverage(lines []string) (*protocol.CoverageReport, error) {
	type fileStat struct {
		total   int
		covered int
	}
	files := map[string]fileStat{}
	for i, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "mode:") {
			continue
		}
		colon := strings.Index(line, ":")
		if colon <= 0 {
			return nil, fmt.Errorf("invalid coverprofile line %d", i+1)
		}
		path := strings.TrimSpace(line[:colon])
		fields := strings.Fields(strings.TrimSpace(line[colon+1:]))
		if len(fields) != 3 {
			return nil, fmt.Errorf("invalid coverprofile payload line %d", i+1)
		}
		numStmts, err := strconv.Atoi(fields[1])
		if err != nil {
			return nil, fmt.Errorf("invalid statement count line %d: %w", i+1, err)
		}
		count, err := strconv.ParseFloat(fields[2], 64)
		if err != nil {
			return nil, fmt.Errorf("invalid hit count line %d: %w", i+1, err)
		}
		st := files[path]
		st.total += numStmts
		if count > 0 {
			st.covered += numStmts
		}
		files[path] = st
	}

	report := &protocol.CoverageReport{Format: "go-coverprofile"}
	if len(files) == 0 {
		return report, nil
	}
	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		st := files[path]
		f := protocol.CoverageFileReport{
			Path:              path,
			TotalStatements:   st.total,
			CoveredStatements: st.covered,
		}
		if st.total > 0 {
			f.Percent = 100.0 * float64(st.covered) / float64(st.total)
		}
		report.Files = append(report.Files, f)
		report.TotalStatements += st.total
		report.CoveredStatements += st.covered
	}
	if report.TotalStatements > 0 {
		report.Percent = 100.0 * float64(report.CoveredStatements) / float64(report.TotalStatements)
	}
	return report, nil
}

func parseLCOVCoverage(lines []string) (*protocol.CoverageReport, error) {
	type fileStat struct {
		total           int
		covered         int
		seenLF, seenLH  bool
		daTotal, daHits int
	}
	stats := map[string]fileStat{}
	current := ""

	for i, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		switch {
		case strings.HasPrefix(line, "SF:"):
			current = strings.TrimSpace(strings.TrimPrefix(line, "SF:"))
			if current == "" {
				return nil, fmt.Errorf("invalid lcov SF line %d", i+1)
			}
			if _, ok := stats[current]; !ok {
				stats[current] = fileStat{}
			}
		case line == "end_of_record":
			current = ""
		case strings.HasPrefix(line, "LF:"):
			if current == "" {
				continue
			}
			v, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "LF:")))
			if err != nil {
				return nil, fmt.Errorf("invalid lcov LF line %d: %w", i+1, err)
			}
			st := stats[current]
			st.total = v
			st.seenLF = true
			stats[current] = st
		case strings.HasPrefix(line, "LH:"):
			if current == "" {
				continue
			}
			v, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "LH:")))
			if err != nil {
				return nil, fmt.Errorf("invalid lcov LH line %d: %w", i+1, err)
			}
			st := stats[current]
			st.covered = v
			st.seenLH = true
			stats[current] = st
		case strings.HasPrefix(line, "DA:"):
			if current == "" {
				continue
			}
			parts := strings.Split(strings.TrimSpace(strings.TrimPrefix(line, "DA:")), ",")
			if len(parts) < 2 {
				return nil, fmt.Errorf("invalid lcov DA line %d", i+1)
			}
			hits, err := strconv.Atoi(strings.TrimSpace(parts[1]))
			if err != nil {
				return nil, fmt.Errorf("invalid lcov DA hits line %d: %w", i+1, err)
			}
			st := stats[current]
			st.daTotal++
			if hits > 0 {
				st.daHits++
			}
			stats[current] = st
		}
	}

	report := &protocol.CoverageReport{Format: "lcov"}
	if len(stats) == 0 {
		return report, nil
	}
	paths := make([]string, 0, len(stats))
	for path := range stats {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		st := stats[path]
		total := st.total
		covered := st.covered
		if !st.seenLF {
			total = st.daTotal
		}
		if !st.seenLH {
			covered = st.daHits
		}
		f := protocol.CoverageFileReport{
			Path:         path,
			TotalLines:   total,
			CoveredLines: covered,
		}
		if total > 0 {
			f.Percent = 100.0 * float64(covered) / float64(total)
		}
		report.Files = append(report.Files, f)
		report.TotalLines += total
		report.CoveredLines += covered
	}
	if report.TotalLines > 0 {
		report.Percent = 100.0 * float64(report.CoveredLines) / float64(report.TotalLines)
	}
	return report, nil
}
