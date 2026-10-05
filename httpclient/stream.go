package httpclient

import (
	"context"
	"errors"
	"io"
	"net/http/httptrace"
	"strings"

	"github.com/kbukum/gokit/httpclient/sse"
	rootsse "github.com/kbukum/gokit/sse"
)

//nolint:contextcheck // The lifetime context and Policy.Acquire both derive from ctx; the analyzer cannot trace context fields.
func (c *Adapter) doStream(ctx context.Context, req Request) (*StreamResponse, error) {
	life := newStreamLifetime(ctx, c.config.Stream, c.streamClock, req.RequireStreamCompletion)
	callCtx, release, err := c.config.ResiliencePolicy.Acquire(life.ctx)
	if err != nil {
		life.finish(err)
		<-life.watched
		return nil, life.outcome
	}
	life.release = release
	// A policy can impose a shorter lifetime than the transport budget.
	stopPolicy := context.AfterFunc(callCtx, func() { life.cancel(context.Cause(callCtx)) })
	originalRelease := life.release
	life.release = func(err error) error {
		stopPolicy()
		return originalRelease(err)
	}
	fail := func(err error) (*StreamResponse, error) {
		life.finish(err)
		<-life.watched
		return nil, life.outcome
	}
	httpReq, err := c.buildRequest(callCtx, req)
	if err != nil {
		return fail(err)
	}
	life.setPhase("headers", c.config.Stream.HeaderTimeout)
	httpReq = httpReq.WithContext(httptrace.WithClientTrace(httpReq.Context(), &httptrace.ClientTrace{
		GetConn: func(string) { life.setPhase("connect", c.config.Stream.ConnectTimeout) },
		GotConn: func(httptrace.GotConnInfo) { life.setPhase("headers", c.config.Stream.HeaderTimeout) },
	}))
	streamClient := *c.httpClient
	streamClient.Timeout = 0
	resp, err := streamClient.Do(httpReq) //nolint:bodyclose // Ownership transfers to lifetime.attach; every exit closes through lifetime.finish.
	if err != nil {
		return fail(NewConnectionError(err))
	}
	life.attach(resp.Body)
	life.setPhase("first_progress", c.config.Stream.FirstProgressTimeout)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, c.config.MaxResponseBodyBytes+1))
		if readErr != nil {
			return fail(NewConnectionError(readErr))
		}
		if int64(len(body)) > c.config.MaxResponseBodyBytes {
			return fail(NewResponseTooLargeError(c.config.MaxResponseBodyBytes))
		}
		return fail(ClassifyStatusCode(resp.StatusCode, body))
	}
	body := &streamBody{life: life, rawEOF: !strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream")}
	result := &StreamResponse{StatusCode: resp.StatusCode, Headers: flattenHeaders(resp.Header), lifetime: life}
	if body.rawEOF {
		result.Body = body
	} else {
		result.SSE = &streamReader{
			Reader: sse.NewReader(body, rootsse.WithMaxFrameSize(c.config.Stream.MaxFrameBytes), rootsse.WithMaxLineSize(c.config.Stream.MaxFrameBytes)),
			life:   life,
		}
	}
	if !life.manual {
		// Generic callers need not keep reading for a cancellation to release capacity.
		go func() {
			<-life.watched
			life.finish(context.Cause(life.ctx))
		}()
	}
	return result, nil
}

type streamReader struct {
	sse.Reader
	life *streamLifetime
}

func (r *streamReader) Next() (*sse.Event, error) {
	if err, done := r.life.terminal(); done {
		return nil, err
	}
	event, err := r.Reader.Next()
	if cause := context.Cause(r.life.ctx); cause != nil {
		err = cause
	}
	if !r.life.manual {
		if err == nil {
			r.life.progress(true)
		} else {
			outcome := err
			if errors.Is(err, io.EOF) {
				outcome = nil
			}
			r.life.finish(outcome)
			if r.life.outcome != nil {
				err = r.life.outcome
			}
		}
	}
	return event, err
}
