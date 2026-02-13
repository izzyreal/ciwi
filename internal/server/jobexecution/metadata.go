package jobexecution

import (
	"strings"

	"github.com/izzyreal/ciwi/internal/protocol"
)

func metadataPatchFromEvents(events []protocol.JobExecutionEvent) map[string]string {
	out := map[string]string{}
	for _, event := range events {
		if strings.TrimSpace(event.Type) != protocol.JobExecutionEventTypeMetadataPatch {
			continue
		}
		for key, val := range event.Metadata {
			k := strings.TrimSpace(key)
			if k == "" {
				continue
			}
			out[k] = strings.TrimSpace(val)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
