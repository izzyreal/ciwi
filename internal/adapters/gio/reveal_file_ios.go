//go:build ios

package gio

import "fmt"

func revealDownloadedFile(string) error {
	return fmt.Errorf("revealing downloaded files is unavailable on iOS")
}
