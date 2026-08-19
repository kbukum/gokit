//go:build darwin

package fs

import (
	"os"
	"syscall"
	"time"
)

// createdTime returns the file creation time from the Darwin birthtime field, or
// the zero time when the underlying stat data is unavailable.
func createdTime(info os.FileInfo) time.Time {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return time.Time{}
	}
	birth := stat.Birthtimespec
	return time.Unix(birth.Sec, birth.Nsec)
}
