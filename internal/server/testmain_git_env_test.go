package server

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	restores := []func(){
		setEnv("GIT_CONFIG_NOSYSTEM", "1"),
		setEnv("GIT_CONFIG_GLOBAL", "/dev/null"),
		setEnv("GIT_TERMINAL_PROMPT", "0"),
		setEnv("GIT_ASKPASS", "/usr/bin/false"),
	}
	code := m.Run()
	for i := len(restores) - 1; i >= 0; i-- {
		restores[i]()
	}
	os.Exit(code)
}

func setEnv(key, value string) func() {
	prev, had := os.LookupEnv(key)
	_ = os.Setenv(key, value)
	return func() {
		if had {
			_ = os.Setenv(key, prev)
			return
		}
		_ = os.Unsetenv(key)
	}
}
