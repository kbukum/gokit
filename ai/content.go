package ai

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
	ID    string         `json:"id"`
	Name  string         `json:"name"`
	Input map[string]any `json:"input"`
}

func (ToolUseBlock) PartType() string   { return "tool_use" }
func (ToolUseBlock) BlockType() string  { return "tool_use" }
func (ToolUseBlock) contentPartMarker() {}

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
