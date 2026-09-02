package ai

// StreamEvent is the sealed interface for AI streaming events.
// EventType returns the canonical wire string identifying the event variant,
// shared with the sibling kit so streamed events interoperate across kits.
type StreamEvent interface {
	StreamEventMarker()
	EventType() string
}

// Canonical stream event-type wire strings, shared across kits.
const (
	EventTypeMessageStart    = "message.start"
	EventTypeTextDelta       = "text.delta"
	EventTypeReasoningDelta  = "reasoning.delta"
	EventTypeToolUseStart    = "tool_use.start"
	EventTypeToolUseDelta    = "tool_use.delta"
	EventTypeToolUseStop     = "tool_use.stop"
	EventTypeUsageDelta      = "usage.delta"
	EventTypeMessageStop     = "message.stop"
	EventTypeMessageComplete = "message.complete"
	EventTypeError           = "error"
)

// TextDelta carries incremental text content.
type TextDelta struct {
	Index int    `json:"index"`
	Text  string `json:"text"`
}

func (TextDelta) StreamEventMarker() {}

// EventType returns the canonical wire string for a text delta.
func (TextDelta) EventType() string { return EventTypeTextDelta }

// UsageDelta reports streaming token usage updates.
type UsageDelta struct {
	InputTokens     int `json:"input_tokens,omitempty"`
	OutputTokens    int `json:"output_tokens,omitempty"`
	CachedTokens    int `json:"cached_tokens,omitempty"`
	ReasoningTokens int `json:"reasoning_tokens,omitempty"`
}

func (UsageDelta) StreamEventMarker() {}

// EventType returns the canonical wire string for a usage delta.
func (UsageDelta) EventType() string { return EventTypeUsageDelta }

// Error is the terminal stream error event.
type Error struct {
	Err error `json:"-"`
}

func (Error) StreamEventMarker() {}

// EventType returns the canonical wire string for a terminal stream error.
func (Error) EventType() string { return EventTypeError }

func (e Error) Error() string {
	if e.Err != nil {
		return e.Err.Error()
	}
	return "ai: unknown stream error"
}

func (e Error) Unwrap() error { return e.Err }
