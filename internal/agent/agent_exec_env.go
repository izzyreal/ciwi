package agent

import (
	"errors"
	"os"
	"os/exec"
	"strings"
)

func withGoVerbose(env []string, enabled bool) []string {
	if !enabled {
		return env
	}
	for _, e := range env {
		if strings.HasPrefix(e, "GOFLAGS=") {
			return env
		}
	}
	return append(env, "GOFLAGS=-v")
}

func mergeEnv(base []string, extra map[string]string) []string {
	if len(extra) == 0 {
		return base
	}
	out := append([]string(nil), base...)
	index := map[string]int{}
	for i, e := range out {
		if eq := strings.IndexByte(e, '='); eq > 0 {
			index[e[:eq]] = i
		}
	}
	for k, v := range extra {
		entry := k + "=" + v
		if pos, ok := index[k]; ok {
			out[pos] = entry
		} else {
			out = append(out, entry)
		}
	}
	return out
}

func redactSensitive(in string, sensitive []string) string {
	out := in
	for _, secret := range sensitive {
		if strings.TrimSpace(secret) == "" {
			continue
		}
		out = strings.ReplaceAll(out, secret, "***")
	}
	return out
}

func boolEnv(key string, defaultValue bool) bool {
	v, ok := os.LookupEnv(key)
	if !ok {
		return defaultValue
	}
	v = strings.ToLower(strings.TrimSpace(v))
	switch v {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return defaultValue
	}
}

func trimOutput(output string) string {
	if len(output) <= maxReportedOutputBytes {
		return output
	}
	return output[len(output)-maxReportedOutputBytes:]
}

func exitCodeFromErr(err error) *int {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		code := exitErr.ExitCode()
		return &code
	}
	return nil
}

func defaultAgentID() string {
	hostname, err := os.Hostname()
	if err != nil || hostname == "" {
		return "agent-unknown"
	}
	return "agent-" + hostname
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
