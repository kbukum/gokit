package mcp_test

import (
	"context"
	"testing"
	"time"

	kitMcp "github.com/kbukum/gokit/mcp"
	"github.com/kbukum/gokit/tool"
)

func TestParseTransport(t *testing.T) {
	t.Parallel()
	valid := []struct {
		in   string
		want kitMcp.Transport
	}{
		{"stdio", kitMcp.TransportStdio},
		{"streamable_http", kitMcp.TransportStreamableHTTP},
	}
	for _, c := range valid {
		got, err := kitMcp.ParseTransport(c.in)
		if err != nil {
			t.Errorf("ParseTransport(%q) unexpected error: %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("ParseTransport(%q) = %q want %q", c.in, got, c.want)
		}
	}
	for _, in := range []string{"", "http", "sse", "STDIO", "grpc"} {
		if _, err := kitMcp.ParseTransport(in); err == nil {
			t.Errorf("ParseTransport(%q) must fail closed", in)
		}
	}
}

func FuzzParseTransport(f *testing.F) {
	for _, s := range []string{"stdio", "streamable_http", "sse", "", "x"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, name string) {
		got, err := kitMcp.ParseTransport(name)
		if err != nil {
			return
		}
		// Only the two canonical transports may ever be accepted.
		if got != kitMcp.TransportStdio && got != kitMcp.TransportStreamableHTTP {
			t.Fatalf("accepted non-canonical transport %q", got)
		}
	})
}

func TestServeStdioReturnsOnCancel(t *testing.T) {
	t.Parallel()
	server, err := kitMcp.NewServer("s", "1.0.0", tool.NewRegistry())
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- server.ServeStdio(ctx) }()

	cancel()
	select {
	case <-done:
		// Returned promptly after cancellation (error value is transport-dependent).
	case <-time.After(3 * time.Second):
		t.Fatal("ServeStdio did not return after context cancellation")
	}
}
