//go:build (darwin && !ios) || linux || windows

package gio

import (
	"path/filepath"
	"runtime"
	"testing"
)

func TestRevealDownloadedFileCommandTargetsPlatformFileManager(t *testing.T) {
	path := filepath.Join(t.TempDir(), "artifact.zip")
	command, err := revealDownloadedFileCommand(path)
	if err != nil {
		t.Fatal(err)
	}
	switch runtime.GOOS {
	case "darwin":
		if filepath.Base(command.Path) != "open" || len(command.Args) != 3 || command.Args[1] != "-R" || command.Args[2] != path {
			t.Fatalf("Finder command = %#v", command.Args)
		}
	case "windows":
		if filepath.Base(command.Path) != "explorer.exe" || len(command.Args) != 3 || command.Args[1] != "/select," || command.Args[2] != path {
			t.Fatalf("Explorer command = %#v", command.Args)
		}
	case "linux":
		if filepath.Base(command.Path) != "xdg-open" || len(command.Args) != 2 || command.Args[1] != filepath.Dir(path) {
			t.Fatalf("file-manager command = %#v", command.Args)
		}
	}
}

func TestRevealDownloadedFileCommandRejectsRelativePath(t *testing.T) {
	if _, err := revealDownloadedFileCommand("artifact.zip"); err == nil {
		t.Fatal("relative reveal path was accepted")
	}
}
