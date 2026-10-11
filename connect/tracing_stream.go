package connect

import (
	"context"
	"errors"
	"io"
	"net/http"
	"sync"

	"connectrpc.com/connect"
	"go.opentelemetry.io/otel/trace"
)

func (i *tracingInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return func(ctx context.Context, spec connect.Spec) connect.StreamingClientConn {
		header := make(http.Header)
		ctx, span := i.start(ctx, spec, header)
		conn := next(ctx, spec)
		return &tracedStream{
			StreamingClientConn: conn, span: span, header: header,
			fields: i.cfg.Propagator.Fields(),
		}
	}
}

type tracedStream struct {
	connect.StreamingClientConn
	span       trace.Span
	once       sync.Once
	injectOnce sync.Once
	header     http.Header
	fields     []string
}

func (s *tracedStream) finish(err error) {
	s.once.Do(func() {
		setRPCSpanStatus(s.span, err)
		s.span.End()
	})
}

func (s *tracedStream) Send(message any) error {
	s.inject()
	err := s.StreamingClientConn.Send(message)
	if err != nil && !errors.Is(err, io.EOF) {
		s.finish(err)
	}
	return err
}

func (s *tracedStream) Receive(message any) error {
	err := s.StreamingClientConn.Receive(message)
	if errors.Is(err, io.EOF) {
		s.finish(nil)
	} else if err != nil {
		s.finish(err)
	}
	return err
}

func (s *tracedStream) CloseRequest() error {
	s.inject()
	err := s.StreamingClientConn.CloseRequest()
	if err != nil && !errors.Is(err, io.EOF) {
		s.finish(err)
	}
	return err
}

func (s *tracedStream) CloseResponse() error {
	err := s.StreamingClientConn.CloseResponse()
	statusErr := err
	if statusErr == nil {
		statusErr = context.Canceled
	}
	// A terminal Receive already ended the span; otherwise Close abandons it.
	s.finish(statusErr)
	return err
}

func (s *tracedStream) inject() {
	s.injectOnce.Do(func() {
		header := s.RequestHeader()
		for _, key := range s.fields {
			header.Del(key)
		}
		header.Del("Baggage")
		for key, values := range s.header {
			header[key] = values
		}
	})
}
