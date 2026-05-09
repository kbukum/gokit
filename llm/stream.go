package llm

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/kbukum/gokit/httpclient"
	"github.com/kbukum/gokit/httpclient/sse"
)

// readStream dispatches to the appropriate stream reader based on the dialect's format.
func (a *Adapter) readStream(ctx context.Context, resp *httpclient.StreamResponse, ch chan<- streamChunk) {
	defer close(ch)

	switch a.dialect.StreamFormat() {
	case StreamSSE:
		a.readSSEStream(ctx, resp.SSE, ch)
	case StreamNDJSON:
		a.readNDJSONStream(ctx, resp.Body, ch)
	default:
		ch <- streamChunk{Err: fmt.Errorf("unsupported stream format: %v", a.dialect.StreamFormat())}
	}
}

// readSSEStream reads Server-Sent Events and parses each data payload.
func (a *Adapter) readSSEStream(ctx context.Context, reader sse.Reader, ch chan<- streamChunk) {
	if reader == nil {
		select {
		case ch <- streamChunk{Err: ErrNoSSEReader}:
		case <-ctx.Done():
		}
		return
	}
	defer func() { _ = reader.Close() }()

	for {
		event, err := reader.Next()
		if err != nil {
			if !errors.Is(err, io.EOF) {
				select {
				case ch <- streamChunk{Err: err}:
				case <-ctx.Done():
				}
			}
			return
		}

		chunk, parseErr := a.dialect.ParseStreamChunk([]byte(event.Data))
		if parseErr != nil {
			select {
			case ch <- streamChunk{Err: parseErr}:
			case <-ctx.Done():
			}
			return
		}

		select {
		case ch <- chunk:
		case <-ctx.Done():
			return
		}
		if chunk.Done {
			return
		}
	}
}

// readNDJSONStream reads newline-delimited JSON and parses each line.
func (a *Adapter) readNDJSONStream(ctx context.Context, body io.ReadCloser, ch chan<- streamChunk) {
	if body == nil {
		ch <- streamChunk{Err: ErrNoStreamBody}
		return
	}
	defer func() { _ = body.Close() }()

	scanner := bufio.NewScanner(body)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		chunk, err := a.dialect.ParseStreamChunk(line)
		if err != nil {
			select {
			case ch <- streamChunk{Err: err}:
			case <-ctx.Done():
			}
			return
		}

		select {
		case ch <- chunk:
		case <-ctx.Done():
			return
		}
		if chunk.Done {
			return
		}
	}
	if err := scanner.Err(); err != nil {
		select {
		case ch <- streamChunk{Err: err}:
		case <-ctx.Done():
		}
	}
}
