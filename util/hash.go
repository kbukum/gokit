package util

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"io"

	"github.com/zeebo/blake3"
)

// ContentHasher is an incremental content hasher producing a stable lowercase-hex
// digest, backed by BLAKE3. Feed bytes with Update (raw) or UpdateFramed
// (domain-separated), then read the digest with FinalizeHex. Finalizing does not
// consume the hasher, so it may be reused. BLAKE3 is the canonical content hash for
// cache keys, change detection, and deduplication; the digest identity matches the
// rskit and pykit content hashers.
type ContentHasher struct {
	inner *blake3.Hasher
}

// NewContentHasher creates an empty content hasher.
func NewContentHasher() *ContentHasher {
	return &ContentHasher{inner: blake3.New()}
}

// Update folds bytes into the digest verbatim, without framing, returning the hasher
// for chaining.
func (h *ContentHasher) Update(bytes []byte) *ContentHasher {
	_, _ = h.inner.Write(bytes)
	return h
}

// UpdateFramed folds a labeled value into the digest with unambiguous framing. Each
// of label and value is folded length-prefixed (its length as a little-endian uint64,
// then its bytes), so field boundaries stay unambiguous even when inputs contain
// arbitrary bytes; independently folded fields cannot alias one another. Returns the
// hasher for chaining.
func (h *ContentHasher) UpdateFramed(label, value []byte) *ContentHasher {
	h.updateLengthPrefixed(label)
	h.updateLengthPrefixed(value)
	return h
}

func (h *ContentHasher) updateLengthPrefixed(bytes []byte) {
	var length [8]byte
	binary.LittleEndian.PutUint64(length[:], uint64(len(bytes)))
	_, _ = h.inner.Write(length[:])
	_, _ = h.inner.Write(bytes)
}

// FinalizeHex renders the current digest as a 64-character lowercase hex string. It
// does not consume the hasher: further updates may follow and produce a new digest.
func (h *ContentHasher) FinalizeHex() string {
	sum := h.inner.Sum(nil)
	return hex.EncodeToString(sum)
}

// HashHex returns the lowercase-hex BLAKE3 content digest of a single byte slice.
func HashHex(bytes []byte) string {
	return NewContentHasher().Update(bytes).FinalizeHex()
}

// Sha256Hex returns the lowercase-hex SHA-256 digest of a byte slice. SHA-256 is for
// wire-format and interop use cases only; prefer HashHex (BLAKE3) for internal identity.
func Sha256Hex(bytes []byte) string {
	sum := sha256.Sum256(bytes)
	return hex.EncodeToString(sum[:])
}

// Sha256Reader computes the lowercase-hex SHA-256 digest of a stream, reading in
// bounded chunks so large inputs never need to be fully resident in memory.
func Sha256Reader(reader io.Reader) (string, error) {
	hasher := sha256.New()
	if _, err := io.Copy(hasher, reader); err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}
