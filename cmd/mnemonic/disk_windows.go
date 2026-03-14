//go:build windows

package main

import (
	"fmt"

	"golang.org/x/sys/windows"
)

// diskAvailable returns the number of free bytes on the volume containing path.
func diskAvailable(path string) (uint64, error) {
	pathPtr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, fmt.Errorf("converting path to UTF-16: %w", err)
	}
	var freeBytesAvailable uint64
	if err := windows.GetDiskFreeSpaceEx(pathPtr, &freeBytesAvailable, nil, nil); err != nil {
		return 0, fmt.Errorf("getting disk free space: %w", err)
	}
	return freeBytesAvailable, nil
}
