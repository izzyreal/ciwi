package logtext

import (
	"strings"
	"testing"
)

func TestSplitBoundsOneHundredMiBLog(t *testing.T) {
	const size = 100 * 1024 * 1024
	text := strings.Repeat("x", size)
	chunks := Split(text, ChunkBytes)
	if len(chunks) < 2 {
		t.Fatal("large log was not chunked")
	}
	total := 0
	for index, chunk := range chunks {
		if len(chunk) > ChunkBytes {
			t.Fatalf("chunk %d contains %d bytes, limit %d", index, len(chunk), ChunkBytes)
		}
		total += len(chunk)
	}
	if total != size {
		t.Fatalf("chunked bytes = %d, want %d", total, size)
	}
}

func TestCleanNormalizesTerminalText(t *testing.T) {
	got := Clean("one\r\n\x1b[31mtwo\x1b[0m\rthree\x00")
	if got != "one\ntwo\nthree" {
		t.Fatalf("Clean() = %q", got)
	}
}
