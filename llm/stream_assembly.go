package llm

import (
	"io"
	"strings"

	"github.com/kbukum/gokit/ai"
	"github.com/kbukum/gokit/ai/chat"
	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/llm/internal/streamwire"
)

type streamAssembler struct {
	limits      StreamLimits
	result      CompletionResponse
	content     strings.Builder
	reasoning   strings.Builder
	calls       []streamToolCall
	bytes       int
	args        int
	done        bool
	metadata    chat.MessageStart
	hasMetadata bool
}

func newStreamAssembler(model string, limits StreamLimits) *streamAssembler {
	return &streamAssembler{limits: limits, result: CompletionResponse{Model: model}}
}

func (a *streamAssembler) add(chunk streamChunk, send func(StreamEvent) error) error {
	if chunk.Err != nil {
		return chunk.Err
	}
	size := len(chunk.Content) + len(chunk.Reasoning)
	if chunk.Metadata != nil {
		size += len(chunk.Metadata.ID) + len(chunk.Metadata.Model) + len(chunk.Metadata.RequestID) + len(chunk.Metadata.Role)
	}
	args := 0
	for _, tc := range chunk.ToolCalls {
		size += len(tc.ID) + len(tc.Name) + len(tc.InputDelta)
		args += len(tc.InputDelta)
	}
	if size > a.limits.MaxOutputBytes-a.bytes {
		return streamLimit("output bytes", a.limits.MaxOutputBytes)
	}
	if args > a.limits.MaxToolArgsBytes-a.args {
		return streamLimit("tool argument bytes", a.limits.MaxToolArgsBytes)
	}
	a.bytes += size
	a.args += args
	if chunk.Metadata != nil {
		if chunk.Metadata.Model != "" {
			a.result.Model = chunk.Metadata.Model
		}
		if chunk.Metadata.ID != "" {
			a.result.ID = chunk.Metadata.ID
		}
		if !a.hasMetadata || a.metadata != *chunk.Metadata {
			a.metadata, a.hasMetadata = *chunk.Metadata, true
			if err := send(*chunk.Metadata); err != nil {
				return err
			}
		}
	}
	if chunk.Reasoning != "" {
		a.reasoning.WriteString(chunk.Reasoning)
		if err := send(ReasoningDelta{Text: chunk.Reasoning}); err != nil {
			return err
		}
	}
	if chunk.Content != "" {
		a.content.WriteString(chunk.Content)
		if err := send(TextDelta{Text: chunk.Content}); err != nil {
			return err
		}
	}
	for _, tc := range chunk.ToolCalls {
		// Check cardinality before appending; argument and metadata bytes were checked above.
		if streamwire.FindTool(a.calls, tc) < 0 && len(a.calls) >= a.limits.MaxTools {
			return streamLimit("tool count", a.limits.MaxTools)
		}
		a.calls = streamwire.MergeToolDelta(a.calls, tc)
		if err := send(ToolUseDelta{Index: tc.Index, ID: tc.ID, Name: tc.Name, InputDelta: tc.InputDelta}); err != nil {
			return err
		}
	}
	if chunk.Usage != nil {
		if err := chunk.Usage.Apply(&a.result.Usage); err != nil {
			return apperrors.New(apperrors.ErrCodeExternalService, "llm: invalid reported usage").WithCause(err)
		}
		a.result.UsageReported = true
		u := a.result.Usage
		if err := send(UsageDelta(u)); err != nil {
			return err
		}
	}
	if chunk.StopReason != "" {
		a.result.StopReason = chunk.StopReason
	}
	a.done = chunk.Done
	return nil
}

func (a *streamAssembler) complete() (CompletionResponse, error) {
	if !a.done {
		return CompletionResponse{}, apperrors.New(apperrors.ErrCodeExternalService, "llm: stream ended without protocol completion").WithCause(io.ErrUnexpectedEOF)
	}
	calls, err := streamwire.ToolUseBlocks(a.calls)
	if err != nil {
		return CompletionResponse{}, apperrors.New(apperrors.ErrCodeExternalService, "llm: invalid streamed tool arguments").WithCause(err)
	}
	a.result.Message = chat.AssistantMessage{ToolCalls: calls}
	if a.content.Len() > 0 {
		a.result.Message.Content = ai.TextContent(a.content.String())
	}
	a.result.Reasoning = a.reasoning.String()
	if a.result.StopReason == "" {
		a.result.StopReason = chat.FinishReasonStop
		if len(calls) > 0 {
			a.result.StopReason = chat.FinishReasonToolUse
		}
	}
	return a.result, nil
}
