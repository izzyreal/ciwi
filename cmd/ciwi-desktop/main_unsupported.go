//go:build !darwin && !ios && !linux && !windows

package main

import (
	"fmt"
	"os"
)

func main() {
	_, _ = fmt.Fprintln(os.Stderr, "ciwi-desktop currently supports iOS, macOS, Windows, and Linux; the CNP client library is platform-independent")
	os.Exit(1)
}
