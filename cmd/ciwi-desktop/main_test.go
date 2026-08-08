//go:build darwin || linux || windows

package main

import (
	"bytes"
	"errors"
	"testing"
)

func TestFinishDarwinRunExitsAfterNormalWindowClose(t *testing.T) {
	var stderr bytes.Buffer
	exitCode := -1
	if !finishDarwinRun("darwin", nil, &stderr, func(code int) { exitCode = code }) {
		t.Fatal("normal macOS completion did not request process exit")
	}
	if exitCode != 0 || stderr.Len() != 0 {
		t.Fatalf("normal macOS completion = exit %d, stderr %q", exitCode, stderr.String())
	}
}

func TestFinishDarwinRunReportsErrors(t *testing.T) {
	var stderr bytes.Buffer
	exitCode := -1
	err := errors.New("window failed")
	if !finishDarwinRun("darwin", err, &stderr, func(code int) { exitCode = code }) {
		t.Fatal("failed macOS completion did not request process exit")
	}
	if exitCode != 1 || stderr.String() != "window failed\n" {
		t.Fatalf("failed macOS completion = exit %d, stderr %q", exitCode, stderr.String())
	}
}

func TestFinishDarwinRunLeavesOtherPlatformsToAppMain(t *testing.T) {
	for _, goos := range []string{"ios", "linux", "windows"} {
		t.Run(goos, func(t *testing.T) {
			exited := false
			if finishDarwinRun(goos, errors.New("ignored"), &bytes.Buffer{}, func(int) { exited = true }) {
				t.Fatalf("%s unexpectedly selected the forced-exit path", goos)
			}
			if exited {
				t.Fatalf("%s unexpectedly requested process exit", goos)
			}
		})
	}
}
