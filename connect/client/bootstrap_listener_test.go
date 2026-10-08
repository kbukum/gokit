package client

import (
	"context"
	"net/http"
	"testing"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/wrapperspb"

	bootstraptest "github.com/kbukum/gokit/bootstrap/testutil"
)

func TestBootstrapListenerSupportsProductionClient(t *testing.T) {
	t.Parallel()
	listener := bootstraptest.NewListener("public")
	unary := connect.NewUnaryHandler("/test.Service/Unary", func(context.Context, *connect.Request[wrapperspb.StringValue]) (*connect.Response[wrapperspb.StringValue], error) {
		return connect.NewResponse(wrapperspb.String("unary")), nil
	})
	stream := connect.NewServerStreamHandler("/test.Service/Stream", func(_ context.Context, _ *connect.Request[wrapperspb.StringValue], stream *connect.ServerStream[wrapperspb.StringValue]) error {
		return stream.Send(wrapperspb.String("stream"))
	})
	for path, handler := range map[string]http.Handler{"/test.Service/Unary": unary, "/test.Service/Stream": stream} {
		if err := listener.Handle(path, handler); err != nil {
			t.Fatal(err)
		}
	}
	if err := listener.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := listener.Stop(ctx); err != nil {
			t.Error(err)
		}
	})
	httpClient, err := NewHTTPClient(Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(httpClient.CloseIdleConnections)
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	c := connect.NewClient[wrapperspb.StringValue, wrapperspb.StringValue](httpClient, listener.URL()+"/test.Service/Unary")
	response, err := c.CallUnary(ctx, connect.NewRequest(wrapperspb.String("request")))
	if err != nil {
		t.Fatal(err)
	}
	if response.Msg.Value != "unary" {
		t.Fatalf("unary = %q", response.Msg.Value)
	}
	c = connect.NewClient[wrapperspb.StringValue, wrapperspb.StringValue](httpClient, listener.URL()+"/test.Service/Stream")
	messages, err := c.CallServerStream(ctx, connect.NewRequest(wrapperspb.String("request")))
	if err != nil {
		t.Fatal(err)
	}
	defer messages.Close()
	if !messages.Receive() || messages.Msg().Value != "stream" {
		t.Fatalf("first message: %v", messages.Err())
	}
	if messages.Receive() || messages.Err() != nil {
		t.Fatalf("stream completion: %v", messages.Err())
	}
}
