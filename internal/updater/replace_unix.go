//go:build !windows

package updater

import "os"

// replaceBinary atomically replaces the running binary with a new one.
// On Unix systems, os.Rename over a running binary works because the old
// process keeps the deleted inode open until it exits.
func replaceBinary(newBinaryPath, execPath string) error {
	return os.Rename(newBinaryPath, execPath)
}
