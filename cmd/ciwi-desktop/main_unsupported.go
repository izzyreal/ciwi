//go:build !darwin && !linux && !windows

package main

import (
	"fmt"
	"os"
)

func main() {
	_, _ = fmt.Fprintln(os.Stderr, "ciwi-desktop currently supports macOS, Windows, and Linux; the CNP client library is platform-independent")
	os.Exit(1)
}
