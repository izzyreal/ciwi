package agent

import (
	"fmt"
	"time"

	"github.com/izzyreal/ciwi/internal/protocol"
)

func executionPhase(timeline []protocol.JobExecutionTimelineItem, id string) protocol.JobExecutionPhase {
	phase, _ := protocol.TimelinePhase(timeline, id)
	return phase
}

func executionPhaseTitle(phase protocol.JobExecutionPhase) string {
	return fmt.Sprintf("Step %d/%d: %s", phase.Index, phase.Total, phase.Name)
}

func phaseStartedEvent(phase protocol.JobExecutionPhase, started time.Time) protocol.JobExecutionEvent {
	return protocol.JobExecutionEvent{Type: protocol.JobExecutionEventTypePhaseStarted, Phase: &phase, TimestampUTC: started}
}

func phaseOutputEvent(phase protocol.JobExecutionPhase, output string) []protocol.JobExecutionEvent {
	if output == "" {
		return nil
	}
	return []protocol.JobExecutionEvent{{Type: protocol.JobExecutionEventTypePhaseOutput, Phase: &phase, Output: output, TimestampUTC: time.Now().UTC()}}
}

func phaseFinishedEvent(phase protocol.JobExecutionPhase, started time.Time, phaseErr error) protocol.JobExecutionEvent {
	event := protocol.JobExecutionEvent{
		Type: protocol.JobExecutionEventTypePhaseFinished, Phase: &phase,
		DurationMS: time.Since(started).Milliseconds(), TimestampUTC: time.Now().UTC(),
	}
	if phaseErr != nil {
		event.Error = phaseErr.Error()
		if code := exitCodeFromErr(phaseErr); code != nil {
			event.ExitCode = code
		}
	}
	return event
}
