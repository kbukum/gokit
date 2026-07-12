package framing

import (
	"io"
	"unicode/utf8"

	"github.com/kbukum/gokit/codec"
	apperrors "github.com/kbukum/gokit/errors"
)

// WriteValue encodes value with codec and writes it as one length-delimited
// frame.
//
// It returns a typed error (cause preserved) if encoding or the frame write
// fails, or the payload exceeds maxBytes.
func WriteValue[T any](w io.Writer, c codec.Codec, value T, maxBytes int) error {
	text, err := codec.Encode(c, value)
	if err != nil {
		return err
	}
	return WriteFrame(w, []byte(text), maxBytes)
}

// ReadValue reads one frame and decodes it into T through codec.
//
// It returns io.EOF on a clean end-of-stream between frames, or a typed error
// (cause preserved) on a transport failure or a payload that does not decode
// into T.
func ReadValue[T any](r io.Reader, c codec.Codec, maxBytes int) (T, error) {
	var out T
	payload, err := ReadFrame(r, maxBytes)
	if err != nil {
		return out, err
	}
	return DecodeValue[T](c, payload)
}

// DecodeValue decodes an already-read frame payload into T through codec.
//
// It is split from [ReadValue] so a caller that must inspect one frame as more
// than one shape can decode the same bytes without re-reading the stream. It
// returns a typed error (cause preserved) if payload is not valid UTF-8 or does
// not decode into T.
func DecodeValue[T any](c codec.Codec, payload []byte) (T, error) {
	var out T
	if !utf8.Valid(payload) {
		return out, apperrors.InvalidInput("frame", "frame payload is not valid UTF-8")
	}
	return codec.Decode[T](c, string(payload))
}
