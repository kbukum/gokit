//go:build windows

package signal

import "os"

// terminationSignals adds nothing on Windows: the OS has no SIGTERM analog,
// so [os.Interrupt] (Ctrl+C / Ctrl+Break) is the only signal Go delivers.
func terminationSignals() []os.Signal {
	return nil
}
