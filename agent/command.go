package agent

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/kbukum/gokit/ai"
	"github.com/kbukum/gokit/ai/chat"
)

// CommandHandler executes a slash command. Returns a result string shown to the user.
type CommandHandler func(ctx context.Context, args string, agent *Agent) (string, error)

// Command defines a slash command that is handled before the LLM loop.
type Command struct {
	// Name is the slash command name (without the leading /).
	Name string
	// Description is a short help text.
	Description string
	// Usage is an optional usage pattern (e.g., "/model <name>").
	Usage string
	// Handler executes the command.
	Handler CommandHandler
}

// CommandRegistry holds registered slash commands.
type CommandRegistry struct {
	mu   sync.RWMutex
	cmds map[string]Command
}

// NewCommandRegistry creates an empty CommandRegistry.
func NewCommandRegistry() *CommandRegistry {
	return &CommandRegistry{cmds: make(map[string]Command)}
}

// Register adds a command to the registry.
func (r *CommandRegistry) Register(cmd Command) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cmds[cmd.Name] = cmd
}

// Get returns the named command if it exists.
func (r *CommandRegistry) Get(name string) (Command, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	cmd, ok := r.cmds[name]
	return cmd, ok
}

// List returns all registered commands sorted by name.
func (r *CommandRegistry) List() []Command {
	r.mu.RLock()
	defer r.mu.RUnlock()
	cmds := make([]Command, 0, len(r.cmds))
	for _, cmd := range r.cmds {
		cmds = append(cmds, cmd)
	}
	sort.Slice(cmds, func(i, j int) bool { return cmds[i].Name < cmds[j].Name })
	return cmds
}

// IsCommand returns true if the input starts with a registered slash command.
func (r *CommandRegistry) IsCommand(input string) bool {
	name, _, ok := r.ParseCommand(input)
	if !ok {
		return false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, exists := r.cmds[name]
	return exists
}

// ParseCommand extracts the command name and args from input like "/model gpt-4o".
func (r *CommandRegistry) ParseCommand(input string) (name, args string, ok bool) {
	input = strings.TrimSpace(input)
	if !strings.HasPrefix(input, "/") {
		return "", "", false
	}
	rest := input[1:]
	parts := strings.SplitN(rest, " ", 2)
	name = parts[0]
	if name == "" {
		return "", "", false
	}
	if len(parts) > 1 {
		args = strings.TrimSpace(parts[1])
	}
	return name, args, true
}

// RegisterBuiltins adds the default built-in slash commands (/help, /clear, /model, /compact).
func (r *CommandRegistry) RegisterBuiltins() {
	r.Register(Command{
		Name:        "help",
		Description: "List all available commands",
		Usage:       "/help",
		Handler:     builtinHelp,
	})
	r.Register(Command{
		Name:        "clear",
		Description: "Clear conversation memory",
		Usage:       "/clear",
		Handler:     builtinClear,
	})
	r.Register(Command{
		Name:        "model",
		Description: "Switch the model override",
		Usage:       "/model <name>",
		Handler:     builtinModel,
	})
	r.Register(Command{
		Name:        "compact",
		Description: "Force context compaction",
		Usage:       "/compact",
		Handler:     builtinCompact,
	})
}

// --- Built-in command handlers ---

func builtinHelp(_ context.Context, _ string, a *Agent) (string, error) {
	if a.config.Commands == nil {
		return "No commands registered.", nil
	}
	cmds := a.config.Commands.List()
	if len(cmds) == 0 {
		return "No commands registered.", nil
	}
	var b strings.Builder
	b.WriteString("Available commands:\n")
	for _, cmd := range cmds {
		if cmd.Usage != "" {
			fmt.Fprintf(&b, "  %-20s %s\n", cmd.Usage, cmd.Description)
		} else {
			fmt.Fprintf(&b, "  /%-19s %s\n", cmd.Name, cmd.Description)
		}
	}
	return b.String(), nil
}

func builtinClear(ctx context.Context, _ string, a *Agent) (string, error) {
	if err := a.ClearMemory(ctx); err != nil {
		return "", err
	}
	return "Conversation memory cleared.", nil
}

func builtinModel(ctx context.Context, args string, a *Agent) (string, error) {
	args = strings.TrimSpace(args)
	if args == "" {
		current := a.GetModel()
		if current == "" {
			return "Current model: (default)", nil
		}
		return fmt.Sprintf("Current model: %s", current), nil
	}
	prev := a.GetModel()
	a.SetModel(args)
	_ = a.emitHook(ctx, ModelSwitched{
		PreviousModel: prev,
		NewModel:      args,
		Reason:        "command",
	})
	return fmt.Sprintf("Model switched to %s", args), nil
}

func builtinCompact(ctx context.Context, _ string, a *Agent) (string, error) {
	if a.config.Compaction == nil {
		return "No compaction policy configured.", nil
	}
	if a.config.Store == nil || a.config.SessionID == "" {
		return "No memory configured for compaction.", nil
	}
	msgs, err := a.config.Store.Load(ctx, a.config.SessionID)
	if err != nil {
		return "", fmt.Errorf("compact: failed to load memory: %w", err)
	}
	if len(msgs) == 0 {
		return "No messages to compact.", nil
	}
	maxTokens := 0
	if a.config.Provider != nil {
		maxTokens = a.config.Provider.Capabilities().MaxInputTokens
	}
	compacted, err := a.config.Compaction.Compact(ctx, msgs, maxTokens)
	if err != nil {
		return "", fmt.Errorf("compact: %w", err)
	}
	if err := a.config.Store.Save(ctx, a.config.SessionID, compacted); err != nil {
		return "", fmt.Errorf("compact: failed to save: %w", err)
	}
	return fmt.Sprintf("Compacted %d messages to %d.", len(msgs), len(compacted)), nil
}

// --- Agent helper methods for commands ---

// SetModel updates the model override in the agent configuration.
func (a *Agent) SetModel(model string) { a.config.Model = model }

// GetModel returns the current model override.
func (a *Agent) GetModel() string { return a.config.Model }

// ClearMemory clears the conversation memory for the current session.
func (a *Agent) ClearMemory(ctx context.Context) error {
	if a.config.Store == nil {
		return fmt.Errorf("agent: no memory configured")
	}
	return a.config.Store.Clear(ctx, a.config.SessionID)
}

// extractUserText returns the text content of the last user message.
func extractUserText(msgs []chat.Message) (string, bool) {
	if len(msgs) == 0 {
		return "", false
	}
	um, ok := msgs[len(msgs)-1].(chat.UserMessage)
	if !ok {
		return "", false
	}
	for _, block := range um.Content {
		if tb, ok := block.(ai.Text); ok {
			return tb.Text, true
		}
	}
	return "", false
}

// handleCommand checks if the last message is a slash command and executes it. Returns the result
// and true if a command was handled.
func (a *Agent) handleCommand(ctx context.Context, msgs []chat.Message) (*Result, bool) {
	if a.config.Commands == nil {
		return nil, false
	}
	text, ok := extractUserText(msgs)
	if !ok {
		return nil, false
	}
	name, args, parsed := a.config.Commands.ParseCommand(text)
	if !parsed {
		return nil, false
	}
	cmd, found := a.config.Commands.Get(name)
	if !found {
		return nil, false
	}
	output, err := cmd.Handler(ctx, args, a)
	if err != nil {
		output = fmt.Sprintf("Error: %v", err)
	}
	finalMsg := chat.Assistant(output)
	return &Result{
		Messages:     append(msgs, finalMsg),
		FinalMessage: finalMsg,
		StopReason:   StopCommand,
	}, true
}
