//go:build darwin || ios || linux || windows

package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"

	"gioui.org/app"
	gioadapter "github.com/izzyreal/ciwi/internal/adapters/gio"
	"github.com/izzyreal/ciwi/internal/version"
)

func main() {
	address := flag.String("addr", strings.TrimSpace(os.Getenv("CIWI_NATIVE_SERVER")), "ciwi CNP endpoint ([quic|tcp]://host:port); discovers with mDNS when omitted")
	theme := flag.String("theme", envOrDefault("CIWI_NATIVE_THEME", ""), "native UI theme (defaults to the saved preference)")
	route := flag.String("route", envOrDefault("CIWI_NATIVE_ROUTE", ""), "initial native UI route, for example /projects/1 or /settings")
	flag.Parse()
	done := make(chan error, 1)
	go func() {
		err := gioadapter.Run(gioadapter.Options{Address: *address, Theme: *theme, Version: version.Current(), Route: *route})
		if finishDarwinRun(runtime.GOOS, err, os.Stderr, os.Exit) {
			return
		}
		done <- err
	}()
	app.Main()
	if err := <-done; err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func finishDarwinRun(goos string, err error, stderr io.Writer, exit func(int)) bool {
	if goos != "darwin" {
		return false
	}
	code := 0
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		code = 1
	}
	exit(code)
	return true
}

func envOrDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
