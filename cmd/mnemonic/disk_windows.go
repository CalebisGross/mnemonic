//go:build windows

package main

import "fmt"

// diskAvailable is not implemented on Windows.
func diskAvailable(_ string) (uint64, error) {
	return 0, fmt.Errorf("disk check not supported on Windows")
}
