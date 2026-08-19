//go:build !darwin

package fs

import (
	"os"
	"time"
)

// createdTime returns the zero time on platforms where Go does not portably expose a
// file creation timestamp. Callers treat a zero value as "unknown".
func createdTime(_ os.FileInfo) time.Time {
	return time.Time{}
}
