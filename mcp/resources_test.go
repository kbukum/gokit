package mcp_test

import (
	"context"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	kitMcp "github.com/kbukum/gokit/mcp"
	"github.com/kbukum/gokit/tool"
)

func resourceHandler(text string) sdkmcp.ResourceHandler {
	return func(_ context.Context, req *sdkmcp.ReadResourceRequest) (*sdkmcp.ReadResourceResult, error) {
		return &sdkmcp.ReadResourceResult{
			Contents: []*sdkmcp.ResourceContents{{URI: req.Params.URI, MIMEType: "text/plain", Text: text}},
		}, nil
	}
}

// TestResourceReadRoundTrip proves a registered resource is readable
// end-to-end. Allow-list and audit gating are covered by the handlers package
// unit tests.
func TestResourceReadRoundTrip(t *testing.T) {
	ctx := context.Background()
	res := &sdkmcp.Resource{URI: "file:///doc.txt", Name: "doc", MIMEType: "text/plain"}
	server, err := kitMcp.NewServer("s", "1.0.0", tool.NewRegistry(), kitMcp.WithResource(res, resourceHandler("hello")))
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	p := connectPeer(t, ctx, server, nil)

	got, err := p.clientSession.ReadResource(ctx, &sdkmcp.ReadResourceParams{URI: "file:///doc.txt"})
	if err != nil {
		t.Fatalf("ReadResource: %v", err)
	}
	if len(got.Contents) != 1 || got.Contents[0].Text != "hello" {
		t.Fatalf("unexpected contents: %+v", got.Contents)
	}
}

// TestResourceTemplateRead proves the SDK matches a registered resource
// template and dispatches through the gated handler.
func TestResourceTemplateRead(t *testing.T) {
	ctx := context.Background()
	tmpl := &sdkmcp.ResourceTemplate{URITemplate: "file:///docs/{name}", Name: "docs"}
	server, err := kitMcp.NewServer("s", "1.0.0", tool.NewRegistry(), kitMcp.WithResourceTemplate(tmpl, resourceHandler("templated")))
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	p := connectPeer(t, ctx, server, nil)

	got, err := p.clientSession.ReadResource(ctx, &sdkmcp.ReadResourceParams{URI: "file:///docs/readme"})
	if err != nil {
		t.Fatalf("ReadResource via template: %v", err)
	}
	if len(got.Contents) != 1 || got.Contents[0].Text != "templated" {
		t.Fatalf("unexpected templated contents: %+v", got.Contents)
	}
}

// TestResourceSubscribeUnsubscribe proves the subscribe/unsubscribe handlers
// wired via WithSubscribeHandler are invoked over the transport.
func TestResourceSubscribeUnsubscribe(t *testing.T) {
	ctx := context.Background()
	subscribed := make(chan string, 1)
	unsubscribed := make(chan string, 1)
	res := &sdkmcp.Resource{URI: "file:///watch.txt", Name: "watch"}
	server, err := kitMcp.NewServer("s", "1.0.0", tool.NewRegistry(),
		kitMcp.WithResource(res, resourceHandler("v1")),
		kitMcp.WithSubscribeHandler(
			func(_ context.Context, r *sdkmcp.SubscribeRequest) error { subscribed <- r.Params.URI; return nil },
			func(_ context.Context, r *sdkmcp.UnsubscribeRequest) error { unsubscribed <- r.Params.URI; return nil },
		))
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	p := connectPeer(t, ctx, server, nil)

	if err := p.clientSession.Subscribe(ctx, &sdkmcp.SubscribeParams{URI: "file:///watch.txt"}); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	if uri := <-subscribed; uri != "file:///watch.txt" {
		t.Errorf("subscribe URI: got %q", uri)
	}
	if err := p.clientSession.Unsubscribe(ctx, &sdkmcp.UnsubscribeParams{URI: "file:///watch.txt"}); err != nil {
		t.Fatalf("Unsubscribe: %v", err)
	}
	if uri := <-unsubscribed; uri != "file:///watch.txt" {
		t.Errorf("unsubscribe URI: got %q", uri)
	}
}
