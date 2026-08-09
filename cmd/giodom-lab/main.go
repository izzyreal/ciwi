//go:build darwin || ios || linux || windows

package main

import (
	"fmt"
	"os"
	"runtime"

	"gioui.org/app"
	"github.com/izzyreal/ciwi/internal/giodom/lab"
)

func main() {
	done := make(chan error, 1)
	go func() {
		err := lab.Run()
		if runtime.GOOS == "darwin" {
			if err != nil {
				_, _ = fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			os.Exit(0)
		}
		done <- err
	}()
	app.Main()
	if err := <-done; err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
