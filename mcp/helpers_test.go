package mcp_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	kitMcp "github.com/kbukum/gokit/mcp"
	"github.com/kbukum/gokit/observability"
	"github.com/kbukum/gokit/tool"
)

// --- Shared test tool types ---

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

// newTestRegistry builds a registry with add (mutating), greet (read-only), and
// fail (always errors) tools.
func newTestRegistry(t *testing.T) *tool.Registry {
	t.Helper()
	reg := tool.NewRegistry()
	mustReg(t, reg, tool.FromFunc("add", "Add two numbers", addHandler).AsCallable())

	greetTool := tool.FromFunc("greet", "Greet someone", greetHandler)
	greetTool.Def.Envelope.Safety = tool.SafetyReadOnly
	greetTool.Def.Annotations = tool.Annotations{Title: "Greeting Tool"}
	mustReg(t, reg, greetTool.AsCallable())

	failTool := tool.FromFunc("fail", "Always fails", func(_ context.Context, _ struct{}) (struct{}, error) {
		return struct{}{}, errTestFailure
	})
	mustReg(t, reg, failTool.AsCallable())
	return reg
}

var errTestFailure = errTest("intentional error")

type errTest string

func (e errTest) Error() string { return string(e) }

// serveClient connects an in-memory MCP client to server and returns the
// discovered callables. The server and client sessions are closed at test end.
func serveClient(t *testing.T, ctx context.Context, server *kitMcp.Server, opts *kitMcp.ConnectOptions) []tool.Callable {
	t.Helper()
	serverTransport, clientTransport := sdkmcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	t.Cleanup(func() { _ = serverSession.Close() })

	session, callables, err := kitMcp.Connect(ctx, clientTransport, opts)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return callables
}

// peer holds the connected server + client sessions for server-to-client
// protocol tests (sampling, elicitation, roots, logging, progress).
type peer struct {
	server        *kitMcp.Server
	serverSession *sdkmcp.ServerSession
	clientSession *sdkmcp.ClientSession
}

// connectPeer wires a kit Server to a raw SDK client over in-memory transports,
// applying clientOpts (used to install client-side sampling/elicitation
// handlers) and advertising roots. Both sessions are closed at test end.
func connectPeer(t *testing.T, ctx context.Context, server *kitMcp.Server, clientOpts *sdkmcp.ClientOptions, roots ...*sdkmcp.Root) peer {
	t.Helper()
	serverTransport, clientTransport := sdkmcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	t.Cleanup(func() { _ = serverSession.Close() })

	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "test-client", Version: "1.0.0"}, clientOpts)
	if len(roots) > 0 {
		client.AddRoots(roots...)
	}
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { _ = clientSession.Close() })
	return peer{server: server, serverSession: serverSession, clientSession: clientSession}
}

// callablesByName indexes callables by their (possibly prefixed) tool name.
func callablesByName(callables []tool.Callable) map[string]tool.Callable {
	m := make(map[string]tool.Callable, len(callables))
	for _, c := range callables {
		m[c.Definition().Name] = c
	}
	return m
}

func mustJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return data
}

func mustReg(tb testing.TB, reg *tool.Registry, c tool.Callable) {
	tb.Helper()
	if err := reg.Register(c); err != nil {
		tb.Fatalf("register: %v", err)
	}
}

// captureAuditor records audit events for e2e assertions.
type captureAuditor struct {
	mu     sync.Mutex
	events []observability.AuditEvent
}

func (c *captureAuditor) Audit(_ context.Context, e observability.AuditEvent) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, e)
}

// outcomeFor returns the recorded outcome for the audit event whose target or
// tool attribute matches target, or "" when none is found.
func (c *captureAuditor) outcomeFor(target string) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, e := range c.events {
		if e.Attributes["target"] == target || e.Attributes["tool"] == target {
			return e.Attributes["outcome"]
		}
	}
	return ""
}

// reasonFor returns the recorded reason for the audit event whose target or
// tool attribute matches target, or "" when none is found.
func (c *captureAuditor) reasonFor(target string) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, e := range c.events {
		if e.Attributes["target"] == target || e.Attributes["tool"] == target {
			return e.Attributes["reason"]
		}
	}
	return ""
}
