//go:build !unix

package fs

import "os"

func openReadFile(path string) (*os.File, error) {
	return os.Open(path)
}
