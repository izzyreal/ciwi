//go:build !darwin

package main

import (
	"fmt"
	"os"
)

func main() {
	_, _ = fmt.Fprintln(os.Stderr, "ciwi-desktop currently supports macOS; the CNP client library is platform-independent")
	os.Exit(1)
}
