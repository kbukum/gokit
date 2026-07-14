package mcp_test

import (
	"context"
	"testing"

	kitMcp "github.com/kbukum/gokit/mcp"
	"github.com/kbukum/gokit/tool"
)

func TestConnectDiscoversToolsWithPrefix(t *testing.T) {
	ctx := context.Background()
	server, err := kitMcp.NewServer("s", "1.0.0", newTestRegistry(t))
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	callables := callablesByName(serveClient(t, ctx, server, &kitMcp.ConnectOptions{
		ClientName:    "custom-client",
		ClientVersion: "9.9.9",
		Prefix:        "remote_",
	}))
	if _, ok := callables["remote_add"]; !ok {
		t.Fatalf("expected prefixed 'remote_add', got %v", callables)
	}
}

func TestRemoteCallableValidatesInput(t *testing.T) {
	ctx := context.Background()
	server, err := kitMcp.NewServer("s", "1.0.0", newTestRegistry(t))
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	add := callablesByName(serveClient(t, ctx, server, nil))["add"]
	if vr := add.Validate(mustJSON(t, AddInput{A: 1, B: 2})); !vr.Valid {
		t.Errorf("valid input rejected: %+v", vr.Errors)
	}
	if vr := add.Validate([]byte(`{"a":"not-a-number"}`)); vr.Valid {
		t.Error("invalid input must fail client-side validation")
	}
}

func TestRemoteCallableCallErrorSurfacesAsErrorResult(t *testing.T) {
	ctx := context.Background()
	server, err := kitMcp.NewServer("s", "1.0.0", newTestRegistry(t))
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	fail := callablesByName(serveClient(t, ctx, server, nil))["fail"]
	result, err := fail.Call(tool.NewContext(ctx), mustJSON(t, struct{}{}))
	if err != nil {
		t.Fatalf("call should not return a protocol error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result from failing remote tool")
	}
}

func TestRemoteCallableCallUnmarshalError(t *testing.T) {
	ctx := context.Background()
	server, err := kitMcp.NewServer("s", "1.0.0", newTestRegistry(t))
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	add := callablesByName(serveClient(t, ctx, server, nil))["add"]
	if _, err := add.Call(tool.NewContext(ctx), []byte("{not json")); err == nil {
		t.Fatal("expected unmarshal error for malformed arguments")
	}
}

func TestRemoteCallableCallNoInput(t *testing.T) {
	ctx := context.Background()
	server, err := kitMcp.NewServer("s", "1.0.0", newTestRegistry(t))
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	greet := callablesByName(serveClient(t, ctx, server, nil))["greet"]
	// Empty input exercises the no-arguments path; server validation may reject,
	// but it must surface as an error result, never a transport error.
	if _, err := greet.Call(tool.NewContext(ctx), nil); err != nil {
		t.Fatalf("empty input must not produce a protocol error: %v", err)
	}
}
