package jobhistory

import "github.com/izzyreal/ciwi/internal/protocol"

// SummarySelection freezes the established grouping, latest-attempt, and card
// limit decisions so callers can enrich only the visible jobs before rendering
// the final views.
type SummarySelection struct {
	cards []executionCard
}

func SelectSummaryCards(jobs []protocol.JobExecution, active bool, limit int) SummarySelection {
	latest := latestAttemptIDs(jobs)
	cards := buildExecutionCards(jobs, func(job protocol.JobExecution) bool {
		if _, ok := latest[job.ID]; !ok {
			return false
		}
		return protocol.IsActiveJobExecutionStatus(job.Status) == active
	})
	if limit > 0 && len(cards) > limit {
		cards = cards[:limit]
	}
	return SummarySelection{cards: cards}
}

func (s SummarySelection) VisibleJobIDs(jobs []protocol.JobExecution) []string {
	seen := make(map[string]struct{})
	out := make([]string, 0)
	for _, card := range s.cards {
		for _, index := range card.VisibleIndices {
			if index < 0 || index >= len(jobs) {
				continue
			}
			jobID := jobs[index].ID
			if _, exists := seen[jobID]; exists {
				continue
			}
			seen[jobID] = struct{}{}
			out = append(out, jobID)
		}
	}
	return out
}

func (s SummarySelection) Views(jobs []protocol.JobExecution) []CardView {
	out := make([]CardView, 0, len(s.cards))
	for _, card := range s.cards {
		// Active cards render only their currently active rows, but their
		// progress model must include the latest completed attempts as well.
		out = append(out, cardView(jobs, card, true, true))
	}
	return out
}

// SummaryCards exposes the established execution grouping rules to
// transport-neutral presentation adapters without involving HTTP handlers.
func SummaryCards(jobs []protocol.JobExecution, active bool, limit int) []CardView {
	return SelectSummaryCards(jobs, active, limit).Views(jobs)
}
