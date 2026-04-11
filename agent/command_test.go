package agent_test

import (
	"context"
	"strings"
	"testing"

	"github.com/kbukum/gokit/agent"
	"github.com/kbukum/gokit/hook"
	"github.com/kbukum/gokit/llm"
)

// --- CommandRegistry Tests ---

func TestCommandRegistry_RegisterAndGet(t *testing.T) {
	r := agent.NewCommandRegistry()
	r.Register(agent.Command{
		Name:        "ping",
		Description: "Ping the agent",
		Handler: func(_ context.Context, _ string, _ *agent.Agent) (string, error) {
			return "pong", nil
		},
	})

	cmd, ok := r.Get("ping")
	if !ok {
		t.Fatal("expected to find 'ping' command")
	}
	if cmd.Name != "ping" {
		t.Errorf("Name = %q, want %q", cmd.Name, "ping")
	}

	_, ok = r.Get("nonexistent")
	if ok {
		t.Error("expected 'nonexistent' to not be found")
	}
}

func TestCommandRegistry_List(t *testing.T) {
	r := agent.NewCommandRegistry()
	r.Register(agent.Command{Name: "beta", Description: "B"})
	r.Register(agent.Command{Name: "alpha", Description: "A"})
	r.Register(agent.Command{Name: "gamma", Description: "G"})

	cmds := r.List()
	if len(cmds) != 3 {
		t.Fatalf("expected 3 commands, got %d", len(cmds))
	}
	if cmds[0].Name != "alpha" || cmds[1].Name != "beta" || cmds[2].Name != "gamma" {
		t.Errorf("commands not sorted: %v, %v, %v", cmds[0].Name, cmds[1].Name, cmds[2].Name)
	}
}

func TestCommandRegistry_ParseCommand(t *testing.T) {
	r := agent.NewCommandRegistry()

	tests := []struct {
		input    string
		wantName string
		wantArgs string
		wantOK   bool
	}{
		{"/help", "help", "", true},
		{"/model gpt-4o", "model", "gpt-4o", true},
		{"/model   gpt-4o  ", "model", "gpt-4o", true},
		{"  /clear  ", "clear", "", true},
		{"hello", "", "", false},
		{"/", "", "", false},
		{"", "", "", false},
		{"/compact extra args here", "compact", "extra args here", true},
	}

	for _, tt := range tests {
		name, args, ok := r.ParseCommand(tt.input)
		if ok != tt.wantOK {
			t.Errorf("ParseCommand(%q) ok = %v, want %v", tt.input, ok, tt.wantOK)
			continue
		}
		if name != tt.wantName {
			t.Errorf("ParseCommand(%q) name = %q, want %q", tt.input, name, tt.wantName)
		}
		if args != tt.wantArgs {
			t.Errorf("ParseCommand(%q) args = %q, want %q", tt.input, args, tt.wantArgs)
		}
	}
}

func TestCommandRegistry_IsCommand(t *testing.T) {
	r := agent.NewCommandRegistry()
	r.Register(agent.Command{Name: "help"})
	r.Register(agent.Command{Name: "model"})

	if !r.IsCommand("/help") {
		t.Error("expected /help to be a command")
	}
	if !r.IsCommand("/model gpt-4") {
		t.Error("expected /model gpt-4 to be a command")
	}
	if r.IsCommand("/unknown") {
		t.Error("expected /unknown not to be a command")
	}
	if r.IsCommand("hello") {
		t.Error("expected 'hello' not to be a command")
	}
}

// --- Built-in Command Tests ---

func TestBuiltin_Help(t *testing.T) {
	reg := agent.NewCommandRegistry()
	reg.RegisterBuiltins()

	a := agent.New(agent.Config{
		Provider: newMockProvider(textResponse("unused")),
		Commands: reg,
	})

	result, err := a.Run(context.Background(), []llm.Message{llm.User("/help")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.StopReason != agent.StopCommand {
		t.Errorf("StopReason = %q, want %q", result.StopReason, agent.StopCommand)
	}

	output := result.FinalMessage.Text()
	if !strings.Contains(output, "Available commands:") {
		t.Errorf("expected help output to contain 'Available commands:', got: %s", output)
	}
	if !strings.Contains(output, "/help") {
		t.Error("expected help output to list /help")
	}
	if !strings.Contains(output, "/clear") {
		t.Error("expected help output to list /clear")
	}
	if !strings.Contains(output, "/model") {
		t.Error("expected help output to list /model")
	}
	if !strings.Contains(output, "/compact") {
		t.Error("expected help output to list /compact")
	}
}

func TestBuiltin_Clear(t *testing.T) {
	mem := agent.NewInMemoryStore()
	_ = mem.Save(context.Background(), "s1", []llm.Message{llm.User("old")})

	reg := agent.NewCommandRegistry()
	reg.RegisterBuiltins()

	a := agent.New(agent.Config{
		Provider:  newMockProvider(textResponse("unused")),
		Memory:    mem,
		SessionID: "s1",
		Commands:  reg,
	})

	result, err := a.Run(context.Background(), []llm.Message{llm.User("/clear")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.StopReason != agent.StopCommand {
		t.Errorf("StopReason = %q, want %q", result.StopReason, agent.StopCommand)
	}
	if !strings.Contains(result.FinalMessage.Text(), "cleared") {
		t.Errorf("expected 'cleared' in output, got: %s", result.FinalMessage.Text())
	}

	// Verify memory was actually cleared.
	msgs, _ := mem.Load(context.Background(), "s1")
	if len(msgs) != 0 {
		t.Errorf("expected 0 messages after clear, got %d", len(msgs))
	}
}

func TestBuiltin_Clear_NoMemory(t *testing.T) {
	reg := agent.NewCommandRegistry()
	reg.RegisterBuiltins()

	a := agent.New(agent.Config{
		Provider: newMockProvider(textResponse("unused")),
		Commands: reg,
	})

	result, err := a.Run(context.Background(), []llm.Message{llm.User("/clear")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should return error message in output (not a Go error).
	if !strings.Contains(result.FinalMessage.Text(), "no memory") {
		t.Errorf("expected 'no memory' error in output, got: %s", result.FinalMessage.Text())
	}
}

func TestBuiltin_Model_Switch(t *testing.T) {
	reg := agent.NewCommandRegistry()
	reg.RegisterBuiltins()

	var switched bool
	hooks := hook.NewRegistry()
	hooks.On(agent.EventModelSwitched, func(e hook.Event) hook.Result {
		ms := e.(agent.ModelSwitched)
		if ms.NewModel == "gpt-4o" && ms.PreviousModel == "" {
			switched = true
		}
		return hook.Continue()
	})

	a := agent.New(agent.Config{
		Provider: newMockProvider(textResponse("unused")),
		Commands: reg,
		Hooks:    hooks,
	})

	result, err := a.Run(context.Background(), []llm.Message{llm.User("/model gpt-4o")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.StopReason != agent.StopCommand {
		t.Errorf("StopReason = %q, want %q", result.StopReason, agent.StopCommand)
	}
	if !strings.Contains(result.FinalMessage.Text(), "gpt-4o") {
		t.Errorf("expected 'gpt-4o' in output, got: %s", result.FinalMessage.Text())
	}
	if !switched {
		t.Error("expected ModelSwitched hook to fire")
	}
	if a.GetModel() != "gpt-4o" {
		t.Errorf("GetModel() = %q, want %q", a.GetModel(), "gpt-4o")
	}
}

func TestBuiltin_Model_ShowCurrent(t *testing.T) {
	reg := agent.NewCommandRegistry()
	reg.RegisterBuiltins()

	a := agent.New(agent.Config{
		Provider: newMockProvider(textResponse("unused")),
		Commands: reg,
	})

	result, err := a.Run(context.Background(), []llm.Message{llm.User("/model")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result.FinalMessage.Text(), "(default)") {
		t.Errorf("expected '(default)' in output, got: %s", result.FinalMessage.Text())
	}
}

func TestBuiltin_Compact(t *testing.T) {
	mem := agent.NewInMemoryStore()
	msgs := []llm.Message{
		llm.User("1"), llm.User("2"), llm.User("3"),
		llm.User("4"), llm.User("5"),
	}
	_ = mem.Save(context.Background(), "s1", msgs)

	reg := agent.NewCommandRegistry()
	reg.RegisterBuiltins()

	a := agent.New(agent.Config{
		Provider:        newMockProvider(textResponse("unused")),
		Memory:          mem,
		SessionID:       "s1",
		Commands:        reg,
		ContextStrategy: agent.TruncateStrategy{KeepLast: 2},
	})

	result, err := a.Run(context.Background(), []llm.Message{llm.User("/compact")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result.FinalMessage.Text(), "Compacted 5 messages to 2") {
		t.Errorf("unexpected output: %s", result.FinalMessage.Text())
	}

	// Verify memory was compacted.
	remaining, _ := mem.Load(context.Background(), "s1")
	if len(remaining) != 2 {
		t.Errorf("expected 2 messages after compact, got %d", len(remaining))
	}
}

// --- Integration Tests ---

func TestRun_CommandBypassesLLM(t *testing.T) {
	// Provider should NOT be called — command is intercepted first.
	provider := newMockProvider() // no responses — would error if called

	reg := agent.NewCommandRegistry()
	reg.Register(agent.Command{
		Name:        "echo",
		Description: "Echo input",
		Handler: func(_ context.Context, args string, _ *agent.Agent) (string, error) {
			return "echo: " + args, nil
		},
	})

	a := agent.New(agent.Config{
		Provider: provider,
		Commands: reg,
	})

	result, err := a.Run(context.Background(), []llm.Message{llm.User("/echo hello world")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.StopReason != agent.StopCommand {
		t.Errorf("StopReason = %q, want %q", result.StopReason, agent.StopCommand)
	}
	if result.FinalMessage.Text() != "echo: hello world" {
		t.Errorf("output = %q, want %q", result.FinalMessage.Text(), "echo: hello world")
	}
	if result.TurnCount != 0 {
		t.Errorf("TurnCount = %d, want 0", result.TurnCount)
	}
}

func TestRun_NonCommandPassesToLLM(t *testing.T) {
	provider := newMockProvider(textResponse("Hello!"))

	reg := agent.NewCommandRegistry()
	reg.RegisterBuiltins()

	a := agent.New(agent.Config{
		Provider: provider,
		Commands: reg,
	})

	result, err := a.Run(context.Background(), []llm.Message{llm.User("Hi there")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.StopReason != agent.StopEndTurn {
		t.Errorf("StopReason = %q, want %q", result.StopReason, agent.StopEndTurn)
	}
	if result.FinalMessage.Text() != "Hello!" {
		t.Errorf("output = %q, want %q", result.FinalMessage.Text(), "Hello!")
	}
}

func TestStream_CommandEmitsCompleteEvent(t *testing.T) {
	reg := agent.NewCommandRegistry()
	reg.Register(agent.Command{
		Name: "ping",
		Handler: func(_ context.Context, _ string, _ *agent.Agent) (string, error) {
			return "pong", nil
		},
	})

	a := agent.New(agent.Config{
		Provider: newMockProvider(),
		Commands: reg,
	})

	ch, err := a.Stream(context.Background(), []llm.Message{llm.User("/ping")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var events []agent.Event
	for e := range ch {
		events = append(events, e)
	}

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	ce, ok := events[0].(agent.CompleteEvent)
	if !ok {
		t.Fatalf("expected CompleteEvent, got %T", events[0])
	}
	if ce.Result.StopReason != agent.StopCommand {
		t.Errorf("StopReason = %q, want %q", ce.Result.StopReason, agent.StopCommand)
	}
	if ce.Result.FinalMessage.Text() != "pong" {
		t.Errorf("output = %q, want %q", ce.Result.FinalMessage.Text(), "pong")
	}
}

func TestRun_UnregisteredSlashNotIntercepted(t *testing.T) {
	// An unregistered /foo should pass through to the LLM.
	provider := newMockProvider(textResponse("I don't understand /foo"))

	reg := agent.NewCommandRegistry()
	reg.RegisterBuiltins()

	a := agent.New(agent.Config{
		Provider: provider,
		Commands: reg,
	})

	result, err := a.Run(context.Background(), []llm.Message{llm.User("/foo")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.StopReason != agent.StopEndTurn {
		t.Errorf("StopReason = %q, want %q", result.StopReason, agent.StopEndTurn)
	}
}
