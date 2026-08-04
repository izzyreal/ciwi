package jobhistory

import "github.com/izzyreal/ciwi/internal/protocol"

// SummaryCards exposes the established execution grouping rules to
// transport-neutral presentation adapters without involving HTTP handlers.
func SummaryCards(jobs []protocol.JobExecution, active bool, limit int) []CardView {
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
	out := make([]CardView, 0, len(cards))
	for _, card := range cards {
		// Active cards render only their currently active rows, but their
		// progress model must include the latest completed attempts as well.
		out = append(out, cardView(jobs, card, true, true))
	}
	return out
}
