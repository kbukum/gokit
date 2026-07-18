package chat

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/kbukum/gokit/ai"
	"github.com/kbukum/gokit/errors"
)

// Message is implemented by all conversation message types.
type Message interface {
	Role() string
	messageMarker()
}

// UserMessage carries user-turn content.
type UserMessage struct {
	Content []ai.ContentPart `json:"content"`
}

func (UserMessage) Role() string   { return string(RoleUser) }
func (UserMessage) messageMarker() {}

// AssistantMessage carries model-turn content and tool invocations. Usage is preserved for per-turn spend tracking in conversation history.
type AssistantMessage struct {
	Content   []ai.ContentPart  `json:"content,omitempty"`
	ToolCalls []ai.ToolUseBlock `json:"tool_calls,omitempty"`
	Usage     *ai.Usage         `json:"usage,omitempty"`
}

func (AssistantMessage) Role() string   { return string(RoleAssistant) }
func (AssistantMessage) messageMarker() {}

// SystemMessage carries system instructions.
type SystemMessage struct {
	Content string `json:"content"`
}

func (SystemMessage) Role() string   { return string(RoleSystem) }
func (SystemMessage) messageMarker() {}

// ToolResultMessage carries tool execution results back to the model.
type ToolResultMessage struct {
	ToolUseID string `json:"tool_use_id"`
	Content   string `json:"content"`
	IsError   bool   `json:"is_error,omitempty"`
}

func (ToolResultMessage) Role() string   { return string(RoleTool) }
func (ToolResultMessage) messageMarker() {}

// User constructs a UserMessage with a single text part.
func User(text string) UserMessage {
	return UserMessage{Content: ai.TextContent(text)}
}

// Assistant constructs an AssistantMessage with a single text part.
func Assistant(text string) AssistantMessage {
	return AssistantMessage{Content: ai.TextContent(text)}
}

// System constructs a SystemMessage.
func System(text string) SystemMessage {
	return SystemMessage{Content: text}
}

// ToolResultMsg constructs a ToolResultMessage.
func ToolResultMsg(toolUseID, content string, isError bool) ToolResultMessage {
	return ToolResultMessage{ToolUseID: toolUseID, Content: content, IsError: isError}
}

func (m AssistantMessage) Text() string       { return ai.TextOf(m.Content) }
func (m AssistantMessage) HasToolCalls() bool { return len(m.ToolCalls) > 0 }

// MarshalMessage serializes a Message to JSON with a "role" discriminator. This is a generic marshaler for logging and history storage — not for provider wire format.
func MarshalMessage(m Message) ([]byte, error) {
	switch msg := m.(type) {
	case UserMessage:
		return json.Marshal(struct {
			Role    string           `json:"role"`
			Content []ai.ContentPart `json:"content"`
		}{Role: string(RoleUser), Content: msg.Content})
	case AssistantMessage:
		return json.Marshal(struct {
			Role      string            `json:"role"`
			Content   []ai.ContentPart  `json:"content,omitempty"`
			ToolCalls []ai.ToolUseBlock `json:"tool_calls,omitempty"`
			Usage     *ai.Usage         `json:"usage,omitempty"`
		}{Role: string(RoleAssistant), Content: msg.Content, ToolCalls: msg.ToolCalls, Usage: msg.Usage})
	case SystemMessage:
		return json.Marshal(struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		}{Role: string(RoleSystem), Content: msg.Content})
	case ToolResultMessage:
		return json.Marshal(struct {
			Role      string `json:"role"`
			ToolUseID string `json:"tool_use_id"`
			Content   string `json:"content"`
			IsError   bool   `json:"is_error,omitempty"`
		}{Role: string(RoleTool), ToolUseID: msg.ToolUseID, Content: msg.Content, IsError: msg.IsError})
	default:
		return nil, errors.New(errors.ErrCodeInvalidInput, fmt.Sprintf("ai/chat: unknown message type %T", m), http.StatusBadRequest)
	}
}
