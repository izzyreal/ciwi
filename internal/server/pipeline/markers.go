package pipeline

import (
	"fmt"
	"strings"
)

func SanitizeMarkerToken(v string) string {
	v = strings.TrimSpace(v)
	v = strings.ReplaceAll(v, " ", "_")
	v = strings.ReplaceAll(v, "\t", "_")
	v = strings.ReplaceAll(v, "\"", "")
	return v
}

func BuildStepMarkerCommand(shell string, index, total int, label string, meta map[string]string) string {
	label = SanitizeMarkerToken(label)
	if label == "" {
		label = fmt.Sprintf("step_%d", index)
	}
	payload := fmt.Sprintf("__CIWI_STEP_BEGIN__ index=%d total=%d name=%s", index, total, label)
	if len(meta) > 0 {
		if v := SanitizeMarkerToken(meta["kind"]); v != "" {
			payload += " kind=" + v
		}
		if v := SanitizeMarkerToken(meta["test_name"]); v != "" {
			payload += " test_name=" + v
		}
		if v := SanitizeMarkerToken(meta["test_format"]); v != "" {
			payload += " test_format=" + v
		}
		if v := SanitizeMarkerToken(meta["test_report"]); v != "" {
			payload += " test_report=" + v
		}
	}
	switch strings.ToLower(strings.TrimSpace(shell)) {
	case "cmd":
		return "echo " + payload
	case "powershell":
		return `Write-Output "` + payload + `"`
	default:
		return `echo "` + payload + `"`
	}
}

func BuildTestStepBlock(shell, command string) string {
	switch strings.ToLower(strings.TrimSpace(shell)) {
	case "cmd":
		return command
	case "powershell":
		return command
	default:
		return command
	}
}
