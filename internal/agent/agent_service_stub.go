//go:build !windows

package agent

import "context"

func runAsWindowsServiceIfNeeded(_ func(context.Context) error) (bool, error) {
	return false, nil
}

func windowsServiceInfo() (bool, string) {
	return false, ""
}
