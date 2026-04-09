package llm

// StreamEvent is the interface for all stream event types.
// Use type-switch to handle specific events:
//
//	for event := range events {
//	    switch e := event.(type) {
//	    case ContentDelta:
//	        fmt.Print(e.Text)
//	    case ToolCallDelta:
//	        // accumulate tool call
//	    case MessageComplete:
//	        // final message
//	    case StreamError:
//	        // handle error
//	    }
//	}
type StreamEvent interface {
	streamEventMarker()
}

// ContentDelta represents an incremental text content update during streaming.
type ContentDelta struct {
	// Index identifies which content block this delta belongs to.
	Index int `json:"index"`
	// Text is the incremental text content.
	Text string `json:"text"`
}

func (ContentDelta) streamEventMarker() {}

// ToolCallDelta represents an incremental tool call update during streaming.
type ToolCallDelta struct {
	// Index identifies which tool call this delta belongs to.
	Index int `json:"index"`
	// ID is the tool call identifier (set on first delta).
	ID string `json:"id,omitempty"`
	// Name is the tool function name (set on first delta).
	Name string `json:"name,omitempty"`
	// InputDelta is the incremental JSON argument fragment.
	InputDelta string `json:"input_delta"`
}

func (ToolCallDelta) streamEventMarker() {}

// ThinkingDelta represents incremental thinking/reasoning content during streaming.
type ThinkingDelta struct {
	// Text is the incremental thinking content.
	Text string `json:"text"`
}

func (ThinkingDelta) streamEventMarker() {}

// UsageUpdate reports token consumption during streaming.
type UsageUpdate struct {
	// InputTokens is the prompt token count.
	InputTokens int `json:"input_tokens"`
	// OutputTokens is the output token count so far.
	OutputTokens int `json:"output_tokens"`
}

func (UsageUpdate) streamEventMarker() {}

// MessageStart signals the beginning of a streaming response.
type MessageStart struct {
	// ID is the message identifier assigned by the provider.
	ID string `json:"id"`
	// Model is the model producing the response.
	Model string `json:"model"`
}

func (MessageStart) streamEventMarker() {}

// MessageComplete signals the end of a streaming response with the full message.
type MessageComplete struct {
	// Response is the fully assembled completion response.
	Response CompletionResponse `json:"response"`
}

func (MessageComplete) streamEventMarker() {}

// StreamError wraps an error that occurred during streaming.
type StreamError struct {
	// Err is the underlying error.
	Err error `json:"-"`
}

func (StreamError) streamEventMarker() {}

// Error implements the error interface.
func (e StreamError) Error() string {
	if e.Err != nil {
		return e.Err.Error()
	}
	return "llm: unknown stream error"
}
