// Package logtext owns the renderer-neutral clean-text representation used by
// interactive job logs and clean log downloads.
package logtext

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

const (
	ChunkBytes     = 64 * 1024
	PageBytes      = 256 * 1024
	SearchMinRunes = 3
	SearchMaxRunes = 256
	SearchOverlap  = SearchMaxRunes - 1
)

var ansiEscapeRE = regexp.MustCompile(`\x1b\[[0-?]*[ -/]*[@-~]|\x1b\][^\x07]*(?:\x07|\x1b\\)|\x1b[@-Z\\-_]`)

// Clean normalizes line endings, strips terminal escape sequences and drops
// invalid UTF-8 and control characters that cannot be displayed safely.
func Clean(text string) string {
	text = strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n")
	text = ansiEscapeRE.ReplaceAllString(text, "")
	var clean strings.Builder
	clean.Grow(len(text))
	for len(text) > 0 {
		r, size := utf8.DecodeRuneInString(text)
		text = text[size:]
		if r == utf8.RuneError && size == 1 {
			continue
		}
		if r == '\n' || r == '\t' || r >= 0x20 {
			clean.WriteRune(r)
		}
	}
	return clean.String()
}

// Split returns valid UTF-8 chunks no larger than maxBytes.
func Split(text string, maxBytes int) []string {
	if text == "" {
		return nil
	}
	if maxBytes <= 0 {
		maxBytes = ChunkBytes
	}
	chunks := make([]string, 0, len(text)/maxBytes+1)
	for len(text) > maxBytes {
		cut := maxBytes
		for cut > 0 && !utf8.RuneStart(text[cut]) {
			cut--
		}
		if cut == 0 {
			_, cut = utf8.DecodeRuneInString(text)
		}
		chunks = append(chunks, text[:cut])
		text = text[cut:]
	}
	if text != "" {
		chunks = append(chunks, text)
	}
	return chunks
}

// TailRunes returns the final count runes without splitting UTF-8.
func TailRunes(text string, count int) string {
	if count <= 0 || text == "" {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= count {
		return text
	}
	return string(runes[len(runes)-count:])
}
