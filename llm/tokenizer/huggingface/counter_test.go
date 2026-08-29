package huggingface

import (
	"bytes"
	"math"
	"os"
	"strings"
	"testing"

	"github.com/kbukum/gokit/llm"
)

const fixture = "testdata/tokenizer.json"

func TestFromFileRejectsMissingPath(t *testing.T) {
	t.Parallel()

	if _, err := FromFile("testdata/does-not-exist.json"); err == nil {
		t.Fatal("expected error for missing definition, got nil")
	}
}

func TestNewCounterRequiresPath(t *testing.T) {
	t.Parallel()

	if _, err := NewCounter(Config{}); err == nil {
		t.Fatal("expected error for empty Path, got nil")
	}
}

func TestFromReaderRejectsMalformedDefinition(t *testing.T) {
	t.Parallel()

	if _, err := FromReader(strings.NewReader("{ not a tokenizer"), 0); err == nil {
		t.Fatal("expected error for malformed definition, got nil")
	}
}

func TestFromReaderRejectsNilReader(t *testing.T) {
	t.Parallel()

	if _, err := FromReader(nil, 0); err == nil {
		t.Fatal("expected error for nil reader, got nil")
	}

	var typedNil *bytes.Reader
	if _, err := FromReader(typedNil, 0); err == nil {
		t.Fatal("expected error for typed-nil reader, got nil")
	}
}

func TestFromReaderContainsParserPanicOnMalformedModel(t *testing.T) {
	t.Parallel()

	// Syntactically valid JSON whose model fields have the wrong types. The
	// underlying parser type-asserts these without checking and panics; the
	// constructor must translate that into an error, never propagate the panic.
	cases := map[string]string{
		"vocab wrong type":    `{"model":{"type":"BPE","vocab":123,"merges":"nope"}}`,
		"vocab is array":      `{"model":{"type":"WordLevel","vocab":[]}}`,
		"vocab value not int": `{"model":{"type":"BPE","vocab":{"a":"notint"}}}`,
	}
	for name, def := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := FromReader(strings.NewReader(def), 0)
			if err == nil {
				t.Fatalf("expected error for %s, got nil", name)
			}
		})
	}
}

func TestFromReaderRejectsBPEDropout(t *testing.T) {
	t.Parallel()

	def := `{"model":{"type":"BPE","dropout":0.1}}`
	if _, err := FromReader(strings.NewReader(def), 0); err == nil {
		t.Fatal("expected error for BPE dropout, got nil")
	}
}

func TestFromReaderRejectsEmptyDefinition(t *testing.T) {
	t.Parallel()

	if _, err := FromReader(strings.NewReader(""), 0); err == nil {
		t.Fatal("expected error for empty definition, got nil")
	}
}

func TestFromReaderEnforcesByteBound(t *testing.T) {
	t.Parallel()

	def, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	if _, err := FromReader(bytes.NewReader(def), 8); err == nil {
		t.Fatal("expected error when definition exceeds bound, got nil")
	}
}

func TestFromReaderAcceptsMaxInt64Bound(t *testing.T) {
	t.Parallel()

	def, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	if _, err := FromReader(bytes.NewReader(def), math.MaxInt64); err != nil {
		t.Fatalf("FromReader with MaxInt64 bound: %v", err)
	}
}

func TestCounterName(t *testing.T) {
	t.Parallel()

	c, err := FromFile(fixture)
	if err != nil {
		t.Fatalf("FromFile: %v", err)
	}
	if !strings.HasPrefix(c.Name(), "huggingface:") {
		t.Errorf("Name() = %q, want huggingface: prefix", c.Name())
	}
	// The fingerprint is a fixed-width hex digest of the definition bytes.
	if got, want := len(c.Name()), len("huggingface:")+12; got != want {
		t.Errorf("Name() length = %d, want %d", got, want)
	}
}

func TestCounterNameIsStableForSameDefinition(t *testing.T) {
	t.Parallel()

	a, err := FromFile(fixture)
	if err != nil {
		t.Fatalf("FromFile: %v", err)
	}
	b, err := FromFile(fixture)
	if err != nil {
		t.Fatalf("FromFile: %v", err)
	}
	if a.Name() != b.Name() {
		t.Errorf("Name() not stable: %q != %q", a.Name(), b.Name())
	}
}

func TestCounterEmptyIsZero(t *testing.T) {
	t.Parallel()

	c, err := FromFile(fixture)
	if err != nil {
		t.Fatalf("FromFile: %v", err)
	}
	if got, err := c.Count(""); err != nil || got != 0 {
		t.Errorf("Count(\"\") = %d, %v, want 0, nil", got, err)
	}
}

func TestCounterCountsAndIsDeterministic(t *testing.T) {
	t.Parallel()

	var counter llm.TokenCounter
	counter, err := NewCounter(Config{Path: fixture})
	if err != nil {
		t.Fatalf("NewCounter: %v", err)
	}
	// The fixture is a WordLevel tokenizer over a whitespace pre-tokenizer, so
	// counts equal the number of whitespace-separated words.
	got, err := counter.Count("hello world the fox")
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if want := 4; got != want {
		t.Errorf("Count = %d, want %d", got, want)
	}
	first, err := counter.Count("hello world")
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	second, err := counter.Count("hello world")
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if first != second {
		t.Errorf("Count not deterministic: %d != %d", first, second)
	}
	long, err := counter.Count("hello world the fox")
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	short, err := counter.Count("hello")
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if long <= short {
		t.Error("longer text produced no more tokens than shorter text")
	}
}
