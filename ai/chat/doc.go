// Package chat provides chat-completion-specific types for the gokit AI module.
//
// This package holds types that are specific to conversational/chat APIs:
// roles, message types (user/assistant/system/tool-result), chat stream events
// (MessageStart, MessageStop, tool-use events, finish reasons), and token
// counting over message slices.
//
// Universal AI/ML primitives (ContentPart, ToolUseBlock, Model, Usage,
// StreamEvent interface, TextDelta, UsageDelta, Error) live in the parent ai/
// package and are consumed by both chat and non-chat modules (embedding,
// inference, transcription).
//
// Modules that only do text-generation streaming (inference, embedding) import
// ai/ only. Modules that do conversational LLM APIs (llm, agent, mcp) import
// both ai/ and ai/chat/.
package chat
