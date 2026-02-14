package agent

import (
	"os"
	"time"
)

func restartAgentProcess() {
	go func() {
		time.Sleep(250 * time.Millisecond)
		os.Exit(0)
	}()
}
