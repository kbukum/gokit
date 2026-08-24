package handlers

import (
	"context"
	"strings"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kbukum/gokit/mcp/security"
)

// reqWithInputResponses builds a re-invocation request carrying the given
// server input responses keyed by label.
func reqWithInputResponses(responses sdkmcp.InputResponseMap) *sdkmcp.CallToolRequest {
	return &sdkmcp.CallToolRequest{Params: &sdkmcp.CallToolParamsRaw{InputResponses: responses}}
}

func TestSampleResponseExtractsAndAudits(t *testing.T) {
	t.Parallel()
	sink := &auditSink{}
	h := newHandler(t, nil, &security.Policy{Auditor: sink})
	want := &sdkmcp.CreateMessageWithToolsResult{
		Model:   "m",
		Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: "sampled"}},
	}
	got, err := h.SampleResponse(context.Background(),
		reqWithInputResponses(sdkmcp.InputResponseMap{"draft": want}), "draft")
	if err != nil {
		t.Fatalf("SampleResponse: %v", err)
	}
	if got.Model != "m" {
		t.Fatalf("unexpected result: %#v", got)
	}
	if sink.last().Attributes["outcome"] != security.OutcomeSuccess {
		t.Errorf("expected success audit, got %q", sink.last().Attributes["outcome"])
	}
}

func TestSampleResponseMissingFailsClosed(t *testing.T) {
	t.Parallel()
	sink := &auditSink{}
	h := newHandler(t, nil, &security.Policy{Auditor: sink})
	_, err := h.SampleResponse(context.Background(),
		reqWithInputResponses(sdkmcp.InputResponseMap{}), "draft")
	if err == nil {
		t.Fatal("missing input response must fail closed")
	}
	if sink.last().Attributes["outcome"] != security.OutcomeToolError {
		t.Errorf("expected tool_error audit, got %q", sink.last().Attributes["outcome"])
	}
}

// TestSampleResponseOversizedContentFailsClosed proves untrusted sampled model
// output that exceeds the configured result-size limit is rejected, not returned.
func TestSampleResponseOversizedContentFailsClosed(t *testing.T) {
	t.Parallel()
	sink := &auditSink{}
	h := newHandler(t, nil, &security.Policy{MaxResultBytes: 8, Auditor: sink})
	big := &sdkmcp.CreateMessageWithToolsResult{
		Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: strings.Repeat("x", 256)}},
	}
	_, err := h.SampleResponse(context.Background(),
		reqWithInputResponses(sdkmcp.InputResponseMap{"draft": big}), "draft")
	if err == nil || !strings.Contains(err.Error(), "too large") {
		t.Fatalf("expected oversized rejection, got %v", err)
	}
	if sink.last().Attributes["outcome"] != security.OutcomeResultTooLarge {
		t.Errorf("expected result_too_large audit, got %q", sink.last().Attributes["outcome"])
	}
}

func TestElicitResponseExtractsAndAudits(t *testing.T) {
	t.Parallel()
	sink := &auditSink{}
	h := newHandler(t, nil, &security.Policy{Auditor: sink})
	want := &sdkmcp.ElicitResult{Action: "accept", Content: map[string]any{"name": "go"}}
	got, err := h.ElicitResponse(context.Background(),
		reqWithInputResponses(sdkmcp.InputResponseMap{"form": want}), "form")
	if err != nil {
		t.Fatalf("ElicitResponse: %v", err)
	}
	if got.Action != "accept" {
		t.Fatalf("unexpected result: %#v", got)
	}
	if sink.last().Attributes["outcome"] != security.OutcomeSuccess {
		t.Errorf("expected success audit, got %q", sink.last().Attributes["outcome"])
	}
}

// TestElicitResponseWrongTypeFailsClosed proves a response of an unexpected
// concrete type for the label is rejected rather than mis-cast.
func TestElicitResponseWrongTypeFailsClosed(t *testing.T) {
	t.Parallel()
	sink := &auditSink{}
	h := newHandler(t, nil, &security.Policy{Auditor: sink})
	// A roots result stored under the label the caller reads as an elicitation.
	_, err := h.ElicitResponse(context.Background(),
		reqWithInputResponses(sdkmcp.InputResponseMap{"form": &sdkmcp.ListRootsResult{}}), "form")
	if err == nil || !strings.Contains(err.Error(), "unexpected type") {
		t.Fatalf("expected type-mismatch rejection, got %v", err)
	}
	if sink.last().Attributes["outcome"] != security.OutcomeToolError {
		t.Errorf("expected tool_error audit, got %q", sink.last().Attributes["outcome"])
	}
}

// TestElicitResponseTypedNilFailsClosed proves a typed-nil pointer (which
// satisfies the interface but would panic on deref) is rejected.
func TestElicitResponseTypedNilFailsClosed(t *testing.T) {
	t.Parallel()
	h := newHandler(t, nil, &security.Policy{})
	var typedNil *sdkmcp.ElicitResult
	_, err := h.ElicitResponse(context.Background(),
		reqWithInputResponses(sdkmcp.InputResponseMap{"form": typedNil}), "form")
	if err == nil || !strings.Contains(err.Error(), "is nil") {
		t.Fatalf("expected typed-nil rejection, got %v", err)
	}
}

func TestRootsResponseExtractsAndAudits(t *testing.T) {
	t.Parallel()
	sink := &auditSink{}
	h := newHandler(t, nil, &security.Policy{Auditor: sink})
	want := &sdkmcp.ListRootsResult{Roots: []*sdkmcp.Root{{URI: "file:///workspace"}}}
	got, err := h.RootsResponse(context.Background(),
		reqWithInputResponses(sdkmcp.InputResponseMap{"roots": want}), "roots")
	if err != nil {
		t.Fatalf("RootsResponse: %v", err)
	}
	if len(got.Roots) != 1 {
		t.Fatalf("unexpected roots: %#v", got.Roots)
	}
	if sink.last().Attributes["outcome"] != security.OutcomeSuccess {
		t.Errorf("expected success audit, got %q", sink.last().Attributes["outcome"])
	}
}

// TestServerToClientCallsRequireSession proves every server-driven call fails
// closed with a typed error when there is no active session, rather than
// dereferencing a nil session.
func TestServerToClientCallsRequireSession(t *testing.T) {
	t.Parallel()
	h := newHandler(t, nil, &security.Policy{})
	ctx := context.Background()

	if _, err := h.Sample(ctx, nil, &sdkmcp.CreateMessageParams{}); err == nil {
		t.Error("Sample with nil session must error")
	}
	if _, err := h.Elicit(ctx, nil, &sdkmcp.ElicitParams{}); err == nil {
		t.Error("Elicit with nil session must error")
	}
	if _, err := h.ListRoots(ctx, nil, &sdkmcp.ListRootsParams{}); err == nil {
		t.Error("ListRoots with nil session must error")
	}
	if err := h.Log(ctx, nil, &sdkmcp.LoggingMessageParams{}); err == nil {
		t.Error("Log with nil session must error")
	}
	if err := h.NotifyProgress(ctx, nil, &sdkmcp.ProgressNotificationParams{}); err == nil {
		t.Error("NotifyProgress with nil session must error")
	}
}
