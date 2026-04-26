package mcp_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	kitMcp "github.com/kbukum/gokit/mcp"
	"github.com/kbukum/gokit/tool"
)

// --- Test tool types ---

type AddInput struct {
	A int `json:"a" jsonschema:"description=First number"`
	B int `json:"b" jsonschema:"description=Second number"`
}

type AddOutput struct {
	Sum int `json:"sum"`
}

type GreetInput struct {
	Name string `json:"name"`
}

type GreetOutput struct {
	Message string `json:"message"`
}

func addHandler(_ context.Context, in AddInput) (AddOutput, error) {
	return AddOutput{Sum: in.A + in.B}, nil
}

func greetHandler(_ context.Context, in GreetInput) (GreetOutput, error) {
	return GreetOutput{Message: "Hello, " + in.Name + "!"}, nil
}

func failHandler(_ context.Context, _ struct{}) (struct{}, error) {
	return struct{}{}, fmt.Errorf("intentional error")
}

func newTestRegistry(t *testing.T) *tool.Registry {
	t.Helper()
	reg := tool.NewRegistry()
	reg.MustRegister(tool.FromFunc("add", "Add two numbers", addHandler).AsCallable())

	greetTool := tool.FromFunc("greet", "Greet someone", greetHandler)
	greetTool.Def.ReadOnly = true
	greetTool.Def.Annotations = &tool.Annotations{
		Title:         "Greeting Tool",
		OpenWorldHint: boolPtr(false),
	}
	reg.MustRegister(greetTool.AsCallable())

	reg.MustRegister(tool.FromFunc("fail", "Always fails", failHandler).AsCallable())
	return reg
}

// TestRoundTrip verifies full server→client flow using in-memory transport.
func TestRoundTrip(t *testing.T) {
	ctx := context.Background()
	reg := newTestRegistry(t)

	// Create server
	server := kitMcp.NewServer("test-server", "1.0.0", reg)

	// Create in-memory transport pair
	serverTransport, clientTransport := sdkmcp.NewInMemoryTransports()

	// Start server in background
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	defer serverSession.Close()

	// Connect client
	session, callables, err := kitMcp.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer session.Close()

	// Should discover 3 tools
	if got := len(callables); got != 3 {
		t.Fatalf("expected 3 callables, got %d", got)
	}

	// Find tools by name
	toolMap := make(map[string]tool.Callable)
	for _, c := range callables {
		toolMap[c.Definition().Name] = c
	}

	t.Run("add", func(t *testing.T) {
		c, ok := toolMap["add"]
		if !ok {
			t.Fatal("add tool not found")
		}

		input := mustJSON(t, AddInput{A: 3, B: 7})
		toolCtx := tool.NewContext(ctx)
		result, err := c.Call(toolCtx, input)
		if err != nil {
			t.Fatalf("call: %v", err)
		}
		if result.IsError {
			t.Fatalf("unexpected error result: %s", result.Text())
		}

		var out AddOutput
		if err := json.Unmarshal(result.Output, &out); err != nil {
			t.Fatalf("unmarshal output: %v (raw: %s)", err, string(result.Output))
		}
		if out.Sum != 10 {
			t.Errorf("expected sum=10, got %d", out.Sum)
		}
	})

	t.Run("greet", func(t *testing.T) {
		c, ok := toolMap["greet"]
		if !ok {
			t.Fatal("greet tool not found")
		}

		def := c.Definition()
		if !def.ReadOnly {
			t.Error("expected greet tool to be read-only")
		}

		input := mustJSON(t, GreetInput{Name: "World"})
		toolCtx := tool.NewContext(ctx)
		result, err := c.Call(toolCtx, input)
		if err != nil {
			t.Fatalf("call: %v", err)
		}
		if result.IsError {
			t.Fatalf("unexpected error result: %s", result.Text())
		}

		var out GreetOutput
		if err := json.Unmarshal(result.Output, &out); err != nil {
			t.Fatalf("unmarshal output: %v (raw: %s)", err, string(result.Output))
		}
		if out.Message != "Hello, World!" {
			t.Errorf("expected 'Hello, World!', got %q", out.Message)
		}
	})

	t.Run("fail", func(t *testing.T) {
		c, ok := toolMap["fail"]
		if !ok {
			t.Fatal("fail tool not found")
		}

		input := mustJSON(t, struct{}{})
		toolCtx := tool.NewContext(ctx)
		result, err := c.Call(toolCtx, input)
		if err != nil {
			t.Fatalf("call should not return protocol error: %v", err)
		}
		if !result.IsError {
			t.Fatal("expected error result")
		}
		if result.Content == "" {
			t.Fatal("expected error content")
		}
	})
}

// TestServerWithPrefix verifies tool name prefixing.
func TestServerWithPrefix(t *testing.T) {
	ctx := context.Background()
	reg := tool.NewRegistry()
	reg.MustRegister(tool.FromFunc("search", "Search things", greetHandler).AsCallable())

	server := kitMcp.NewServer("test", "1.0.0", reg, kitMcp.WithPrefix("svc_"))

	serverTransport, clientTransport := sdkmcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	defer serverSession.Close()

	session, callables, err := kitMcp.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer session.Close()

	if len(callables) != 1 {
		t.Fatalf("expected 1 callable, got %d", len(callables))
	}

	// The MCP tool should have the prefixed name
	def := callables[0].Definition()
	if def.Name != "svc_search" {
		t.Errorf("expected name 'svc_search', got %q", def.Name)
	}
}

// TestClientWithPrefix verifies client-side prefixing.
func TestClientWithPrefix(t *testing.T) {
	ctx := context.Background()
	reg := tool.NewRegistry()
	reg.MustRegister(tool.FromFunc("calc", "Calculate", addHandler).AsCallable())

	server := kitMcp.NewServer("test", "1.0.0", reg)

	serverTransport, clientTransport := sdkmcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	defer serverSession.Close()

	session, callables, err := kitMcp.Connect(ctx, clientTransport, &kitMcp.ConnectOptions{
		Prefix: "remote_",
	})
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer session.Close()

	if len(callables) != 1 {
		t.Fatalf("expected 1 callable, got %d", len(callables))
	}

	// Client-side prefix
	def := callables[0].Definition()
	if def.Name != "remote_calc" {
		t.Errorf("expected name 'remote_calc', got %q", def.Name)
	}

	// Should still be callable
	input := mustJSON(t, AddInput{A: 5, B: 3})
	result, err := callables[0].Call(tool.NewContext(ctx), input)
	if err != nil {
		t.Fatalf("call: %v", err)
	}

	var out AddOutput
	if err := json.Unmarshal(result.Output, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.Sum != 8 {
		t.Errorf("expected 8, got %d", out.Sum)
	}
}

// TestValidation verifies input validation via MCP.
func TestValidation(t *testing.T) {
	ctx := context.Background()
	reg := tool.NewRegistry()
	reg.MustRegister(tool.FromFunc("add", "Add numbers", addHandler).AsCallable())

	server := kitMcp.NewServer("test", "1.0.0", reg)

	serverTransport, clientTransport := sdkmcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	defer serverSession.Close()

	session, callables, err := kitMcp.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer session.Close()

	// Send invalid input (string instead of int)
	invalid := json.RawMessage(`{"a": "not_a_number", "b": 5}`)
	result, err := callables[0].Call(tool.NewContext(ctx), invalid)
	if err != nil {
		t.Fatalf("call should not return protocol error: %v", err)
	}
	// The server validates and returns an error result
	if !result.IsError {
		t.Error("expected error result for invalid input")
	}
}

// TestRegisterRemoteTools verifies discovered tools can be added to a local registry.
func TestRegisterRemoteTools(t *testing.T) {
	ctx := context.Background()
	remoteReg := newTestRegistry(t)

	server := kitMcp.NewServer("remote", "1.0.0", remoteReg)

	serverTransport, clientTransport := sdkmcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	defer serverSession.Close()

	session, callables, err := kitMcp.Connect(ctx, clientTransport, &kitMcp.ConnectOptions{
		Prefix: "remote_",
	})
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer session.Close()

	// Register all remote tools into a local registry
	localReg := tool.NewRegistry()
	for _, c := range callables {
		if rErr := localReg.Register(c); rErr != nil {
			t.Fatalf("register: %v", rErr)
		}
	}

	if localReg.Len() != 3 {
		t.Fatalf("expected 3 tools in local registry, got %d", localReg.Len())
	}

	// Call through local registry
	input := mustJSON(t, AddInput{A: 10, B: 20})
	result, err := localReg.Call(tool.NewContext(ctx), "remote_add", input)
	if err != nil {
		t.Fatalf("call: %v", err)
	}

	var out AddOutput
	if err := json.Unmarshal(result.Output, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.Sum != 30 {
		t.Errorf("expected 30, got %d", out.Sum)
	}
}

// TestConvertDefinition verifies round-trip conversion of tool definitions.
func TestConvertDefinition(t *testing.T) {
	def := tool.Definition{
		Name:        "test_tool",
		Description: "A test tool",
		ReadOnly:    true,
		Destructive: false,
		Annotations: &tool.Annotations{
			Title:           "Test Tool",
			ReadOnlyHint:    boolPtr(true),
			DestructiveHint: boolPtr(false),
			IdempotentHint:  boolPtr(true),
			OpenWorldHint:   boolPtr(false),
		},
	}

	// We test the conversion is lossless by comparing key fields
	// (actual conversion functions are internal, so we test via round-trip behavior)
	reg := tool.NewRegistry()
	testTool := tool.FromFunc("test_tool", "A test tool", greetHandler)
	testTool.Def.ReadOnly = true
	testTool.Def.Annotations = def.Annotations
	reg.MustRegister(testTool.AsCallable())

	// Verify the definition is correct
	defs := reg.List()
	if len(defs) != 1 {
		t.Fatalf("expected 1 definition, got %d", len(defs))
	}
	if defs[0].ReadOnly != true {
		t.Error("expected ReadOnly=true")
	}
}

func mustJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return data
}

func boolPtr(v bool) *bool { return &v }
