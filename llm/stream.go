package llm

import (
	"bufio"
	"context"
	"errors"
	"io"

	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/httpclient"
)

// runStream owns decoding, assembly and the admission lifetime in one goroutine.
//
//nolint:contextcheck // resp.Context derives from deliveryCtx through the HTTP lifetime owner.
func (a *Adapter) runStream(deliveryCtx context.Context, resp *httpclient.StreamResponse, model string, out chan StreamEvent, finished func(error)) {
	defer close(out)
	ctx := resp.Context()
	send := func(event StreamEvent) error {
		select {
		case out <- event:
			return nil
		case <-ctx.Done():
			return context.Cause(ctx)
		}
	}
	result, err := a.consumeStream(resp, model, send)
	err = resp.Complete(err)
	if finished != nil {
		finished(err)
	}
	if err != nil {
		sendTerminalError(out, err)
		return
	}
	select {
	case out <- MessageComplete{Response: result}:
	case <-deliveryCtx.Done():
		sendTerminalError(out, context.Cause(deliveryCtx))
	}
}

// A failed stream may discard its last undelivered delta, but never its terminal error. The sole producer owns a one-slot channel, so replacing that slot cannot block.
func sendTerminalError(out chan StreamEvent, err error) {
	select {
	case out <- StreamError{Err: err}:
		return
	default:
	}
	select {
	case <-out:
	default:
	}
	out <- StreamError{Err: err}
}

func (a *Adapter) consumeStream(resp *httpclient.StreamResponse, model string, send func(StreamEvent) error) (CompletionResponse, error) {
	next, err := a.streamDecoder(resp)
	if err != nil {
		return CompletionResponse{}, err
	}
	assembler := newStreamAssembler(model, a.streamLimits)
	for {
		chunk, err := next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return assembler.complete()
			}
			return CompletionResponse{}, err
		}
		modelProgress := chunk.Content != "" || chunk.Reasoning != "" || len(chunk.ToolCalls) > 0
		if modelProgress || chunk.Metadata != nil || chunk.Usage != nil || chunk.StopReason != "" || chunk.Done {
			resp.Progress(modelProgress)
		}
		if err := assembler.add(chunk, send); err != nil {
			return CompletionResponse{}, err
		}
		if chunk.Done {
			return assembler.complete()
		}
	}
}

func (a *Adapter) streamDecoder(resp *httpclient.StreamResponse) (func() (streamChunk, error), error) {
	switch a.dialect.StreamFormat() {
	case StreamSSE:
		if resp.SSE == nil {
			return nil, ErrNoSSEReader
		}
		return func() (streamChunk, error) {
			event, err := resp.SSE.Next()
			if err != nil {
				return streamChunk{}, err
			}
			return a.dialect.ParseStreamChunk([]byte(event.Data))
		}, nil
	case StreamNDJSON:
		if resp.Body == nil {
			return nil, ErrNoStreamBody
		}
		limit := a.rest.HTTP().GetConfig().Stream.MaxFrameBytes
		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 0, min(4096, limit+2)), limit+2)
		return func() (streamChunk, error) {
			for scanner.Scan() {
				line := scanner.Bytes()
				if len(line) > limit {
					return streamChunk{}, streamLimit("frame bytes", limit)
				}
				if len(line) == 0 {
					continue
				}
				return a.dialect.ParseStreamChunk(line)
			}
			if err := scanner.Err(); err != nil {
				if errors.Is(err, bufio.ErrTooLong) {
					return streamChunk{}, streamLimit("frame bytes", limit)
				}
				return streamChunk{}, apperrors.New(apperrors.ErrCodeExternalService, "llm: read stream frame").WithCause(err)
			}
			return streamChunk{}, io.EOF
		}, nil
	default:
		return nil, apperrors.InvalidInput("stream_format", "unsupported model stream format")
	}
}
