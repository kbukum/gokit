package tiktoken

import (
	"fmt"
	"sync"

	tk "github.com/pkoukk/tiktoken-go"
	loader "github.com/pkoukk/tiktoken-go-loader"

	"github.com/kbukum/gokit/llm"
)

// offlineOnce installs the embedded, offline BPE loader exactly once. tiktoken-go
// resolves vocabularies through a package-global loader; pointing it at the
// embedded assets keeps counting fully offline with no network access. It is done
// in a constructor under sync.Once rather than an init() so there is no
// import-time side effect.
var offlineOnce sync.Once

func useOfflineVocab() {
	offlineOnce.Do(func() {
		tk.SetBpeLoader(loader.NewOfflineLoader())
	})
}

// Counter counts tokens using an OpenAI BPE encoding. It implements
// [llm.TokenCounter]. Construct it with [New] or [NewCounter].
type counter struct {
	encoding Encoding
	enc      *tk.Tiktoken
}

// New builds a counter for the given [Encoding] from the offline embedded vocab.
// It returns an error for an unknown encoding or if the vocab fails to load.
func New(encoding Encoding) (llm.TokenCounter, error) {
	if !encoding.valid() {
		return nil, fmt.Errorf("tiktoken: unknown encoding %q", encoding)
	}
	useOfflineVocab()
	enc, err := tk.GetEncoding(string(encoding))
	if err != nil {
		return nil, fmt.Errorf("tiktoken: load encoding %q: %w", encoding, err)
	}
	return &counter{encoding: encoding, enc: enc}, nil
}

// NewCounter builds an [llm.TokenCounter] from [Config]. This is the explicit,
// config-driven registration seam: the caller injects the returned counter
// wherever a [llm.TokenCounter] is required.
func NewCounter(cfg Config) (llm.TokenCounter, error) {
	return New(Encoding(cfg.Encoding))
}

// Name returns the stable strategy identifier, for example "tiktoken:cl100k_base".
func (c *counter) Name() string { return "tiktoken:" + string(c.encoding) }

// Count returns the exact number of BPE tokens in text (0 for an empty string).
// Ordinary text is encoded without special-token handling, so counting is
// deterministic and never treats input as control tokens. EncodeOrdinary does
// not fail, so the error is always nil.
func (c *counter) Count(text string) (int, error) {
	if text == "" {
		return 0, nil
	}
	return len(c.enc.EncodeOrdinary(text)), nil
}

var _ llm.TokenCounter = (*counter)(nil)
