package server

import (
	"strings"
	"time"
)

const maxAgentLogLines = 30

func appendAgentLog(lines []string, msg string) []string {
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return lines
	}
	ts := time.Now().Local().Format("Mon 02 Jan 15:04:05")
	lines = append(lines, ts+" "+msg)
	if len(lines) > maxAgentLogLines {
		lines = append([]string(nil), lines[len(lines)-maxAgentLogLines:]...)
	}
	return lines
}
