package jobexecution

import (
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/izzyreal/ciwi/internal/protocol"
	"github.com/izzyreal/ciwi/internal/server/httpx"
)

var ansiEscapeRE = regexp.MustCompile(`\x1b\[[0-?]*[ -/]*[@-~]|\x1b\][^\x07]*(?:\x07|\x1b\\)|\x1b[@-Z\\-_]`)

func handleJobEvents(w http.ResponseWriter, r *http.Request, deps HandlerDeps, jobID string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	events, err := deps.Store.ListJobExecutionEvents(jobID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, EventsViewResponse{Events: events})
}

func handleJobLog(w http.ResponseWriter, r *http.Request, deps HandlerDeps, jobID string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	job, err := deps.Store.GetJobExecution(jobID)
	if err != nil {
		http.Error(w, "job not found", http.StatusNotFound)
		return
	}
	events, err := deps.Store.ListJobExecutionEvents(jobID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	format := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format")))
	if format == "" {
		format = "clean"
	}
	var body string
	switch format {
	case "clean":
		body = renderCleanJobLog(job, events)
	case "raw":
		body = renderRawJobLog(job, events)
	default:
		http.Error(w, "format must be clean or raw", http.StatusBadRequest)
		return
	}
	if !strings.HasSuffix(body, "\n") {
		body += "\n"
	}
	filenameFormat := format
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="ciwi-%s-%s.log"`, sanitizeDownloadToken(jobID), filenameFormat))
	_, _ = w.Write([]byte(body))
}

func renderRawJobLog(job protocol.JobExecution, events []protocol.JobExecutionEvent) string {
	if !hasStructuredLogEvents(events) {
		// CIWI_LEGACY_LOG_FALLBACK: historical executions only have output_text tail.
		return normalizeLogText(job.Output)
	}
	var b strings.Builder
	for _, event := range events {
		switch event.Type {
		case protocol.JobExecutionEventTypeSystemMessage:
			if strings.TrimSpace(event.Message) != "" {
				b.WriteString(event.Message)
				if !strings.HasSuffix(event.Message, "\n") {
					b.WriteByte('\n')
				}
			}
		case protocol.JobExecutionEventTypeStepOutput:
			b.WriteString(event.Output)
			if event.Output != "" && !strings.HasSuffix(event.Output, "\n") {
				b.WriteByte('\n')
			}
		case protocol.JobExecutionEventTypeStepFinished:
			if strings.TrimSpace(event.Error) != "" {
				b.WriteString("[run] step failed: ")
				b.WriteString(stepEventTitle(event.Step))
				b.WriteString(" (")
				b.WriteString(event.Error)
				b.WriteString(")\n")
			}
		}
	}
	rendered := normalizeLogText(b.String())
	if strings.TrimSpace(rendered) == "" {
		// CIWI_LEGACY_LOG_FALLBACK: keep raw download useful if event projection is incomplete.
		return normalizeLogText(job.Output)
	}
	return rendered
}

func renderCleanJobLog(job protocol.JobExecution, events []protocol.JobExecutionEvent) string {
	if !hasStructuredLogEvents(events) {
		// CIWI_LEGACY_LOG_FALLBACK: historical executions only have output_text tail.
		var b strings.Builder
		b.WriteString("ciwi legacy job log\n")
		b.WriteString("This execution has no complete structured log events; output below is the persisted tail only.\n\n")
		b.WriteString(stripANSIAndControls(job.Output))
		return b.String()
	}
	var b strings.Builder
	b.WriteString("ciwi job log\n")
	b.WriteString("Job execution ID: " + job.ID + "\n")
	b.WriteString("Status: " + protocol.NormalizeJobExecutionStatus(job.Status) + "\n")
	if !job.StartedUTC.IsZero() {
		b.WriteString("Started: " + job.StartedUTC.UTC().Format(time.RFC3339Nano) + "\n")
	}
	if !job.FinishedUTC.IsZero() {
		b.WriteString("Finished: " + job.FinishedUTC.UTC().Format(time.RFC3339Nano) + "\n")
	}
	if job.ExitCode != nil {
		b.WriteString(fmt.Sprintf("Exit code: %d\n", *job.ExitCode))
	}
	if strings.TrimSpace(job.Error) != "" {
		b.WriteString("Error: " + stripANSIAndControls(job.Error) + "\n")
	}
	b.WriteByte('\n')

	steps := groupStepEvents(events)
	for _, step := range steps {
		writeCleanStepLog(&b, step)
	}
	return b.String()
}

func hasStructuredLogEvents(events []protocol.JobExecutionEvent) bool {
	// CIWI_LEGACY_LOG_FALLBACK: start-only step events are not enough for downloads.
	for _, event := range events {
		if event.Step != nil && (event.Type == protocol.JobExecutionEventTypeStepOutput || event.Type == protocol.JobExecutionEventTypeStepFinished) {
			return true
		}
	}
	return false
}

type stepLogGroup struct {
	step    protocol.JobStepPlanItem
	started time.Time
	output  strings.Builder
	finish  *protocol.JobExecutionEvent
}

func groupStepEvents(events []protocol.JobExecutionEvent) []*stepLogGroup {
	byIndex := map[int]*stepLogGroup{}
	order := []int{}
	for _, event := range events {
		if event.Step == nil {
			continue
		}
		idx := event.Step.Index
		if idx <= 0 {
			idx = len(order) + 1
		}
		group := byIndex[idx]
		if group == nil {
			group = &stepLogGroup{step: *event.Step}
			byIndex[idx] = group
			order = append(order, idx)
		}
		if strings.TrimSpace(group.step.Name) == "" {
			group.step = *event.Step
		}
		switch event.Type {
		case protocol.JobExecutionEventTypeStepStarted:
			group.started = event.TimestampUTC
		case protocol.JobExecutionEventTypeStepOutput:
			group.output.WriteString(event.Output)
			if event.Output != "" && !strings.HasSuffix(event.Output, "\n") {
				group.output.WriteByte('\n')
			}
		case protocol.JobExecutionEventTypeStepFinished:
			ev := event
			group.finish = &ev
		}
	}
	sort.Ints(order)
	out := make([]*stepLogGroup, 0, len(order))
	for _, idx := range order {
		out = append(out, byIndex[idx])
	}
	return out
}

func writeCleanStepLog(b *strings.Builder, group *stepLogGroup) {
	sep := strings.Repeat("-", 80)
	title := stepEventTitle(&group.step)
	b.WriteString(sep + "\n")
	b.WriteString(title + "\n")
	b.WriteString(sep + "\n")
	if !group.started.IsZero() {
		b.WriteString("Start time: " + group.started.UTC().Format(time.RFC3339Nano) + "\n")
	}
	if group.finish != nil && group.finish.DurationMS > 0 {
		b.WriteString("Step duration: " + formatDurationMS(group.finish.DurationMS) + "\n")
	}
	if group.finish != nil && group.finish.ExitCode != nil {
		b.WriteString(fmt.Sprintf("Exit code: %d\n", *group.finish.ExitCode))
	}
	if group.finish != nil && strings.TrimSpace(group.finish.Error) != "" {
		b.WriteString("Error: " + stripANSIAndControls(group.finish.Error) + "\n")
	}
	b.WriteString("\nYAML literal:\n'''\n")
	yamlLiteral := group.step.YAMLLiteral
	if strings.TrimSpace(yamlLiteral) == "" {
		yamlLiteral = group.step.Script
	}
	b.WriteString(stripANSIAndControls(yamlLiteral))
	if !strings.HasSuffix(yamlLiteral, "\n") {
		b.WriteByte('\n')
	}
	b.WriteString("'''\n\nExpanded command:\n'''\n")
	b.WriteString(stripANSIAndControls(group.step.Script))
	if !strings.HasSuffix(group.step.Script, "\n") {
		b.WriteByte('\n')
	}
	b.WriteString("'''\n\nOutput:\n'''\n")
	b.WriteString(stripANSIAndControls(group.output.String()))
	if !strings.HasSuffix(group.output.String(), "\n") {
		b.WriteByte('\n')
	}
	b.WriteString("'''\n\n")
	if group.finish == nil {
		b.WriteString("Step finished: not reported\n\n")
	} else if strings.TrimSpace(group.finish.Error) != "" || group.finish.ExitCode != nil {
		b.WriteString("Step finished: failed\n\n")
	} else {
		b.WriteString("Step finished: succeeded\n\n")
	}
}

func stepEventTitle(step *protocol.JobStepPlanItem) string {
	if step == nil {
		return "Step"
	}
	name := strings.TrimSpace(step.Name)
	name = strings.Join(strings.Fields(name), " ")
	if name == "" {
		name = fmt.Sprintf("Step %d", step.Index)
	} else {
		name = strings.ReplaceAll(name, "_", " ")
	}
	if step.Total > 0 && step.Index > 0 {
		return fmt.Sprintf("Step %d/%d: %s", step.Index, step.Total, name)
	}
	if step.Index > 0 {
		return fmt.Sprintf("Step %d: %s", step.Index, name)
	}
	return name
}

func stripANSIAndControls(text string) string {
	text = normalizeLogText(text)
	text = ansiEscapeRE.ReplaceAllString(text, "")
	var b strings.Builder
	for len(text) > 0 {
		r, size := utf8.DecodeRuneInString(text)
		if r == utf8.RuneError && size == 1 {
			text = text[size:]
			continue
		}
		text = text[size:]
		if r == '\n' || r == '\t' || r >= 0x20 {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func normalizeLogText(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	return text
}

func formatDurationMS(ms int64) string {
	if ms < 0 {
		ms = 0
	}
	return (time.Duration(ms) * time.Millisecond).Round(time.Millisecond).String()
}

func sanitizeDownloadToken(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return "job"
	}
	var b strings.Builder
	for _, r := range v {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "job"
	}
	return b.String()
}
