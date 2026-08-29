package huggingface

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"reflect"
	"strings"

	"github.com/sugarme/tokenizer"
	"github.com/sugarme/tokenizer/pretrained"

	"github.com/kbukum/gokit/fs"
	"github.com/kbukum/gokit/llm"
	"github.com/kbukum/gokit/util"
)

// probeText is encoded once at construction to validate that the loaded
// tokenizer produces tokens without error, so Count operates on a known-good
// tokenizer and never has to surface a per-call encode failure.
const probeText = "the quick brown fox"

// Counter counts tokens with a Hugging Face tokenizer definition. It implements
// [llm.TokenCounter]. Construct it with [FromFile], [FromReader], or [NewCounter].
type counter struct {
	tk          *tokenizer.Tokenizer
	fingerprint string
}

// FromFile builds a counter from a local tokenizer.json file, reading at most
// [DefaultMaxDefinitionBytes]. It never accesses the network.
func FromFile(path string) (llm.TokenCounter, error) {
	return newCounterFromConfig(Config{Path: path})
}

// FromReader builds a counter from a tokenizer.json definition read from r,
// reading at most maxBytes (a non-positive value selects
// [DefaultMaxDefinitionBytes]).
func FromReader(r io.Reader, maxBytes int64) (llm.TokenCounter, error) {
	if maxBytes <= 0 {
		maxBytes = DefaultMaxDefinitionBytes
	}
	if isNilReader(r) {
		return nil, fmt.Errorf("huggingface: reader is required")
	}
	def, err := readBounded(r, maxBytes)
	if err != nil {
		return nil, err
	}
	return newCounter(def)
}

// NewCounter builds an [llm.TokenCounter] from [Config]. This is the explicit,
// config-driven registration seam: the caller injects the returned counter
// wherever a [llm.TokenCounter] is required.
func NewCounter(cfg Config) (llm.TokenCounter, error) {
	return newCounterFromConfig(cfg)
}

// newCounterFromConfig builds a counter from [Config]. It is the concrete
// constructor behind [NewCounter] and [FromFile].
func newCounterFromConfig(cfg Config) (llm.TokenCounter, error) {
	if cfg.Path == "" {
		return nil, fmt.Errorf("huggingface: config Path is required")
	}
	def, err := fs.ReadFileLimit(cfg.Path, cfg.maxBytes())
	if err != nil {
		return nil, fmt.Errorf("huggingface: read definition: %w", err)
	}
	return newCounter(def)
}

// readBounded reads up to maxBytes from r, erroring if the source exceeds the
// bound rather than truncating silently.
func readBounded(r io.Reader, maxBytes int64) ([]byte, error) {
	if isNilReader(r) {
		return nil, fmt.Errorf("huggingface: reader is required")
	}
	probe := maxBytes
	if probe < math.MaxInt64 {
		probe++
	}
	def, err := io.ReadAll(io.LimitReader(r, probe))
	if err != nil {
		return nil, fmt.Errorf("huggingface: read definition: %w", err)
	}
	if int64(len(def)) > maxBytes {
		return nil, fmt.Errorf("huggingface: definition exceeds %d bytes", maxBytes)
	}
	return def, nil
}

func isNilReader(r io.Reader) bool {
	if r == nil {
		return true
	}
	v := reflect.ValueOf(r)
	switch v.Kind() {
	case reflect.Pointer, reflect.Func, reflect.Map, reflect.Slice, reflect.Chan, reflect.Interface:
		return v.IsNil()
	default:
		return false
	}
}

func rejectBPEDropout(def []byte) error {
	var envelope struct {
		Model struct {
			Type    string   `json:"type"`
			Dropout *float64 `json:"dropout"`
		} `json:"model"`
	}
	if err := json.Unmarshal(def, &envelope); err == nil {
		if strings.EqualFold(envelope.Model.Type, "BPE") &&
			envelope.Model.Dropout != nil &&
			*envelope.Model.Dropout != 0 {
			return fmt.Errorf("huggingface: tokenizer model dropout must be 0 for deterministic counting")
		}
	}
	return nil
}

// newCounter parses and validates a tokenizer definition and computes its
// fingerprint.
func newCounter(def []byte) (llm.TokenCounter, error) {
	if len(def) == 0 {
		return nil, fmt.Errorf("huggingface: empty tokenizer definition")
	}
	if err := rejectBPEDropout(def); err != nil {
		return nil, err
	}
	tk, err := parseTokenizer(def)
	if err != nil {
		return nil, err
	}
	if _, err := tk.EncodeSingle(probeText, false); err != nil {
		return nil, fmt.Errorf("huggingface: validate tokenizer: %w", err)
	}
	return &counter{tk: tk, fingerprint: util.Sha256Hex(def)[:12]}, nil
}

// parseTokenizer parses a tokenizer.json definition at an untrusted boundary. It
// contains any panic from the third-party parser — sugarme/tokenizer performs
// unchecked type assertions on the decoded model fields, so a syntactically valid
// but malformed definition (for example a BPE "vocab" of the wrong JSON type)
// panics rather than erroring — and translates it into an error so the
// constructor always honors its error contract.
func parseTokenizer(def []byte) (tk *tokenizer.Tokenizer, err error) {
	defer func() {
		if r := recover(); r != nil {
			tk = nil
			err = fmt.Errorf("huggingface: parse tokenizer: %v", r)
		}
	}()
	tk, err = pretrained.FromReader(bytes.NewReader(def))
	if err != nil {
		return nil, fmt.Errorf("huggingface: parse tokenizer: %w", err)
	}
	return tk, nil
}

// Name returns the stable strategy identifier, for example
// "huggingface:8f3a2b1c9d4e".
func (c *counter) Name() string { return "huggingface:" + c.fingerprint }

// Count returns the number of tokens in text (0 for an empty string). Ordinary
// text is encoded without special tokens, so counting is deterministic and never
// treats input as control tokens. The tokenizer is validated at construction, so
// a runtime encode failure is not expected; should one occur, Count returns the
// error rather than a success-shaped count.
func (c *counter) Count(text string) (int, error) {
	if text == "" {
		return 0, nil
	}
	enc, err := c.tk.EncodeSingle(text, false)
	if err != nil {
		return 0, fmt.Errorf("huggingface: encode: %w", err)
	}
	if enc == nil {
		return 0, fmt.Errorf("huggingface: encode returned no result")
	}
	return len(enc.Ids), nil
}

var _ llm.TokenCounter = (*counter)(nil)
