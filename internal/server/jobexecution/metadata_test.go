package jobexecution

import (
	"testing"

	"github.com/izzyreal/ciwi/internal/protocol"
)

func TestMetadataPatchFromEvents(t *testing.T) {
	meta := metadataPatchFromEvents([]protocol.JobExecutionEvent{
		{
			Type: protocol.JobExecutionEventTypeMetadataPatch,
			Metadata: map[string]string{
				"build_target":  "linux-amd64",
				"build_version": "v1.2.3",
			},
		},
	})
	if meta["build_target"] != "linux-amd64" {
		t.Fatalf("unexpected build_target: %q", meta["build_target"])
	}
	if meta["build_version"] != "v1.2.3" {
		t.Fatalf("unexpected build_version: %q", meta["build_version"])
	}
}
