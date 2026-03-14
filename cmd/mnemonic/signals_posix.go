//go:build !windows

package main

import (
	"os"
	"syscall"
)

// shutdownSignals returns the OS signals that should trigger graceful shutdown.
// On Unix, this includes SIGTERM (sent by systemd on "systemctl stop").
func shutdownSignals() []os.Signal {
	return []os.Signal{os.Interrupt, syscall.SIGTERM}
}
