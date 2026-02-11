package server

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
)

func startUpdateHelper(helperPath, targetPath, newBinaryPath string, parentPID int, restartArgs []string) error {
	args := []string{
		"update-helper",
		"--target", targetPath,
		"--new", newBinaryPath,
		"--pid", strconv.Itoa(parentPID),
	}
	for _, a := range restartArgs {
		args = append(args, "--arg", a)
	}

	cmd := exec.Command(helperPath, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = nil
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start helper: %w", err)
	}
	return nil
}
