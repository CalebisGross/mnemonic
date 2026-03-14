//go:build windows

package main

import "os"

// shutdownSignals returns the OS signals that should trigger graceful shutdown.
// On Windows, only os.Interrupt (Ctrl+C) is supported; SIGTERM does not exist.
func shutdownSignals() []os.Signal {
	return []os.Signal{os.Interrupt}
}
