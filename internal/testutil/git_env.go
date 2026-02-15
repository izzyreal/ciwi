package testutil

import "os"

// SetGitEnvHardening applies deterministic git env for tests and returns a restore func.
func SetGitEnvHardening() func() {
	restores := []func(){
		setEnv("GIT_CONFIG_NOSYSTEM", "1"),
		setEnv("GIT_CONFIG_GLOBAL", "/dev/null"),
		setEnv("GIT_TERMINAL_PROMPT", "0"),
		setEnv("GIT_ASKPASS", "/usr/bin/false"),
	}
	return func() {
		for i := len(restores) - 1; i >= 0; i-- {
			restores[i]()
		}
	}
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
