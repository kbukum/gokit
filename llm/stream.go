package llm

import (
	"bufio"
	"context"
	"io"

	"github.com/kbukum/gokit/httpclient"
	"github.com/kbukum/gokit/httpclient/sse"
)

// readStream dispatches to the appropriate stream reader based on the dialect's format.
func (a *Adapter) readStream(ctx context.Context, resp *httpclient.StreamResponse, ch chan<- StreamChunk) {
	defer close(ch)

	switch a.dialect.StreamFormat() {
	case StreamSSE:
		a.readSSEStream(ctx, resp.SSE, ch)
	case StreamNDJSON:
		a.readNDJSONStream(ctx, resp.Body, ch)
	}
}

// readSSEStream reads Server-Sent Events and parses each data payload.
func (a *Adapter) readSSEStream(ctx context.Context, reader sse.Reader, ch chan<- StreamChunk) {
	if reader == nil {
		select {
		case ch <- StreamChunk{Err: ErrNoSSEReader}:
		case <-ctx.Done():
		}
		return
	}
	defer func() { _ = reader.Close() }()

	for {
		event, err := reader.Next()
		if err != nil {
			if err != io.EOF {
				select {
				case ch <- StreamChunk{Err: err}:
				case <-ctx.Done():
				}
			}
			return
		}

		chunk, parseErr := a.dialect.ParseStreamChunk([]byte(event.Data))
		if parseErr != nil {
			select {
			case ch <- StreamChunk{Err: parseErr}:
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
func (a *Adapter) readNDJSONStream(ctx context.Context, body io.ReadCloser, ch chan<- StreamChunk) {
	if body == nil {
		ch <- StreamChunk{Err: ErrNoStreamBody}
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
			case ch <- StreamChunk{Err: err}:
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
		case ch <- StreamChunk{Err: err}:
		case <-ctx.Done():
		}
	}
}
