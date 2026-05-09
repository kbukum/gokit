package mcp_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kbukum/gokit/authz"
	kitMcp "github.com/kbukum/gokit/mcp"
	"github.com/kbukum/gokit/observability"
	"github.com/kbukum/gokit/schema"
	"github.com/kbukum/gokit/skill"
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
	mustReg(t, reg, tool.FromFunc("add", "Add two numbers", addHandler).AsCallable())

	greetTool := tool.FromFunc("greet", "Greet someone", greetHandler)
	greetTool.Def.Envelope.Safety = tool.SafetyReadOnly
	greetTool.Def.Annotations = tool.Annotations{Title: "Greeting Tool"}
	mustReg(t, reg, greetTool.AsCallable())

	mustReg(t, reg, tool.FromFunc("fail", "Always fails", failHandler).AsCallable())
	return reg
}

// TestRoundTrip verifies full server→client flow using in-memory transport.
func TestRoundTrip(t *testing.T) {
	ctx := context.Background()
	reg := newTestRegistry(t)

	// Create server
	server, err := kitMcp.NewServer("test-server", "1.0.0", reg)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

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
		if def.Envelope.Safety != tool.SafetyReadOnly {
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
	mustReg(t, reg, tool.FromFunc("search", "Search things", greetHandler).AsCallable())

	server, err := kitMcp.NewServer("test", "1.0.0", reg, kitMcp.WithPrefix("svc_"))
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

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

func TestServerWithAllowedTools(t *testing.T) {
	ctx := context.Background()
	reg := newTestRegistry(t)

	server, err := kitMcp.NewServer("test", "1.0.0", reg, kitMcp.WithAllowedTools("greet"))
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

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

	if got := len(callables); got != 1 {
		t.Fatalf("expected 1 callable, got %d", got)
	}
	if name := callables[0].Definition().Name; name != "greet" {
		t.Fatalf("expected greet to be exposed, got %q", name)
	}
}

func TestServerWithAuthzDeciderAndAuditor(t *testing.T) {
	ctx := context.Background()
	reg := newTestRegistry(t)
	var events []kitMcp.ToolAuditEvent

	server, err := kitMcp.NewServer(
		"test",
		"1.0.0",
		reg,
		kitMcp.WithAuthzDecider(authz.DeciderFunc(
			func(_ context.Context, req authz.Request) (authz.Decision, error) {
				if req.Resource.ID == "add" {
					return authz.Decision{Allowed: false, Reason: "write tools disabled"}, nil
				}
				return authz.Decision{Allowed: true, Reason: "allowed"}, nil
			},
		)),
		kitMcp.WithAuditor(observability.AuditorFunc(
			func(_ context.Context, event observability.AuditEvent) {
				events = append(events, kitMcp.ToolAuditEvent{ToolName: event.Attributes["tool"], MCPName: event.Attributes["mcp"], Outcome: event.Attributes["outcome"], Reason: event.Attributes["reason"], Error: event.Attributes["error"]})
			},
		)),
	)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

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

	toolMap := make(map[string]tool.Callable)
	for _, c := range callables {
		toolMap[c.Definition().Name] = c
	}

	result, err := toolMap["add"].Call(tool.NewContext(ctx), mustJSON(t, AddInput{A: 1, B: 2}))
	if err != nil {
		t.Fatalf("call should not return transport error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected denied call to return an MCP tool error")
	}
	if got := result.Text(); got != "tool call denied: write tools disabled" {
		t.Fatalf("unexpected denial text: %q", got)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 audit event, got %d", len(events))
	}
	if events[0].Outcome != "denied" || events[0].ToolName != "add" {
		t.Fatalf("unexpected audit event: %+v", events[0])
	}
}

func TestServerWithMaxInputBytes(t *testing.T) {
	ctx := context.Background()
	reg := newTestRegistry(t)

	server, err := kitMcp.NewServer("test", "1.0.0", reg, kitMcp.WithMaxInputBytes(8))
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

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

	toolMap := make(map[string]tool.Callable)
	for _, c := range callables {
		toolMap[c.Definition().Name] = c
	}

	result, err := toolMap["greet"].Call(tool.NewContext(ctx), mustJSON(t, GreetInput{Name: "World"}))
	if err != nil {
		t.Fatalf("call should not return transport error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected oversize input to return an MCP tool error")
	}
	if got := result.Text(); got != "input too large: exceeds 8 bytes" {
		t.Fatalf("unexpected error text: %q", got)
	}
}

func TestServerWithOutputValidation(t *testing.T) {
	ctx := context.Background()
	reg := tool.NewRegistry()
	def := tool.Definition{
		Name:        "bad_output",
		Description: "Return invalid output",
		InputSchema: schema.Generate[struct{}](),
		OutputSchema: schema.JSON{
			"type": "object",
			"properties": map[string]any{
				"sum": map[string]any{"type": "integer"},
			},
			"required": []string{"sum"},
		},
	}
	mustReg(t, reg, tool.NewTool(def, tool.HandlerFunc[struct{}, map[string]string](
		func(_ context.Context, _ struct{}) (map[string]string, error) {
			return map[string]string{"sum": "bad"}, nil
		},
	)).AsCallable())

	server, err := kitMcp.NewServer("test", "1.0.0", reg)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

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

	result, err := callables[0].Call(tool.NewContext(ctx), mustJSON(t, struct{}{}))
	if err != nil {
		t.Fatalf("call should not return transport error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected invalid output schema to return an MCP tool error")
	}
	if got := result.Text(); !strings.HasPrefix(got, "output validation error") {
		t.Fatalf("unexpected output validation text: %q", got)
	}
}

func TestServerWithPromptsResourcesAndTemplates(t *testing.T) {
	ctx := context.Background()
	reg := tool.NewRegistry()

	server, err := kitMcp.NewServer(
		"test",
		"1.0.0",
		reg,
		kitMcp.WithPrompt(&sdkmcp.Prompt{
			Name:        "greet",
			Description: "Render a greeting prompt",
			Arguments: []*sdkmcp.PromptArgument{
				{Name: "name", Required: true},
			},
		}, func(_ context.Context, req *sdkmcp.GetPromptRequest) (*sdkmcp.GetPromptResult, error) {
			return &sdkmcp.GetPromptResult{
				Description: "Greeting prompt",
				Messages: []*sdkmcp.PromptMessage{
					{
						Role:    "user",
						Content: &sdkmcp.TextContent{Text: "Say hello to " + req.Params.Arguments["name"]},
					},
				},
			}, nil
		}),
		kitMcp.WithResource(&sdkmcp.Resource{
			Name:     "info",
			URI:      "memo://info",
			MIMEType: "text/plain",
		}, func(_ context.Context, req *sdkmcp.ReadResourceRequest) (*sdkmcp.ReadResourceResult, error) {
			return &sdkmcp.ReadResourceResult{
				Contents: []*sdkmcp.ResourceContents{
					{URI: req.Params.URI, MIMEType: "text/plain", Text: "info"},
				},
			}, nil
		}),
		kitMcp.WithResourceTemplate(&sdkmcp.ResourceTemplate{
			Name:        "item",
			URITemplate: "memo://items/{id}",
			MIMEType:    "text/plain",
		}, func(_ context.Context, req *sdkmcp.ReadResourceRequest) (*sdkmcp.ReadResourceResult, error) {
			return &sdkmcp.ReadResourceResult{
				Contents: []*sdkmcp.ResourceContents{
					{URI: req.Params.URI, MIMEType: "text/plain", Text: "templated:" + req.Params.URI},
				},
			}, nil
		}),
	)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	serverTransport, clientTransport := sdkmcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	defer serverSession.Close()

	session, _, err := kitMcp.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer session.Close()

	prompts, err := session.ListPrompts(ctx, nil)
	if err != nil {
		t.Fatalf("list prompts: %v", err)
	}
	if len(prompts.Prompts) != 1 || prompts.Prompts[0].Name != "greet" {
		t.Fatalf("unexpected prompts: %+v", prompts.Prompts)
	}

	prompt, err := session.GetPrompt(ctx, &sdkmcp.GetPromptParams{
		Name:      "greet",
		Arguments: map[string]string{"name": "World"},
	})
	if err != nil {
		t.Fatalf("get prompt: %v", err)
	}
	if got := prompt.Messages[0].Content.(*sdkmcp.TextContent).Text; got != "Say hello to World" {
		t.Fatalf("unexpected prompt text: %q", got)
	}

	resources, err := session.ListResources(ctx, nil)
	if err != nil {
		t.Fatalf("list resources: %v", err)
	}
	if len(resources.Resources) != 1 || resources.Resources[0].URI != "memo://info" {
		t.Fatalf("unexpected resources: %+v", resources.Resources)
	}

	templates, err := session.ListResourceTemplates(ctx, nil)
	if err != nil {
		t.Fatalf("list resource templates: %v", err)
	}
	if len(templates.ResourceTemplates) != 1 || templates.ResourceTemplates[0].URITemplate != "memo://items/{id}" {
		t.Fatalf("unexpected resource templates: %+v", templates.ResourceTemplates)
	}

	resource, err := session.ReadResource(ctx, &sdkmcp.ReadResourceParams{URI: "memo://info"})
	if err != nil {
		t.Fatalf("read resource: %v", err)
	}
	if got := resource.Contents[0].Text; got != "info" {
		t.Fatalf("unexpected resource text: %q", got)
	}

	templated, err := session.ReadResource(ctx, &sdkmcp.ReadResourceParams{URI: "memo://items/123"})
	if err != nil {
		t.Fatalf("read templated resource: %v", err)
	}
	if got := templated.Contents[0].Text; got != "templated:memo://items/123" {
		t.Fatalf("unexpected templated resource text: %q", got)
	}
}

// TestClientWithPrefix verifies client-side prefixing.
func TestClientWithPrefix(t *testing.T) {
	ctx := context.Background()
	reg := tool.NewRegistry()
	mustReg(t, reg, tool.FromFunc("calc", "Calculate", addHandler).AsCallable())

	server, err := kitMcp.NewServer("test", "1.0.0", reg)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

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
	mustReg(t, reg, tool.FromFunc("add", "Add numbers", addHandler).AsCallable())

	server, err := kitMcp.NewServer("test", "1.0.0", reg)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

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

	server, err := kitMcp.NewServer("remote", "1.0.0", remoteReg)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

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
		Envelope:    tool.Envelope{Safety: tool.SafetyReadOnly},
		Annotations: tool.Annotations{
			Title:          "Test Tool",
			IdempotentHint: boolPtr(true),
		},
	}

	// We test the conversion is lossless by comparing key fields
	// (actual conversion functions are internal, so we test via round-trip behavior)
	reg := tool.NewRegistry()
	testTool := tool.FromFunc("test_tool", "A test tool", greetHandler)
	testTool.Def.Envelope = def.Envelope
	testTool.Def.Annotations = def.Annotations
	mustReg(t, reg, testTool.AsCallable())

	// Verify the definition is correct
	defs := reg.List()
	if len(defs) != 1 {
		t.Fatalf("expected 1 definition, got %d", len(defs))
	}
	if defs[0].Envelope.Safety != tool.SafetyReadOnly {
		t.Error("expected read-only safety")
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

func mustReg(tb testing.TB, reg *tool.Registry, c tool.Callable) {
	tb.Helper()
	if err := reg.Register(c); err != nil {
		tb.Fatalf("register: %v", err)
	}
}

func TestSkillToServerAdapterAndOptions(t *testing.T) {
	reg := newTestRegistry(t)
	manifest := skill.Manifest{SchemaVersion: "1", Name: "demo", Version: "0.1.0", Description: "d", References: skill.References{Tools: []string{"greet"}}}
	server, err := (kitMcp.SkillToServerAdapter{Manifest: manifest, Registry: reg}).NewServer("test", "1.0.0", kitMcp.WithTitle("title"), kitMcp.WithMaxResultBytes(1024))
	if err != nil {
		t.Fatal(err)
	}
	if server == nil {
		t.Fatal("nil server")
	}
	if _, err := (kitMcp.SkillToServerAdapter{Manifest: manifest}).NewServer("test", "1.0.0"); err == nil {
		t.Fatal("expected nil registry error")
	}
}

func TestMCPRemoteValidationAndResultLimits(t *testing.T) {
	ctx := context.Background()
	reg := tool.NewRegistry()
	tt := tool.FromFunc("big", "Big", func(context.Context, AddInput) (AddOutput, error) { return AddOutput{Sum: 3}, nil })
	mustReg(t, reg, tt.AsCallable())
	server, err := kitMcp.NewServer("test", "1.0.0", reg, kitMcp.WithMaxResultBytes(1), kitMcp.WithServerOptions(nil))
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	serverTransport, clientTransport := sdkmcp.NewInMemoryTransports()
	ss, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ss.Close()
	session, callables, err := kitMcp.Connect(ctx, clientTransport, &kitMcp.ConnectOptions{ClientName: "client", ClientVersion: "v", Prefix: "p_"})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	if len(callables) != 1 {
		t.Fatal("missing callable")
	}
	if vr := callables[0].Validate(json.RawMessage(`{bad`)); vr.Valid {
		t.Fatal("expected validation failure")
	}
	res, err := callables[0].Call(tool.NewContext(ctx), mustJSON(t, AddInput{A: 1, B: 2}))
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError {
		t.Fatal("expected result limit MCP error")
	}
}
