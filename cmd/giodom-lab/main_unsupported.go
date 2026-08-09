//go:build !darwin && !ios && !linux && !windows

package main

import "fmt"

func main() {
	fmt.Println("giodom-lab supports iOS, macOS, Linux, and Windows")
}
