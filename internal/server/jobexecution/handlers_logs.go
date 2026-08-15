package jobexecution

import (
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/izzyreal/ciwi/internal/domain"
	"github.com/izzyreal/ciwi/internal/protocol"
	"github.com/izzyreal/ciwi/internal/server/httpx"
)

var ansiEscapeRE = regexp.MustCompile(`\x1b\[[0-?]*[ -/]*[@-~]|\x1b\][^\x07]*(?:\x07|\x1b\\)|\x1b[@-Z\\-_]`)

func handleJobEvents(w http.ResponseWriter, r *http.Request, deps HandlerDeps, jobID string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	afterID := int64(0)
	if raw := strings.TrimSpace(r.URL.Query().Get("after_id")); raw != "" {
		parsed, parseErr := strconv.ParseInt(raw, 10, 64)
		if parseErr != nil || parsed < 0 {
			http.Error(w, "after_id must be a non-negative integer", http.StatusBadRequest)
			return
		}
		afterID = parsed
	}
	events, err := deps.Store.ListJobExecutionEventsAfter(jobID, afterID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	nextEventID := afterID
	if len(events) > 0 {
		nextEventID = events[len(events)-1].ID
	}
	httpx.WriteJSON(w, http.StatusOK, EventsViewResponse{Events: events, NextEventID: nextEventID})
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
	format := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format")))
	if format == "" {
		format = "clean"
	}
	if format != "clean" && format != "raw" {
		http.Error(w, "format must be clean or raw", http.StatusBadRequest)
		return
	}
	fileName := fmt.Sprintf("ciwi-%s-%s.log", sanitizeDownloadToken(job.ID), format)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, fileName))
	_, _ = WriteJobLog(w, deps.Store, job, format)
}

type indexedJobLogStore interface {
	GetJobLogDescriptor(string) (domain.JobLogDescriptor, error)
	GetJobLogPage(string, string, domain.JobLogPageMode, int64) (domain.JobLogPage, error)
	ListJobExecutionTimelineEvents(string) ([]protocol.JobExecutionEvent, error)
}

type pagedJobEventStore interface {
	ListJobExecutionEventsPageAfter(string, int64, int) ([]protocol.JobExecutionEvent, error)
}

// WriteJobLog writes a complete log without retaining the complete rendered
// body in memory. Indexed executions stream clean chunks; raw event histories
// stream in bounded event pages.
func WriteJobLog(w io.Writer, store interface {
	ListJobExecutionEvents(string) ([]protocol.JobExecutionEvent, error)
}, job protocol.JobExecution, format string) (string, error) {
	format = strings.ToLower(strings.TrimSpace(format))
	if format == "" {
		format = "clean"
	}
	if format != "clean" && format != "raw" {
		return "", fmt.Errorf("format must be clean or raw")
	}
	fileName := fmt.Sprintf("ciwi-%s-%s.log", sanitizeDownloadToken(job.ID), format)
	if format == "clean" {
		if indexed, ok := store.(indexedJobLogStore); ok {
			descriptor, err := indexed.GetJobLogDescriptor(job.ID)
			if err == nil && descriptor.Available {
				return fileName, writeIndexedCleanJobLog(w, indexed, job, descriptor)
			}
		}
	}
	if paged, ok := store.(pagedJobEventStore); ok {
		if format == "raw" {
			return fileName, writePagedJobLog(w, paged, job, format)
		}
		return fileName, writePagedCleanJobLog(w, paged, job)
	}
	events, err := store.ListJobExecutionEvents(job.ID)
	if err != nil {
		return "", err
	}
	body, _, err := RenderJobLog(job, events, format)
	if err != nil {
		return "", err
	}
	_, err = io.WriteString(w, body)
	return fileName, err
}

func writePagedJobLog(w io.Writer, store pagedJobEventStore, job protocol.JobExecution, format string) error {
	cursor := int64(0)
	for {
		events, err := store.ListJobExecutionEventsPageAfter(job.ID, cursor, 128)
		if err != nil {
			return err
		}
		if len(events) == 0 {
			return nil
		}
		var body string
		if format == "raw" {
			body = renderRawJobLog(job, events)
		} else {
			body = renderCleanJobLog(job, events)
		}
		if _, err := io.WriteString(w, body); err != nil {
			return err
		}
		next := events[len(events)-1].ID
		if next <= cursor {
			return nil
		}
		cursor = next
		if len(events) < 128 {
			return nil
		}
	}
}

func writePagedCleanJobLog(w io.Writer, store pagedJobEventStore, job protocol.JobExecution) error {
	if _, err := io.WriteString(w, cleanJobLogHeader(job)); err != nil {
		return err
	}
	cursor := int64(0)
	openItem := ""
	openKind := ""
	closeOpen := func(finished *protocol.JobExecutionEvent) error {
		if openItem == "" {
			return nil
		}
		label := "Ciwi phase"
		if openKind == "step" {
			label = "Job step"
		}
		if finished != nil {
			var metadata strings.Builder
			if finished.DurationMS > 0 {
				metadata.WriteString("Duration: " + formatDurationMS(finished.DurationMS) + "\n")
			}
			if finished.ExitCode != nil {
				fmt.Fprintf(&metadata, "Exit code: %d\n", *finished.ExitCode)
			}
			if strings.TrimSpace(finished.Error) != "" {
				metadata.WriteString("Error: " + stripANSIAndControls(finished.Error) + "\n")
			}
			if metadata.Len() > 0 {
				if _, err := io.WriteString(w, "\n"+metadata.String()); err != nil {
					return err
				}
			}
		}
		if err := writeIndexedFinish(w, finished, label); err != nil {
			return err
		}
		openItem, openKind = "", ""
		return nil
	}
	openEvent := func(event protocol.JobExecutionEvent, itemID, kind string) error {
		if openItem == itemID {
			return nil
		}
		if err := closeOpen(nil); err != nil {
			return err
		}
		openItem, openKind = itemID, kind
		var b strings.Builder
		sep := strings.Repeat("-", 80)
		if kind == "step" {
			b.WriteString(sep + "\n" + stepEventTitle(event.Step) + "\n" + sep + "\n")
			if event.Type == protocol.JobExecutionEventTypeStepStarted && !event.TimestampUTC.IsZero() {
				b.WriteString("Start time: " + event.TimestampUTC.UTC().Format(time.RFC3339Nano) + "\n")
			}
			yamlLiteral, script := "", ""
			if event.Step != nil {
				yamlLiteral, script = event.Step.YAMLLiteral, event.Step.Script
				if strings.TrimSpace(yamlLiteral) == "" {
					yamlLiteral = script
				}
			}
			b.WriteString("\nYAML literal:\n'''\n" + stripANSIAndControls(yamlLiteral))
			if !strings.HasSuffix(yamlLiteral, "\n") {
				b.WriteByte('\n')
			}
			b.WriteString("'''\n\nExpanded command:\n'''\n" + stripANSIAndControls(script))
			if !strings.HasSuffix(script, "\n") {
				b.WriteByte('\n')
			}
			b.WriteString("'''\n\nOutput:\n'''\n")
		} else {
			b.WriteString(sep + "\n" + phaseEventTitle(event.Phase) + "\n" + sep + "\n")
			if event.Type == protocol.JobExecutionEventTypePhaseStarted && !event.TimestampUTC.IsZero() {
				b.WriteString("Start time: " + event.TimestampUTC.UTC().Format(time.RFC3339Nano) + "\n")
			}
			description := ""
			if event.Phase != nil {
				description = event.Phase.Description
			}
			b.WriteString("\nDetails:\n" + stripANSIAndControls(description))
			if !strings.HasSuffix(description, "\n") {
				b.WriteByte('\n')
			}
			b.WriteString("\nOutput:\n'''\n")
		}
		_, err := io.WriteString(w, b.String())
		return err
	}
	for {
		events, err := store.ListJobExecutionEventsPageAfter(job.ID, cursor, 128)
		if err != nil {
			return err
		}
		for _, event := range events {
			if event.Type == protocol.JobExecutionEventTypeSystemMessage {
				if err := closeOpen(nil); err != nil {
					return err
				}
				message := strings.TrimSpace(stripANSIAndControls(event.Message))
				if message != "" {
					if _, err := io.WriteString(w, message+"\n\n"); err != nil {
						return err
					}
				}
				continue
			}
			itemID, kind := "", ""
			if event.Step != nil {
				itemID, kind = fmt.Sprintf("step:%d", event.Step.Index), "step"
			} else if event.Phase != nil {
				itemID, kind = strings.TrimSpace(event.Phase.ID), "phase"
			}
			if itemID == "" {
				continue
			}
			if err := openEvent(event, itemID, kind); err != nil {
				return err
			}
			if event.Type == protocol.JobExecutionEventTypeStepOutput || event.Type == protocol.JobExecutionEventTypePhaseOutput {
				if _, err := io.WriteString(w, stripANSIAndControls(event.Output)); err != nil {
					return err
				}
			}
			if event.Type == protocol.JobExecutionEventTypeStepFinished || event.Type == protocol.JobExecutionEventTypePhaseFinished {
				finished := event
				if err := closeOpen(&finished); err != nil {
					return err
				}
			}
		}
		if len(events) == 0 {
			break
		}
		next := events[len(events)-1].ID
		if next <= cursor {
			break
		}
		cursor = next
		if len(events) < 128 {
			break
		}
	}
	return closeOpen(nil)
}

func writeIndexedCleanJobLog(w io.Writer, store indexedJobLogStore, job protocol.JobExecution, descriptor domain.JobLogDescriptor) error {
	if _, err := io.WriteString(w, cleanJobLogHeader(job)); err != nil {
		return err
	}
	lifecycle, err := store.ListJobExecutionTimelineEvents(job.ID)
	if err != nil {
		return err
	}
	byItem := make(map[string][]protocol.JobExecutionEvent)
	for _, event := range lifecycle {
		itemID := ""
		if event.Step != nil {
			itemID = fmt.Sprintf("step:%d", event.Step.Index)
		} else if event.Phase != nil {
			itemID = strings.TrimSpace(event.Phase.ID)
		}
		if itemID != "" {
			byItem[itemID] = append(byItem[itemID], event)
		}
	}
	streams := make(map[string]bool, len(descriptor.Streams))
	for _, stream := range descriptor.Streams {
		streams[stream.ItemID] = true
	}
	if streams[""] {
		if err := writeIndexedLogStream(w, store, job.ID, ""); err != nil {
			return err
		}
		if _, err := io.WriteString(w, "\n\n"); err != nil {
			return err
		}
	}
	for _, item := range protocol.BuildJobExecutionTimeline(job) {
		events := byItem[item.ID]
		if len(events) == 0 && !streams[item.ID] {
			continue
		}
		if item.Kind == "step" {
			var step protocol.JobStepPlanItem
			for _, candidate := range job.StepPlan {
				if candidate.Index == item.StepIndex {
					step = candidate
					break
				}
			}
			step.Index, step.Total = item.StepIndex, len(job.StepPlan)
			if err := writeIndexedStepLog(w, store, job.ID, item.ID, step, events); err != nil {
				return err
			}
			continue
		}
		phase, _ := protocol.TimelinePhase(protocol.BuildJobExecutionTimeline(job), item.ID)
		if err := writeIndexedPhaseLog(w, store, job.ID, item.ID, phase, events); err != nil {
			return err
		}
	}
	return nil
}

func cleanJobLogHeader(job protocol.JobExecution) string {
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
		fmt.Fprintf(&b, "Exit code: %d\n", *job.ExitCode)
	}
	if strings.TrimSpace(job.Error) != "" {
		b.WriteString("Error: " + stripANSIAndControls(job.Error) + "\n")
	}
	b.WriteByte('\n')
	return b.String()
}

func writeIndexedLogStream(w io.Writer, store indexedJobLogStore, jobID, itemID string) error {
	mode, cursor := domain.JobLogPageHead, int64(0)
	for {
		page, err := store.GetJobLogPage(jobID, itemID, mode, cursor)
		if err != nil {
			return err
		}
		for _, chunk := range page.Chunks {
			if _, err := io.WriteString(w, chunk.Text); err != nil {
				return err
			}
		}
		if !page.HasAfter || page.LastCursor <= cursor {
			return nil
		}
		mode, cursor = domain.JobLogPageAfter, page.LastCursor
	}
}

func logLifecycle(events []protocol.JobExecutionEvent) (time.Time, *protocol.JobExecutionEvent) {
	var started time.Time
	var finished *protocol.JobExecutionEvent
	for _, event := range events {
		switch event.Type {
		case protocol.JobExecutionEventTypeStepStarted, protocol.JobExecutionEventTypePhaseStarted:
			started = event.TimestampUTC
		case protocol.JobExecutionEventTypeStepFinished, protocol.JobExecutionEventTypePhaseFinished:
			copy := event
			finished = &copy
		}
	}
	return started, finished
}

func writeIndexedStepLog(w io.Writer, store indexedJobLogStore, jobID, itemID string, step protocol.JobStepPlanItem, events []protocol.JobExecutionEvent) error {
	started, finished := logLifecycle(events)
	var b strings.Builder
	sep := strings.Repeat("-", 80)
	b.WriteString(sep + "\n" + stepEventTitle(&step) + "\n" + sep + "\n")
	writeLifecycleMetadata(&b, started, finished, "Job step duration")
	b.WriteString("\nYAML literal:\n'''\n")
	yamlLiteral := step.YAMLLiteral
	if strings.TrimSpace(yamlLiteral) == "" {
		yamlLiteral = step.Script
	}
	b.WriteString(stripANSIAndControls(yamlLiteral))
	if !strings.HasSuffix(yamlLiteral, "\n") {
		b.WriteByte('\n')
	}
	b.WriteString("'''\n\nExpanded command:\n'''\n" + stripANSIAndControls(step.Script))
	if !strings.HasSuffix(step.Script, "\n") {
		b.WriteByte('\n')
	}
	b.WriteString("'''\n\nOutput:\n'''\n")
	if _, err := io.WriteString(w, b.String()); err != nil {
		return err
	}
	if err := writeIndexedLogStream(w, store, jobID, itemID); err != nil {
		return err
	}
	return writeIndexedFinish(w, finished, "Job step")
}

func writeIndexedPhaseLog(w io.Writer, store indexedJobLogStore, jobID, itemID string, phase protocol.JobExecutionPhase, events []protocol.JobExecutionEvent) error {
	started, finished := logLifecycle(events)
	var b strings.Builder
	sep := strings.Repeat("-", 80)
	b.WriteString(sep + "\n" + phaseEventTitle(&phase) + "\n" + sep + "\n")
	writeLifecycleMetadata(&b, started, finished, "Ciwi phase duration")
	b.WriteString("\nDetails:\n" + stripANSIAndControls(phase.Description))
	if !strings.HasSuffix(phase.Description, "\n") {
		b.WriteByte('\n')
	}
	b.WriteString("\nOutput:\n'''\n")
	if _, err := io.WriteString(w, b.String()); err != nil {
		return err
	}
	if err := writeIndexedLogStream(w, store, jobID, itemID); err != nil {
		return err
	}
	return writeIndexedFinish(w, finished, "Ciwi phase")
}

func writeLifecycleMetadata(b *strings.Builder, started time.Time, finished *protocol.JobExecutionEvent, durationLabel string) {
	if !started.IsZero() {
		b.WriteString("Start time: " + started.UTC().Format(time.RFC3339Nano) + "\n")
	}
	if finished != nil && finished.DurationMS > 0 {
		b.WriteString(durationLabel + ": " + formatDurationMS(finished.DurationMS) + "\n")
	}
	if finished != nil && finished.ExitCode != nil {
		fmt.Fprintf(b, "Exit code: %d\n", *finished.ExitCode)
	}
	if finished != nil && strings.TrimSpace(finished.Error) != "" {
		b.WriteString("Error: " + stripANSIAndControls(finished.Error) + "\n")
	}
}

func writeIndexedFinish(w io.Writer, finished *protocol.JobExecutionEvent, label string) error {
	status := "not reported"
	if finished != nil {
		status = "succeeded"
		if strings.TrimSpace(finished.Error) != "" || (finished.ExitCode != nil && *finished.ExitCode != 0) {
			status = "failed"
		}
	}
	_, err := fmt.Fprintf(w, "\n'''\n\n%s finished: %s\n\n", label, status)
	return err
}

// RenderJobLog generates the same downloadable log used by the HTTP and native clients.
func RenderJobLog(job protocol.JobExecution, events []protocol.JobExecutionEvent, format string) (body, fileName string, err error) {
	format = strings.ToLower(strings.TrimSpace(format))
	if format == "" {
		format = "clean"
	}
	switch format {
	case "clean":
		body = renderCleanJobLog(job, events)
	case "raw":
		body = renderRawJobLog(job, events)
	default:
		return "", "", fmt.Errorf("format must be clean or raw")
	}
	if !strings.HasSuffix(body, "\n") {
		body += "\n"
	}
	return body, fmt.Sprintf("ciwi-%s-%s.log", sanitizeDownloadToken(job.ID), format), nil
}

func renderRawJobLog(_ protocol.JobExecution, events []protocol.JobExecutionEvent) string {
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
		case protocol.JobExecutionEventTypeStepOutput, protocol.JobExecutionEventTypePhaseOutput:
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
		case protocol.JobExecutionEventTypePhaseFinished:
			if strings.TrimSpace(event.Error) != "" {
				b.WriteString("[phase] failed: ")
				b.WriteString(phaseEventTitle(event.Phase))
				b.WriteString(" (")
				b.WriteString(event.Error)
				b.WriteString(")\n")
			}
		}
	}
	return normalizeLogText(b.String())
}

func renderCleanJobLog(job protocol.JobExecution, events []protocol.JobExecutionEvent) string {
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

	for _, unit := range groupCleanLogUnits(job, events) {
		if unit.step != nil {
			writeCleanStepLog(&b, unit.step)
			continue
		}
		if unit.phase != nil {
			writeCleanPhaseLog(&b, unit.phase)
			continue
		}
		message := strings.TrimSpace(stripANSIAndControls(unit.message))
		if message != "" {
			b.WriteString(message)
			b.WriteString("\n\n")
		}
	}
	return b.String()
}

type stepLogGroup struct {
	step    protocol.JobStepPlanItem
	started time.Time
	output  strings.Builder
	finish  *protocol.JobExecutionEvent
}

type phaseLogGroup struct {
	phase   protocol.JobExecutionPhase
	started time.Time
	output  strings.Builder
	finish  *protocol.JobExecutionEvent
}

type cleanLogUnit struct {
	step    *stepLogGroup
	phase   *phaseLogGroup
	message string
}

func groupCleanLogUnits(job protocol.JobExecution, events []protocol.JobExecutionEvent) []cleanLogUnit {
	byIndex := map[int]*stepLogGroup{}
	byPhaseID := map[string]*phaseLogGroup{}
	timeline := protocol.BuildJobExecutionTimeline(job)
	units := []cleanLogUnit{}
	for _, event := range events {
		if event.Type == protocol.JobExecutionEventTypeSystemMessage {
			if event.Message != "" {
				units = append(units, cleanLogUnit{message: event.Message})
			}
			continue
		}
		if event.Phase != nil {
			id := strings.TrimSpace(event.Phase.ID)
			group := byPhaseID[id]
			if group == nil {
				phase := *event.Phase
				if timelinePhase, ok := protocol.TimelinePhase(timeline, id); ok {
					phase = timelinePhase
				}
				group = &phaseLogGroup{phase: phase}
				byPhaseID[id] = group
				units = append(units, cleanLogUnit{phase: group})
			}
			switch event.Type {
			case protocol.JobExecutionEventTypePhaseStarted:
				group.started = event.TimestampUTC
			case protocol.JobExecutionEventTypePhaseOutput:
				group.output.WriteString(event.Output)
				if event.Output != "" && !strings.HasSuffix(event.Output, "\n") {
					group.output.WriteByte('\n')
				}
			case protocol.JobExecutionEventTypePhaseFinished:
				ev := event
				group.finish = &ev
			}
			continue
		}
		if event.Step == nil {
			continue
		}
		idx := event.Step.Index
		if idx <= 0 {
			idx = len(byIndex) + 1
		}
		group := byIndex[idx]
		if group == nil {
			step := *event.Step
			group = &stepLogGroup{step: step}
			byIndex[idx] = group
			units = append(units, cleanLogUnit{step: group})
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
	return units
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
		b.WriteString("Job step duration: " + formatDurationMS(group.finish.DurationMS) + "\n")
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
		b.WriteString("Job step finished: not reported\n\n")
	} else if strings.TrimSpace(group.finish.Error) != "" || group.finish.ExitCode != nil {
		b.WriteString("Job step finished: failed\n\n")
	} else {
		b.WriteString("Job step finished: succeeded\n\n")
	}
}

func writeCleanPhaseLog(b *strings.Builder, group *phaseLogGroup) {
	sep := strings.Repeat("-", 80)
	b.WriteString(sep + "\n")
	b.WriteString(phaseEventTitle(&group.phase) + "\n")
	b.WriteString(sep + "\n")
	if !group.started.IsZero() {
		b.WriteString("Start time: " + group.started.UTC().Format(time.RFC3339Nano) + "\n")
	}
	if group.finish != nil && group.finish.DurationMS > 0 {
		b.WriteString("Ciwi phase duration: " + formatDurationMS(group.finish.DurationMS) + "\n")
	}
	if group.finish != nil && strings.TrimSpace(group.finish.Error) != "" {
		b.WriteString("Error: " + stripANSIAndControls(group.finish.Error) + "\n")
	}
	b.WriteString("\nDetails:\n")
	b.WriteString(stripANSIAndControls(group.phase.Description))
	if !strings.HasSuffix(group.phase.Description, "\n") {
		b.WriteByte('\n')
	}
	b.WriteString("\nOutput:\n'''\n")
	b.WriteString(stripANSIAndControls(group.output.String()))
	if !strings.HasSuffix(group.output.String(), "\n") {
		b.WriteByte('\n')
	}
	b.WriteString("'''\n\n")
	if group.finish == nil {
		b.WriteString("Ciwi phase finished: not reported\n\n")
	} else if strings.TrimSpace(group.finish.Error) != "" || group.finish.ExitCode != nil {
		b.WriteString("Ciwi phase finished: failed\n\n")
	} else {
		b.WriteString("Ciwi phase finished: succeeded\n\n")
	}
}

func phaseEventTitle(phase *protocol.JobExecutionPhase) string {
	if phase == nil {
		return "Ciwi phase"
	}
	if phase.Index > 0 && phase.Total > 0 {
		return fmt.Sprintf("Ciwi phase %d/%d: %s", phase.Index, phase.Total, phase.Name)
	}
	return strings.TrimSpace(phase.Name)
}

func stepEventTitle(step *protocol.JobStepPlanItem) string {
	if step == nil {
		return "Job step"
	}
	name := strings.TrimSpace(step.Name)
	name = strings.Join(strings.Fields(name), " ")
	if name != "" {
		name = strings.ReplaceAll(name, "_", " ")
	}
	if step.Total > 0 && step.Index > 0 {
		if name == "" {
			return fmt.Sprintf("Job step %d/%d", step.Index, step.Total)
		}
		return fmt.Sprintf("Job step %d/%d: %s", step.Index, step.Total, name)
	}
	if step.Index > 0 {
		if name == "" {
			return fmt.Sprintf("Job step %d", step.Index)
		}
		return fmt.Sprintf("Job step %d: %s", step.Index, name)
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
