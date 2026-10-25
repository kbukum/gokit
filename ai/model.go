package ai

import "fmt"

// Provider identifies a model provider. It is string-backed so callers can use well-known constants
// or provider-specific values.
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
	ProviderDMR         Provider = "dmr"
	ProviderLMStudio    Provider = "lmstudio"
	// ProviderOpenAICompat is any server speaking the OpenAI API whose engine is not known.
	ProviderOpenAICompat Provider = "openai_compat"
	ProviderTriton       Provider = "triton"
	ProviderVLLM         Provider = "vllm"
	ProviderTGI          Provider = "tgi"
	ProviderCustom       Provider = "custom"
)

// Source names where a descriptor field came from. The zero Source means nobody stated the field, so the field is
// unknown. Applications may add their own sources, such as an operator's catalog or an admin edit.
type Source string

// Well-known sources.
const (
	// SourceRuntime is what the serving runtime reported about the model.
	SourceRuntime Source = "runtime"
	// SourceDeclared is what code or configuration declares, such as a provider adapter's fixed capabilities.
	SourceDeclared Source = "declared"
	// SourceProbe is what a test request against the model observed.
	SourceProbe Source = "probe"
)

// Support is a tri-state capability answer. The zero value is unknown, which is never treated as supported.
type Support uint8

// Support values.
const (
	SupportUnknown Support = iota
	SupportYes
	SupportNo
)

var supportNames = [...]string{SupportUnknown: "unknown", SupportYes: "supported", SupportNo: "unsupported"}

// String returns the support's wire name.
func (s Support) String() string {
	if int(s) < len(supportNames) {
		return supportNames[s]
	}
	return fmt.Sprintf("support(%d)", uint8(s))
}

// MarshalText encodes the support's name.
func (s Support) MarshalText() ([]byte, error) {
	if int(s) >= len(supportNames) {
		return nil, fmt.Errorf("ai: undeclared support %d", uint8(s))
	}
	return []byte(supportNames[s]), nil
}

// UnmarshalText decodes a support name and refuses any other value.
func (s *Support) UnmarshalText(b []byte) error {
	for i, name := range supportNames {
		if string(b) == name {
			*s = Support(i)
			return nil
		}
	}
	return fmt.Errorf("ai: undeclared support %q", b)
}

// Fact is one capability answer and its source. A fact without a source is unknown.
type Fact struct {
	Support Support `json:"support"`
	Source  Source  `json:"source"`
}

// Supported returns a fact stating the capability is supported, according to src.
func Supported(src Source) Fact { return Fact{Support: SupportYes, Source: src} }

// Unsupported returns a fact stating the capability is not supported, according to src.
func Unsupported(src Source) Fact { return Fact{Support: SupportNo, Source: src} }

// Known reports whether someone stated the fact.
func (f Fact) Known() bool { return f.Source != "" && f.Support != SupportUnknown }

// Supported reports whether the fact is known and positive. Unknown is never supported.
func (f Fact) Supported() bool { return f.Known() && f.Support == SupportYes }

// IsZero reports whether the fact is unknown, so JSON omits it with omitzero.
func (f Fact) IsZero() bool { return !f.Known() }

// Known is a descriptor value and its source. A value without a source is unknown.
type Known[T any] struct {
	Value  T      `json:"value"`
	Source Source `json:"source"`
}

// Stated returns v as stated by src.
func Stated[T any](v T, src Source) Known[T] { return Known[T]{Value: v, Source: src} }

// Known reports whether someone stated the value.
func (k Known[T]) Known() bool { return k.Source != "" }

// IsZero reports whether the value is unknown, so JSON omits it with omitzero.
func (k Known[T]) IsZero() bool { return !k.Known() }

// Task is what a model does.
type Task string

// Tasks.
const (
	TaskTextGeneration  Task = "text-generation"
	TaskEmbedding       Task = "embedding"
	TaskImageGeneration Task = "image-generation"
	TaskTranscription   Task = "transcription"
)

// Modality is a kind of model input or output.
type Modality string

// Modalities.
const (
	ModalityText      Modality = "text"
	ModalityImage     Modality = "image"
	ModalityAudio     Modality = "audio"
	ModalityEmbedding Modality = "embedding"
)

// Variant describes which build of a model family this is. Empty fields are unknown.
type Variant struct {
	Parameters   string `json:"parameters,omitempty"`
	Quantization string `json:"quantization,omitempty"`
	Format       string `json:"format,omitempty"`
	SizeBytes    int64  `json:"size_bytes,omitempty"`
}

// Capabilities are a model's tri-state features and token limits. Each field records its source; an unknown
// field is never treated as supported.
type Capabilities struct {
	Streaming Fact `json:"streaming,omitzero"`
	Vision    Fact `json:"vision,omitzero"`
	Audio     Fact `json:"audio,omitzero"`
	ToolUse   Fact `json:"tool_use,omitzero"`
	JSONMode  Fact `json:"json_mode,omitzero"`
	Reasoning Fact `json:"reasoning,omitzero"`
	// ContextLength is the model's context window in tokens, input and output together.
	ContextLength   Known[int] `json:"context_length,omitzero"`
	MaxOutputTokens Known[int] `json:"max_output_tokens,omitzero"`
}

// Model describes a concrete model: identity, what it does, its variant and capabilities. Every descriptive field
// is unknown until a source states it.
type Model struct {
	Name     string   `json:"name"`
	Provider Provider `json:"provider"`
	Version  string   `json:"version,omitempty"`
	// Digest identifies the model's content, such as "sha256:…", when the runtime reports one.
	Digest  string            `json:"digest,omitempty"`
	Family  Known[string]     `json:"family,omitzero"`
	Variant Variant           `json:"variant,omitzero"`
	Task    Known[Task]       `json:"task,omitzero"`
	Inputs  Known[[]Modality] `json:"inputs,omitzero"`
	Outputs Known[[]Modality] `json:"outputs,omitzero"`
	// Parameters names the request parameters the model accepts, such as "temperature".
	Parameters   Known[[]string] `json:"parameters,omitzero"`
	Capabilities Capabilities    `json:"capabilities,omitzero"`
}

// InputBudget returns how many input tokens fit: the context length less any stated output reservation. It returns
// 0 when the context length is unknown or leaves no room, so callers treat 0 as "no known limit".
func (c Capabilities) InputBudget() int {
	if !c.ContextLength.Known() || c.ContextLength.Value <= 0 {
		return 0
	}
	budget := c.ContextLength.Value
	if c.MaxOutputTokens.Known() && c.MaxOutputTokens.Value > 0 {
		budget -= c.MaxOutputTokens.Value
	}
	return max(budget, 0)
}
