package presentation

import (
	"strings"
	"time"

	"github.com/izzyreal/ciwi/internal/domain"
)

type progressInput struct {
	status             string
	waiting            bool
	started            time.Time
	finished           time.Time
	expectedDurationMS int64
}

func progressForInput(input progressInput, now time.Time) domain.Progress {
	snapshot := now.UTC().UnixMilli()
	status := normalizedProgressStatus(input.status)
	if terminalProgressStatus(status) {
		return domain.Progress{State: domain.ProgressComplete, Fraction: 1, SnapshotUnixMS: snapshot}
	}
	if input.waiting {
		return domain.Progress{State: domain.ProgressWaiting, SnapshotUnixMS: snapshot}
	}
	if !activeProgressStatus(status) {
		return domain.Progress{State: domain.ProgressNone, SnapshotUnixMS: snapshot}
	}
	if input.expectedDurationMS <= 0 {
		return domain.Progress{State: domain.ProgressIndeterminate, SnapshotUnixMS: snapshot}
	}
	if input.started.IsZero() {
		return domain.Progress{State: domain.ProgressDeterminate, SnapshotUnixMS: snapshot}
	}
	elapsed := max(now.Sub(input.started).Milliseconds(), 0)
	ratio := float64(elapsed) / float64(input.expectedDurationMS)
	if ratio >= 1 {
		return domain.Progress{State: domain.ProgressOverrun, Fraction: 1, SnapshotUnixMS: snapshot}
	}
	return domain.Progress{
		State: domain.ProgressDeterminate, Fraction: clampProgress(ratio), SnapshotUnixMS: snapshot,
		RatePerMS: 1 / float64(input.expectedDurationMS),
	}
}

func aggregateCardProgress(jobs []domain.ExecutionCardJob, now time.Time) domain.Progress {
	inputs := make([]progressInput, 0, len(jobs))
	for _, job := range jobs {
		inputs = append(inputs, progressInput{
			status: job.Status, waiting: job.Waiting, started: job.StartedUTC, finished: job.FinishedUTC,
			expectedDurationMS: job.ExpectedDurationMS,
		})
	}
	return aggregateProgress(inputs, now)
}

func aggregateProgress(inputs []progressInput, now time.Time) domain.Progress {
	snapshot := now.UTC().UnixMilli()
	if len(inputs) == 0 {
		return domain.Progress{State: domain.ProgressNone, SnapshotUnixMS: snapshot}
	}
	var totalWeight, completedWeight, weightedRate float64
	active, waiting, waitingWithoutEstimate, overrun := false, false, false, false
	for _, input := range inputs {
		model := progressForInput(input, now)
		weight := float64(input.expectedDurationMS)
		if weight <= 0 && terminalProgressStatus(normalizedProgressStatus(input.status)) &&
			!input.started.IsZero() && !input.finished.IsZero() && input.finished.After(input.started) {
			weight = float64(input.finished.Sub(input.started).Milliseconds())
		}
		if model.State == domain.ProgressWaiting {
			waiting = true
			if weight > 0 {
				totalWeight += weight
			} else {
				waitingWithoutEstimate = true
			}
			continue
		}
		isActive := activeProgressStatus(normalizedProgressStatus(input.status))
		active = active || isActive
		if model.State == domain.ProgressIndeterminate || model.State == domain.ProgressNone || weight <= 0 {
			if isActive {
				return domain.Progress{State: domain.ProgressIndeterminate, SnapshotUnixMS: snapshot}
			}
			continue
		}
		totalWeight += weight
		completedWeight += weight * model.Fraction
		weightedRate += weight * model.RatePerMS
		overrun = overrun || model.State == domain.ProgressOverrun
	}
	if !active {
		if waiting {
			return domain.Progress{State: domain.ProgressNone, SnapshotUnixMS: snapshot}
		}
		return domain.Progress{State: domain.ProgressComplete, Fraction: 1, SnapshotUnixMS: snapshot}
	}
	if waitingWithoutEstimate || totalWeight <= 0 {
		return domain.Progress{State: domain.ProgressIndeterminate, SnapshotUnixMS: snapshot}
	}
	fraction := clampProgress(completedWeight / totalWeight)
	state := domain.ProgressDeterminate
	if overrun && fraction >= .999 {
		state = domain.ProgressOverrun
		weightedRate = 0
	}
	return domain.Progress{
		State: state, Fraction: fraction, SnapshotUnixMS: snapshot,
		RatePerMS: weightedRate / totalWeight,
	}
}

func presentFrontPageProgress(cards []domain.ExecutionCard, now time.Time) {
	for cardIndex := range cards {
		allJobs := make([]domain.ExecutionCardJob, 0)
		for sectionIndex := range cards[cardIndex].Sections {
			section := &cards[cardIndex].Sections[sectionIndex]
			for jobIndex := range section.Jobs {
				job := &section.Jobs[jobIndex]
				job.Progress = progressForInput(progressInput{
					status: job.Status, waiting: job.Waiting, started: job.StartedUTC,
					finished: job.FinishedUTC, expectedDurationMS: job.ExpectedDurationMS,
				}, now)
			}
			section.Progress = aggregateCardProgress(section.Jobs, now)
			allJobs = append(allJobs, section.Jobs...)
		}
		cards[cardIndex].Progress = aggregateCardProgress(allJobs, now)
	}
}

func normalizedProgressStatus(status string) string {
	return strings.ToLower(strings.TrimSpace(status))
}

func activeProgressStatus(status string) bool {
	switch status {
	case "queued", "running", "leased", "in progress", "active":
		return true
	default:
		return false
	}
}

func terminalProgressStatus(status string) bool {
	switch status {
	case "succeeded", "failed", "cancelled", "canceled", "skipped":
		return true
	default:
		return false
	}
}

func clampProgress(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}
