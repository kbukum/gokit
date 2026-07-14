package handlers

import (
	"context"
	"errors"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kbukum/gokit/mcp/security"
	"github.com/kbukum/gokit/tool"
)

func resourceReader(text string) sdkmcp.ResourceHandler {
	return func(_ context.Context, req *sdkmcp.ReadResourceRequest) (*sdkmcp.ReadResourceResult, error) {
		return &sdkmcp.ReadResourceResult{
			Contents: []*sdkmcp.ResourceContents{{URI: req.Params.URI, MIMEType: "text/plain", Text: text}},
		}, nil
	}
}

func resourceHandlerFor(t *testing.T, policy *security.Policy) *Handler {
	t.Helper()
	sdk := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "test", Version: "1.0.0"}, nil)
	return New(sdk, tool.NewRegistry(), policy, "")
}

func TestWrapResourceHandlerAllows(t *testing.T) {
	t.Parallel()
	sink := &auditSink{}
	h := resourceHandlerFor(t, &security.Policy{Auditor: sink})
	wrapped := h.wrapResourceHandler("file:///doc.txt", resourceReader("hello"))
	req := &sdkmcp.ReadResourceRequest{Params: &sdkmcp.ReadResourceParams{URI: "file:///doc.txt"}}
	got, err := wrapped(context.Background(), req)
	if err != nil {
		t.Fatalf("wrapped resource: %v", err)
	}
	if len(got.Contents) != 1 || got.Contents[0].Text != "hello" {
		t.Fatalf("unexpected contents: %+v", got.Contents)
	}
	if sink.last().Attributes["outcome"] != security.OutcomeSuccess {
		t.Errorf("expected success audit, got %q", sink.last().Attributes["outcome"])
	}
}

func TestWrapResourceHandlerDenies(t *testing.T) {
	t.Parallel()
	sink := &auditSink{}
	h := resourceHandlerFor(t, &security.Policy{
		AllowedResources: security.ToSet([]string{"file:///public.txt"}),
		Auditor:          sink,
	})
	wrapped := h.wrapResourceHandler("file:///secret.txt", resourceReader("secret"))
	req := &sdkmcp.ReadResourceRequest{Params: &sdkmcp.ReadResourceParams{URI: "file:///secret.txt"}}
	if _, err := wrapped(context.Background(), req); err == nil {
		t.Fatal("expected denial for resource not in allow-list")
	}
	if sink.last().Attributes["outcome"] != security.OutcomeDenied {
		t.Errorf("expected denied audit, got %q", sink.last().Attributes["outcome"])
	}
}

func TestWrapResourceHandlerErrorAudited(t *testing.T) {
	t.Parallel()
	sink := &auditSink{}
	h := resourceHandlerFor(t, &security.Policy{Auditor: sink})
	handler := func(_ context.Context, _ *sdkmcp.ReadResourceRequest) (*sdkmcp.ReadResourceResult, error) {
		return nil, errors.New("read failed")
	}
	wrapped := h.wrapResourceHandler("file:///boom.txt", handler)
	req := &sdkmcp.ReadResourceRequest{Params: &sdkmcp.ReadResourceParams{URI: "file:///boom.txt"}}
	if _, err := wrapped(context.Background(), req); err == nil {
		t.Fatal("expected handler error to propagate")
	}
	if sink.last().Attributes["outcome"] != security.OutcomeToolError {
		t.Errorf("expected tool_error audit, got %q", sink.last().Attributes["outcome"])
	}
}
