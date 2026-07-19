package agent

import (
	"time"

	"github.com/kbukum/gokit/agent/memory"
	"github.com/kbukum/gokit/ai"
	"github.com/kbukum/gokit/ai/prompt"
	"github.com/kbukum/gokit/hook"
	"github.com/kbukum/gokit/llm"
	"github.com/kbukum/gokit/resilience"
	"github.com/kbukum/gokit/tool"
)

const (
	defaultMaxTurns        = 10
	defaultMaxTokens       = 100_000
	defaultMaxToolCalls    = 50
	defaultToolConcurrency = 4
	defaultStreamBuffer    = 16
)

type Config struct {
	Provider             llm.Provider
	Tools                *tool.Registry
	ToolFormatter        tool.Formatter
	Hooks                *hook.Registry
	SystemPrompt         string
	SystemPromptTemplate *prompt.Template
	SystemPromptData     any
	Model                string
	Budget               ai.Budget
	MaxTurns             int
	MaxTokens            int
	WallClock            time.Duration
	MaxToolCalls         int
	ToolConcurrency      int
	ToolTimeout          time.Duration
	Policy               *resilience.Policy
	Compaction           memory.Policy
	Commands             *CommandRegistry
	Store                memory.Store
	SessionID            string
	StreamBuffer         int
}

func New(config Config) *Agent {
	if config.Budget.MaxTokens > 0 {
		config.MaxTokens = config.Budget.MaxTokens
	}
	if config.Budget.MaxCalls > 0 {
		config.MaxToolCalls = config.Budget.MaxCalls
	}
	if config.Budget.WallClock > 0 {
		config.WallClock = config.Budget.WallClock
	}
	if config.MaxTurns <= 0 {
		config.MaxTurns = defaultMaxTurns
	}
	if config.MaxTokens <= 0 {
		config.MaxTokens = defaultMaxTokens
	}
	if config.WallClock <= 0 {
		config.WallClock = 60 * time.Second
	}
	if config.MaxToolCalls <= 0 {
		config.MaxToolCalls = defaultMaxToolCalls
	}
	if config.ToolConcurrency <= 0 {
		config.ToolConcurrency = defaultToolConcurrency
	}
	if config.ToolTimeout <= 0 {
		config.ToolTimeout = 30 * time.Second
	}
	if config.Compaction == nil {
		config.Compaction = memory.RingBuffer{KeepLast: 20}
	}
	if config.StreamBuffer <= 0 {
		config.StreamBuffer = defaultStreamBuffer
	}
	return &Agent{config: config}
}
