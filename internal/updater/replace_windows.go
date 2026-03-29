//go:build windows

package updater

import (
	"fmt"
	"os"
	"path/filepath"
)

// oldBinarySuffix is the extension used when moving the locked running binary
// out of the way during a Windows update.
const oldBinarySuffix = ".old"

// replaceBinary replaces the running binary on Windows using a rename-dance.
// Windows locks running executables, preventing direct overwrite. However, a
// locked file CAN be renamed. So we:
//  1. Rename the running binary to <name>.old (move it out of the way)
//  2. Rename the new binary into the original path
//
// The .old file is cleaned up on next startup via CleanupOldBinary.
func replaceBinary(newBinaryPath, execPath string) error {
	oldPath := execPath + oldBinarySuffix

	// Remove any leftover .old file from a previous update
	_ = os.Remove(oldPath)

	// Step 1: Rename the running (locked) binary out of the way
	if err := os.Rename(execPath, oldPath); err != nil {
		return fmt.Errorf("moving running binary to %s: %w", filepath.Base(oldPath), err)
	}

	// Step 2: Rename the new binary into place
	if err := os.Rename(newBinaryPath, execPath); err != nil {
		// Try to restore the original binary
		_ = os.Rename(oldPath, execPath)
		return fmt.Errorf("moving new binary into place: %w", err)
	}

	return nil
}
