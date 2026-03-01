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
		ch <- StreamChunk{Err: ErrNoSSEReader}
		return
	}
	defer func() { _ = reader.Close() }()

	for {
		event, err := reader.Next()
		if err != nil {
			if err != io.EOF {
				ch <- StreamChunk{Err: err}
			}
			return
		}

		content, done, parseErr := a.dialect.ParseStreamChunk([]byte(event.Data))
		if parseErr != nil {
			ch <- StreamChunk{Err: parseErr}
			return
		}

		chunk := StreamChunk{Content: content, Done: done}
		select {
		case ch <- chunk:
		case <-ctx.Done():
			ch <- StreamChunk{Err: ctx.Err()}
			return
		}
		if done {
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

		content, done, err := a.dialect.ParseStreamChunk(line)
		if err != nil {
			ch <- StreamChunk{Err: err}
			return
		}

		chunk := StreamChunk{Content: content, Done: done}
		select {
		case ch <- chunk:
		case <-ctx.Done():
			ch <- StreamChunk{Err: ctx.Err()}
			return
		}
		if done {
			return
		}
	}
	if err := scanner.Err(); err != nil {
		ch <- StreamChunk{Err: err}
	}
}
