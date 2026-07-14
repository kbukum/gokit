package security

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kbukum/gokit/authz"
)

func TestAllowListFailsClosed(t *testing.T) {
	t.Parallel()
	empty := &Policy{}
	if !empty.AllowsTool("anything") || !empty.AllowsPrompt("p") || !empty.AllowsResource("r") {
		t.Fatal("empty allow-list must expose everything")
	}
	p := &Policy{
		AllowedTools:     ToSet([]string{"add"}),
		AllowedPrompts:   ToSet([]string{"summary"}),
		AllowedResources: ToSet([]string{"file:///a"}),
	}
	cases := []struct {
		name string
		got  bool
		want bool
	}{
		{"tool allowed", p.AllowsTool("add"), true},
		{"tool denied", p.AllowsTool("delete"), false},
		{"prompt allowed", p.AllowsPrompt("summary"), true},
		{"prompt denied", p.AllowsPrompt("other"), false},
		{"resource allowed", p.AllowsResource("file:///a"), true},
		{"resource denied", p.AllowsResource("file:///b"), false},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s: got %v want %v", c.name, c.got, c.want)
		}
	}
}

func TestSizeLimits(t *testing.T) {
	t.Parallel()
	p := &Policy{MaxInputBytes: 4, MaxResultBytes: 4}
	if p.InputTooLarge(json.RawMessage("abcd")) {
		t.Error("input at limit must be allowed")
	}
	if !p.InputTooLarge(json.RawMessage("abcde")) {
		t.Error("input over limit must fail closed")
	}
	unlimited := &Policy{}
	if unlimited.InputTooLarge(json.RawMessage("anything at all")) || unlimited.ResultTooLarge(1<<20) {
		t.Error("zero limit means unlimited")
	}
	if !p.ResultTooLarge(5) || p.ResultTooLarge(4) {
		t.Error("result limit boundary wrong")
	}
}

func TestAuthorize(t *testing.T) {
	t.Parallel()
	noDecider := &Policy{}
	dec, err := noDecider.Authorize(context.Background(), ToolAuthorizationRequest{ToolName: "x"})
	if err != nil || !dec.Allowed {
		t.Fatalf("nil decider must allow: %+v err=%v", dec, err)
	}

	var seen authz.Request
	p := &Policy{Decider: authz.DeciderFunc(func(_ context.Context, req authz.Request) (authz.Decision, error) {
		seen = req
		return authz.Decision{Allowed: false, Reason: "nope"}, nil
	})}
	dec, err = p.Authorize(context.Background(), ToolAuthorizationRequest{ToolName: "del", MCPName: "svc_del", Arguments: json.RawMessage(`{"id":1}`)})
	if err != nil {
		t.Fatalf("decide: %v", err)
	}
	if dec.Allowed || dec.Reason != "nope" {
		t.Errorf("expected deny with reason, got %+v", dec)
	}
	if seen.Resource.Type != "tool" || seen.Resource.ID != "del" || seen.Action != authz.ActionToolInvoke {
		t.Errorf("authz request mapping wrong: %+v", seen)
	}
	if seen.Context["mcp_name"] != "svc_del" {
		t.Errorf("expected mcp_name in context, got %v", seen.Context)
	}
	if seen.Context["arguments"] != `{"id":1}` {
		t.Errorf("expected raw arguments forwarded to decider, got %v", seen.Context)
	}
}

func TestDeniedMessage(t *testing.T) {
	t.Parallel()
	if got := DeniedMessage(""); got != "tool call denied" {
		t.Errorf("empty reason: got %q", got)
	}
	if got := DeniedMessage("policy"); got != "tool call denied: policy" {
		t.Errorf("with reason: got %q", got)
	}
}
