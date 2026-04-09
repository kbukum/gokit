package llm

import (
	"encoding/json"
	"fmt"
)

// Message is the interface implemented by all message types.
// Use the concrete types (UserMessage, AssistantMessage, SystemMessage,
// ToolResultMessage) to construct messages, and type-switch when inspecting.
type Message interface {
	// Role returns the message role string.
	Role() string
	messageMarker()
}

// --- Concrete Message Types ---

// UserMessage represents a message from the user.
type UserMessage struct {
	Content []ContentBlock `json:"content"`
}

func (UserMessage) Role() string   { return RoleUser }
func (UserMessage) messageMarker() {}

// AssistantMessage represents a response from the LLM.
type AssistantMessage struct {
	Content   []ContentBlock `json:"content,omitempty"`
	ToolCalls []ToolCall     `json:"tool_calls,omitempty"`
	Usage     *Usage         `json:"usage,omitempty"`
}

func (AssistantMessage) Role() string   { return RoleAssistant }
func (AssistantMessage) messageMarker() {}

// SystemMessage represents a system instruction.
type SystemMessage struct {
	Content string `json:"content"`
}

func (SystemMessage) Role() string   { return RoleSystem }
func (SystemMessage) messageMarker() {}

// ToolResultMessage feeds a tool's output back into the conversation.
type ToolResultMessage struct {
	ToolUseID string `json:"tool_use_id"`
	Content   string `json:"content"`
	IsError   bool   `json:"is_error,omitempty"`
}

func (ToolResultMessage) Role() string   { return RoleTool }
func (ToolResultMessage) messageMarker() {}

// --- Content Block Types ---

// ContentBlock is the interface for multimodal content elements.
type ContentBlock interface {
	BlockType() string
	contentBlockMarker()
}

// TextBlock holds text content.
type TextBlock struct {
	Text string `json:"text"`
}

func (TextBlock) BlockType() string   { return "text" }
func (TextBlock) contentBlockMarker() {}

// ImageBlock holds image data or reference.
type ImageBlock struct {
	Source   string `json:"source"`
	MimeType string `json:"mime_type"`
	Data     string `json:"data,omitempty"`
}

func (ImageBlock) BlockType() string   { return "image" }
func (ImageBlock) contentBlockMarker() {}

// ToolUseBlock represents a tool invocation within assistant content.
type ToolUseBlock struct {
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

func (ToolUseBlock) BlockType() string   { return "tool_use" }
func (ToolUseBlock) contentBlockMarker() {}

// ToolResultBlock represents a tool result within content.
type ToolResultBlock struct {
	ToolUseID string `json:"tool_use_id"`
	Content   string `json:"content"`
	IsError   bool   `json:"is_error,omitempty"`
}

func (ToolResultBlock) BlockType() string   { return "tool_result" }
func (ToolResultBlock) contentBlockMarker() {}

// ThinkingBlock represents model thinking/reasoning content.
type ThinkingBlock struct {
	Text string `json:"text"`
}

func (ThinkingBlock) BlockType() string   { return "thinking" }
func (ThinkingBlock) contentBlockMarker() {}

// --- Stop Reason ---

// StopReason indicates why the model stopped generating.
type StopReason string

const (
	StopEndTurn       StopReason = "end_turn"
	StopToolUse       StopReason = "tool_use"
	StopMaxTokens     StopReason = "max_tokens"
	StopContentFilter StopReason = "content_filter"
	StopSequence      StopReason = "stop_sequence"
)

// --- Convenience Constructors ---

// User creates a UserMessage with a single text block.
func User(text string) UserMessage {
	return UserMessage{Content: TextContent(text)}
}

// Assistant creates an AssistantMessage with a single text block.
func Assistant(text string) AssistantMessage {
	return AssistantMessage{Content: TextContent(text)}
}

// System creates a SystemMessage.
func System(text string) SystemMessage {
	return SystemMessage{Content: text}
}

// ToolResultMsg creates a ToolResultMessage.
func ToolResultMsg(toolUseID, content string, isError bool) ToolResultMessage {
	return ToolResultMessage{
		ToolUseID: toolUseID,
		Content:   content,
		IsError:   isError,
	}
}

// --- Content Helpers ---

// TextContent creates a single-element TextBlock slice.
func TextContent(text string) []ContentBlock {
	return []ContentBlock{TextBlock{Text: text}}
}

// TextOf extracts all text from a slice of content blocks.
func TextOf(blocks []ContentBlock) string {
	var result string
	for _, b := range blocks {
		if tb, ok := b.(TextBlock); ok {
			result += tb.Text
		}
	}
	return result
}

// --- AssistantMessage helpers ---

// Text extracts the text content from an AssistantMessage.
func (m AssistantMessage) Text() string {
	return TextOf(m.Content)
}

// HasToolCalls returns true if the message contains tool call requests.
func (m AssistantMessage) HasToolCalls() bool {
	return len(m.ToolCalls) > 0
}

// --- Message JSON serialization helpers ---

// MarshalMessage serializes a Message to JSON with a "role" field.
func MarshalMessage(m Message) ([]byte, error) {
	switch msg := m.(type) {
	case UserMessage:
		return json.Marshal(struct {
			Role    string         `json:"role"`
			Content []ContentBlock `json:"content"`
		}{Role: RoleUser, Content: msg.Content})
	case AssistantMessage:
		return json.Marshal(struct {
			Role      string         `json:"role"`
			Content   []ContentBlock `json:"content,omitempty"`
			ToolCalls []ToolCall     `json:"tool_calls,omitempty"`
		}{Role: RoleAssistant, Content: msg.Content, ToolCalls: msg.ToolCalls})
	case SystemMessage:
		return json.Marshal(struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		}{Role: RoleSystem, Content: msg.Content})
	case ToolResultMessage:
		return json.Marshal(struct {
			Role      string `json:"role"`
			ToolUseID string `json:"tool_use_id"`
			Content   string `json:"content"`
			IsError   bool   `json:"is_error,omitempty"`
		}{Role: RoleTool, ToolUseID: msg.ToolUseID, Content: msg.Content, IsError: msg.IsError})
	default:
		return nil, fmt.Errorf("llm: unknown message type %T", m)
	}
}
