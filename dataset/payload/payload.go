package payload

import (
	"fmt"
	"io"
	"os"

	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/fs"
)

// Payload is a bounded byte payload that is either held in memory or backed by a file on disk.
// Payloads over the in-memory cap must be file-backed
// so a stage never materializes an oversized blob.
type Payload struct {
	data []byte
	path string
}

// FromBytes returns an in-memory payload, rejecting input larger than the configured in-memory cap
// so callers spill to a file instead.
func FromBytes(data []byte, limits Limits) (Payload, error) {
	limits = limits.WithDefaults()
	if int64(len(data)) > limits.MaxInMemoryBytes {
		return Payload{}, apperrors.InvalidInput("payload",
			fmt.Sprintf("payload of %d bytes exceeds in-memory cap of %d bytes", len(data), limits.MaxInMemoryBytes))
	}
	return Payload{data: data}, nil
}

// FromFile returns a file-backed payload referencing path.
// The file is not read until [Payload.ReadBounded] or [Payload.WriteTo] is called.
func FromFile(path string) Payload {
	return Payload{path: path}
}

// IsFile reports whether the payload is file-backed.
func (p Payload) IsFile() bool { return p.path != "" }

// ReadBounded returns the payload bytes, enforcing the in-memory cap.
// A file-backed payload is read through [fs.ReadFileLimit].
func (p Payload) ReadBounded(limits Limits) ([]byte, error) {
	limits = limits.WithDefaults()
	if p.IsFile() {
		return fs.ReadFileLimit(p.path, limits.MaxInMemoryBytes)
	}
	if int64(len(p.data)) > limits.MaxInMemoryBytes {
		return nil, apperrors.InvalidInput("payload",
			fmt.Sprintf("payload of %d bytes exceeds in-memory cap of %d bytes", len(p.data), limits.MaxInMemoryBytes))
	}
	return p.data, nil
}

// WriteTo streams the payload to w without materializing a file-backed payload in memory,
// returning the number of bytes written.
func (p Payload) WriteTo(w io.Writer) (int64, error) {
	if p.IsFile() {
		f, err := os.Open(p.path)
		if err != nil {
			return 0, apperrors.Internal(err)
		}
		defer func() { _ = f.Close() }()
		n, err := io.Copy(w, f)
		if err != nil {
			return n, apperrors.Internal(err)
		}
		return n, nil
	}
	n, err := w.Write(p.data)
	if err != nil {
		return int64(n), apperrors.Internal(err)
	}
	return int64(n), nil
}
