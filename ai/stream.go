package ai

// StreamEvent is the sealed interface for AI streaming events.
type StreamEvent interface {
	StreamEventMarker()
}

// TextDelta carries incremental text content.
type TextDelta struct {
	Index int    `json:"index"`
	Text  string `json:"text"`
}

func (TextDelta) StreamEventMarker() {}

// UsageDelta reports streaming token usage updates.
type UsageDelta struct {
	InputTokens     int `json:"input_tokens,omitempty"`
	OutputTokens    int `json:"output_tokens,omitempty"`
	CachedTokens    int `json:"cached_tokens,omitempty"`
	ReasoningTokens int `json:"reasoning_tokens,omitempty"`
}

func (UsageDelta) StreamEventMarker() {}

// Error is the terminal stream error event.
type Error struct {
	Err error `json:"-"`
}

func (Error) StreamEventMarker() {}

func (e Error) Error() string {
	if e.Err != nil {
		return e.Err.Error()
	}
	return "ai: unknown stream error"
}

func (e Error) Unwrap() error { return e.Err }
