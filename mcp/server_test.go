package mcp_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	kitMcp "github.com/kbukum/gokit/mcp"
	"github.com/kbukum/gokit/tool"
)

func TestServerOptionsApplied(t *testing.T) {
	t.Parallel()
	server, err := kitMcp.NewServer("test", "1.0.0", newTestRegistry(t),
		kitMcp.WithTitle("My Service"),
		kitMcp.WithInstructions("be careful"),
		kitMcp.WithLogger(slog.Default()),
		kitMcp.WithServerOptions(&sdkmcp.ServerOptions{PageSize: 10}),
		kitMcp.WithProgressHandler(func(context.Context, *sdkmcp.ProgressNotificationServerRequest) {}),
		kitMcp.WithRootsListChangedHandler(func(context.Context, *sdkmcp.RootsListChangedRequest) {}),
	)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	if server.SDK() == nil {
		t.Fatal("SDK() must return the underlying server")
	}
}

func TestNewServerRejectsNilRegistry(t *testing.T) {
	t.Parallel()
	if _, err := kitMcp.NewServer("s", "1.0.0", nil); err == nil {
		t.Fatal("expected error for nil registry")
	}
}

func TestRoundTrip(t *testing.T) {
	ctx := context.Background()
	server, err := kitMcp.NewServer("test-server", "1.0.0", newTestRegistry(t))
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	tools := callablesByName(serveClient(t, ctx, server, nil))
	if len(tools) != 3 {
		t.Fatalf("expected 3 callables, got %d", len(tools))
	}

	t.Run("add", func(t *testing.T) {
		result, err := tools["add"].Call(tool.NewContext(ctx), mustJSON(t, AddInput{A: 3, B: 7}))
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

	t.Run("greet_is_read_only", func(t *testing.T) {
		def := tools["greet"].Definition()
		if def.Envelope.Safety != tool.SafetyReadOnly {
			t.Error("expected greet tool to be read-only")
		}
		result, err := tools["greet"].Call(tool.NewContext(ctx), mustJSON(t, GreetInput{Name: "World"}))
		if err != nil {
			t.Fatalf("call: %v", err)
		}
		var out GreetOutput
		if err := json.Unmarshal(result.Output, &out); err != nil {
			t.Fatalf("unmarshal output: %v", err)
		}
		if out.Message != "Hello, World!" {
			t.Errorf("expected 'Hello, World!', got %q", out.Message)
		}
	})

	t.Run("fail_surfaces_as_error_result", func(t *testing.T) {
		result, err := tools["fail"].Call(tool.NewContext(ctx), mustJSON(t, struct{}{}))
		if err != nil {
			t.Fatalf("call should not return protocol error: %v", err)
		}
		if !result.IsError || result.Content == "" {
			t.Fatal("expected error result with content")
		}
	})
}

func TestServerWithPrefix(t *testing.T) {
	ctx := context.Background()
	reg := tool.NewRegistry()
	mustReg(t, reg, tool.FromFunc("search", "Search things", greetHandler).AsCallable())

	server, err := kitMcp.NewServer("test", "1.0.0", reg, kitMcp.WithPrefix("svc_"))
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	callables := serveClient(t, ctx, server, nil)
	if len(callables) != 1 {
		t.Fatalf("expected 1 callable, got %d", len(callables))
	}
	if def := callables[0].Definition(); def.Name != "svc_search" {
		t.Errorf("expected name 'svc_search', got %q", def.Name)
	}
}
