package ai

import (
	"bytes"
	"encoding/json"
)

// ContentPart is the sealed interface for multimodal content parts.
type ContentPart interface {
	PartType() string
	contentPartMarker()
}

// Text is a text content part.
type Text struct {
	Text string `json:"text"`
}

func (Text) PartType() string   { return "text" }
func (Text) BlockType() string  { return "text" }
func (Text) contentPartMarker() {}

// Image is an image content part.
type Image struct {
	Source   string `json:"source"`
	MimeType string `json:"mime_type"`
	Data     string `json:"data,omitempty"`
}

func (Image) PartType() string   { return "image" }
func (Image) BlockType() string  { return "image" }
func (Image) contentPartMarker() {}

// Audio is an audio content part.
type Audio struct {
	Source   string `json:"source"`
	MimeType string `json:"mime_type"`
	Data     string `json:"data,omitempty"`
}

func (Audio) PartType() string   { return "audio" }
func (Audio) BlockType() string  { return "audio" }
func (Audio) contentPartMarker() {}

// Video is a video content part.
type Video struct {
	Source   string `json:"source"`
	MimeType string `json:"mime_type"`
	Data     string `json:"data,omitempty"`
}

func (Video) PartType() string   { return "video" }
func (Video) BlockType() string  { return "video" }
func (Video) contentPartMarker() {}

// File is a file content part.
type File struct {
	Source   string `json:"source"`
	MimeType string `json:"mime_type,omitempty"`
	Name     string `json:"name,omitempty"`
	Data     string `json:"data,omitempty"`
}

func (File) PartType() string   { return "file" }
func (File) BlockType() string  { return "file" }
func (File) contentPartMarker() {}

// ToolUseBlock is the assistant content block form of a tool invocation.
type ToolUseBlock struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Input holds the model-produced tool arguments as a raw JSON object.
	//
	// It is deliberately json.RawMessage rather than a decoded map: the
	// arguments are untrusted model output and must be schema-validated
	// (package schema / tool) before being decoded into a typed input. Keeping
	// the bytes opaque avoids a lossy any round-trip and lets each tool decode
	// into its own concrete type at the trust boundary.
	Input json.RawMessage `json:"input"`
}

func (ToolUseBlock) PartType() string   { return "tool_use" }
func (ToolUseBlock) BlockType() string  { return "tool_use" }
func (ToolUseBlock) contentPartMarker() {}

// NormalizeToolInput returns non-empty, non-null raw JSON for tool arguments,
// substituting an empty object for nil, empty, or JSON null input. Any other
// value is returned trimmed and unchanged — coercion to an object is not
// attempted, since a non-object payload is rejected later by schema validation
// at the tool trust boundary. Providers and stream assemblers use it so
// downstream consumers never have to special-case absent arguments.
func NormalizeToolInput(in json.RawMessage) json.RawMessage {
	trimmed := bytes.TrimSpace(in)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		return json.RawMessage("{}")
	}
	return trimmed
}

// ToolResultBlock is the content block form of a tool result.
type ToolResultBlock struct {
	ID      string `json:"id"`
	Content string `json:"content"`
	IsError bool   `json:"is_error,omitempty"`
}

func (ToolResultBlock) PartType() string   { return "tool_result" }
func (ToolResultBlock) BlockType() string  { return "tool_result" }
func (ToolResultBlock) contentPartMarker() {}

// TextContent wraps a plain string in a single-element ContentPart slice.
func TextContent(text string) []ContentPart {
	return []ContentPart{Text{Text: text}}
}

// TextOf extracts and concatenates all text blocks from a content slice.
func TextOf(blocks []ContentPart) string {
	var result string
	for _, b := range blocks {
		if tb, ok := b.(Text); ok {
			result += tb.Text
		}
	}
	return result
}
