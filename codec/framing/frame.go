// Package framing provides bounded length-delimited framing for streaming structured-text payloads.
//
// A frame is a 4-byte big-endian unsigned length prefix followed by exactly that many payload bytes. Every read is bounded by an explicit maximum so a malformed or hostile peer can never make a reader allocate without limit. Use it to carry one codec-encoded value per frame over any blocking io.Reader/io.Writer transport (a pipe, a socket, a subprocess's stdio).
//
// [WriteValue] / [ReadValue] encode and decode typed values through an injected codec; [WriteFrame] / [ReadFrame] move raw payload bytes when the caller owns serialization.
package framing

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net/http"

	apperrors "github.com/kbukum/gokit/errors"
)

// DefaultMaxFrameBytes is the default maximum accepted payload size for a single frame (16 MiB). It is generous enough for large structured payloads yet bounded so a corrupt length prefix cannot trigger an unbounded allocation.
const DefaultMaxFrameBytes = 16 * 1024 * 1024

// lenPrefixBytes is the width of the big-endian length prefix on every payload.
const lenPrefixBytes = 4

// WriteFrame writes one length-delimited frame carrying payload. A plain io.Writer suffices; flushing any buffered transport is the caller's concern.
//
// It returns a typed error if payload exceeds maxBytes or the underlying writer fails (cause preserved).
func WriteFrame(w io.Writer, payload []byte, maxBytes int) error {
	if len(payload) > maxBytes {
		return apperrors.InvalidInput("frame", fmt.Sprintf(
			"payload of %d bytes exceeds the %d-byte frame limit", len(payload), maxBytes))
	}
	if uint64(len(payload)) > uint64(^uint32(0)) {
		return apperrors.InvalidInput("frame", "payload length exceeds uint32 range")
	}
	var prefix [lenPrefixBytes]byte
	binary.BigEndian.PutUint32(prefix[:], uint32(len(payload)))
	if _, err := w.Write(prefix[:]); err != nil {
		return transportError("write frame length", err)
	}
	if _, err := w.Write(payload); err != nil {
		return transportError("write frame payload", err)
	}
	return nil
}

// ReadFrame reads one length-delimited frame, bounded by maxBytes.
//
// It returns io.EOF (with a nil payload) on a clean end-of-stream observed before any length byte — the peer closed the connection between frames. A partial prefix or payload is a hard transport error, and a length above maxBytes is rejected before any allocation.
func ReadFrame(r io.Reader, maxBytes int) ([]byte, error) {
	var prefix [lenPrefixBytes]byte
	if err := readFull(r, prefix[:]); err != nil {
		return nil, err
	}
	length := int(binary.BigEndian.Uint32(prefix[:]))
	if length > maxBytes {
		return nil, apperrors.InvalidInput("frame", fmt.Sprintf(
			"incoming frame length %d exceeds the %d-byte limit", length, maxBytes))
	}
	payload := make([]byte, length)
	if _, err := io.ReadFull(r, payload); err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return nil, apperrors.New(apperrors.ErrCodeServiceUnavailable,
				"framed transport: stream ended mid-frame (truncated payload)",
				http.StatusServiceUnavailable)
		}
		return nil, transportError("read frame payload", err)
	}
	return payload, nil
}

// readFull fills buf exactly, returning io.EOF only for a clean leading EOF and a typed transport error for a truncated prefix.
func readFull(r io.Reader, buf []byte) error {
	_, err := io.ReadFull(r, buf)
	switch {
	case err == nil:
		return nil
	case errors.Is(err, io.EOF):
		return io.EOF
	case errors.Is(err, io.ErrUnexpectedEOF):
		return apperrors.New(apperrors.ErrCodeServiceUnavailable,
			"framed transport: stream ended mid-frame (truncated length prefix)",
			http.StatusServiceUnavailable)
	default:
		return transportError("read frame length", err)
	}
}

// transportError builds a typed transport error preserving the I/O cause.
func transportError(context string, err error) error {
	return apperrors.New(apperrors.ErrCodeServiceUnavailable,
		fmt.Sprintf("framed transport: %s", context),
		http.StatusServiceUnavailable).WithCause(err)
}
