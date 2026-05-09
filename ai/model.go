package ai

// Provider identifies a model provider. It is string-backed so callers can use
// well-known constants or provider-specific values.
type Provider string

const (
	ProviderOpenAI      Provider = "openai"
	ProviderAnthropic   Provider = "anthropic"
	ProviderGoogle      Provider = "google"
	ProviderCohere      Provider = "cohere"
	ProviderMistral     Provider = "mistral"
	ProviderMeta        Provider = "meta"
	ProviderAWSBedrock  Provider = "aws_bedrock"
	ProviderAzureOpenAI Provider = "azure_openai"
	ProviderOllama      Provider = "ollama"
	ProviderTriton      Provider = "triton"
	ProviderVLLM        Provider = "vllm"
	ProviderTGI         Provider = "tgi"
	ProviderCustom      Provider = "custom"
)

// Capabilities describes model features and token limits.
type Capabilities struct {
	Streaming       bool `json:"streaming"`
	Vision          bool `json:"vision"`
	Audio           bool `json:"audio"`
	ToolUse         bool `json:"tool_use"`
	JSONMode        bool `json:"json_mode"`
	ReasoningTokens bool `json:"reasoning_tokens"`
	MaxInputTokens  int  `json:"max_input_tokens,omitempty"`
	MaxOutputTokens int  `json:"max_output_tokens,omitempty"`
}

// Model identifies a concrete model and its capabilities.
type Model struct {
	Name         string       `json:"name"`
	Provider     Provider     `json:"provider"`
	Version      string       `json:"version,omitempty"`
	Capabilities Capabilities `json:"capabilities"`
}
