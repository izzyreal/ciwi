//go:build darwin || linux || windows

package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"gioui.org/app"
	gioadapter "github.com/izzyreal/ciwi/internal/adapters/gio"
	"github.com/izzyreal/ciwi/internal/version"
)

func main() {
	address := flag.String("addr", strings.TrimSpace(os.Getenv("CIWI_NATIVE_SERVER")), "ciwi CNP address (host:port); discovers with mDNS when omitted")
	theme := flag.String("theme", envOrDefault("CIWI_NATIVE_THEME", ""), "native UI theme (defaults to the saved preference)")
	flag.Parse()
	done := make(chan error, 1)
	go func() {
		done <- gioadapter.Run(gioadapter.Options{Address: *address, Theme: *theme, Version: version.Current()})
	}()
	app.Main()
	if err := <-done; err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func envOrDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
