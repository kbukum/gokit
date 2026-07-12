package framing_test

import (
	"bytes"
	"errors"
	"io"
	"reflect"
	"testing"

	"github.com/kbukum/gokit/codec"
	"github.com/kbukum/gokit/codec/framing"
	apperrors "github.com/kbukum/gokit/errors"
)

func TestValueRoundTripsAsOneFrame(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	sent := []string{"a", "b"}
	if err := framing.WriteValue(&buf, codec.CompactJSON(), sent, framing.DefaultMaxFrameBytes); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := framing.ReadValue[[]string](&buf, codec.CompactJSON(), framing.DefaultMaxFrameBytes)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !reflect.DeepEqual(got, sent) {
		t.Fatalf("got %#v want %#v", got, sent)
	}
}

func TestReadValueCleanEOFIsEOF(t *testing.T) {
	t.Parallel()
	_, err := framing.ReadValue[string](bytes.NewReader(nil), codec.CompactJSON(), framing.DefaultMaxFrameBytes)
	if !errors.Is(err, io.EOF) {
		t.Fatalf("expected io.EOF, got %v", err)
	}
}

func TestNonUTF8PayloadIsInvalidInput(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	if err := framing.WriteFrame(&buf, []byte{0xff, 0xfe}, framing.DefaultMaxFrameBytes); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, err := framing.ReadValue[string](&buf, codec.CompactJSON(), framing.DefaultMaxFrameBytes)
	var appErr *apperrors.AppError
	if !errors.As(err, &appErr) || appErr.Code != apperrors.ErrCodeInvalidInput {
		t.Fatalf("expected InvalidInput, got %v", err)
	}
}

func TestDecodeValueReusesReadBytes(t *testing.T) {
	t.Parallel()
	payload := []byte(`{"name":"svc","retries":3}`)
	got, err := framing.DecodeValue[settings](codec.CompactJSON(), payload)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got != (settings{Name: "svc", Retries: 3}) {
		t.Fatalf("got %+v", got)
	}
}

type settings struct {
	Name    string `json:"name"`
	Retries uint32 `json:"retries"`
}
