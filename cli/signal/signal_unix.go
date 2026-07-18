//go:build !windows

package signal

import (
	"os"
	"syscall"
)

// terminationSignals adds SIGTERM,
// the conventional graceful-shutdown signal on Unix-like platforms.
func terminationSignals() []os.Signal {
	return []os.Signal{syscall.SIGTERM}
}
