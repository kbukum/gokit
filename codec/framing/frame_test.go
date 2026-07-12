package framing_test

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"github.com/kbukum/gokit/codec/framing"
	apperrors "github.com/kbukum/gokit/errors"
)

func TestRoundTripsAFrame(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	if err := framing.WriteFrame(&buf, []byte("hello"), framing.DefaultMaxFrameBytes); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := framing.ReadFrame(&buf, framing.DefaultMaxFrameBytes)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != "hello" {
		t.Fatalf("got %q", got)
	}
}

func TestCleanEOFBetweenFramesIsEOF(t *testing.T) {
	t.Parallel()
	_, err := framing.ReadFrame(bytes.NewReader(nil), framing.DefaultMaxFrameBytes)
	if !errors.Is(err, io.EOF) {
		t.Fatalf("expected io.EOF, got %v", err)
	}
}

func TestTruncatedPrefixIsTransportError(t *testing.T) {
	t.Parallel()
	_, err := framing.ReadFrame(bytes.NewReader([]byte{0, 0}), framing.DefaultMaxFrameBytes)
	var appErr *apperrors.AppError
	if !errors.As(err, &appErr) || appErr.Code != apperrors.ErrCodeServiceUnavailable {
		t.Fatalf("expected ServiceUnavailable, got %v", err)
	}
}

func TestTruncatedPayloadIsTransportError(t *testing.T) {
	t.Parallel()
	// Length prefix says 5 bytes but only 2 follow.
	data := []byte{0, 0, 0, 5, 'h', 'i'}
	_, err := framing.ReadFrame(bytes.NewReader(data), framing.DefaultMaxFrameBytes)
	var appErr *apperrors.AppError
	if !errors.As(err, &appErr) || appErr.Code != apperrors.ErrCodeServiceUnavailable {
		t.Fatalf("expected ServiceUnavailable, got %v", err)
	}
}

func TestOversizedIncomingFrameIsRejected(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	if err := framing.WriteFrame(&buf, []byte{0, 0, 0, 0, 0, 0, 0, 0}, framing.DefaultMaxFrameBytes); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, err := framing.ReadFrame(&buf, 4)
	var appErr *apperrors.AppError
	if !errors.As(err, &appErr) || appErr.Code != apperrors.ErrCodeInvalidInput {
		t.Fatalf("expected InvalidInput, got %v", err)
	}
}

func TestOversizedOutgoingPayloadIsRejected(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	err := framing.WriteFrame(&buf, []byte("too big"), 3)
	var appErr *apperrors.AppError
	if !errors.As(err, &appErr) || appErr.Code != apperrors.ErrCodeInvalidInput {
		t.Fatalf("expected InvalidInput, got %v", err)
	}
}
