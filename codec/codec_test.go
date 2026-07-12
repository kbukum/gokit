package codec_test

import (
	"strings"
	"testing"

	"github.com/kbukum/gokit/codec"
)

type settings struct {
	Name    string `json:"name"`
	Retries uint32 `json:"retries"`
}

func TestEncodeDecodeRoundTripsTypedValue(t *testing.T) {
	t.Parallel()
	for _, c := range []codec.Codec{codec.PrettyJSON(), codec.CompactJSON(), codec.NewTOMLCodec()} {
		want := settings{Name: "svc", Retries: 3}
		encoded, err := codec.Encode(c, want)
		if err != nil {
			t.Fatalf("%s encode: %v", c.Name(), err)
		}
		got, err := codec.Decode[settings](c, encoded)
		if err != nil {
			t.Fatalf("%s decode: %v", c.Name(), err)
		}
		if got != want {
			t.Fatalf("%s round trip: got %+v want %+v", c.Name(), got, want)
		}
	}
}

func TestEncodeReportsUnrepresentableValues(t *testing.T) {
	t.Parallel()
	_, err := codec.Encode(codec.PrettyJSON(), make(chan int))
	if err == nil || !strings.Contains(err.Error(), "value model") {
		t.Fatalf("expected value model error, got %v", err)
	}
}

func TestDecodeReportsTypeMismatch(t *testing.T) {
	t.Parallel()
	_, err := codec.Decode[settings](codec.PrettyJSON(), "[1, 2, 3]")
	if err == nil || !strings.Contains(err.Error(), "deserialize") {
		t.Fatalf("expected deserialize error, got %v", err)
	}
}

func TestDecodePropagatesParseError(t *testing.T) {
	t.Parallel()
	_, err := codec.Decode[settings](codec.PrettyJSON(), "{ not json")
	if err == nil || !strings.Contains(err.Error(), "parse") {
		t.Fatalf("expected parse error, got %v", err)
	}
}
