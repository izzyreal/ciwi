package server

import serverupdate "github.com/izzyreal/ciwi/internal/server/update"

func startUpdateHelper(helperPath, targetPath, newBinaryPath string, parentPID int, restartArgs []string) error {
	return serverupdate.StartUpdateHelper(helperPath, targetPath, newBinaryPath, parentPID, restartArgs)
}
