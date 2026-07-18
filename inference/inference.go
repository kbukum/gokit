package inference

import (
	"context"
	"encoding/json"

	"github.com/kbukum/gokit/ai"
	"github.com/kbukum/gokit/provider"
)

// Inference is the model-serving runtime interface. It is intentionally general — adapters cover text generation (vLLM, TGI), classification / regression (Triton KServe v2), embeddings (BentoML, custom), image / audio inference (custom REST), and arbitrary tensor protocols.
//
// Inference is NOT chat completion. Conversational LLM surface lives in the llm module; inference sits one layer below as the serving runtime.
//
// Inference natively embeds [provider.RequestResponse] so serving adapters plug into canonical provider consumers (pipeline, dag, chain) without a shim.
type Inference interface {
	provider.RequestResponse[PredictRequest, PredictResponse]
	Predict(ctx context.Context, req PredictRequest) (PredictResponse, error)
	Descriptor() Descriptor
}

// StreamingInference is implemented by serving runtimes that emit canonical ai stream events.
type StreamingInference interface {
	Inference
	PredictStream(ctx context.Context, req PredictRequest) (<-chan ai.StreamEvent, error)
}

// Descriptor documents a serving runtime adapter.
//
// Capabilities advertise lean adapter hints (streaming, batching, tool-calls) to consumers and observability without coupling the inference layer to the richer permission envelope owned by package tool. Available reports whether the adapter is a working backend (true) or a not-yet-live skeleton (false).
type Descriptor struct {
	Name            string          `json:"name"`
	Description     string          `json:"description"`
	ServingProtocol string          `json:"serving_protocol"`
	Capabilities    CapabilityHints `json:"capabilities,omitempty"`
	Available       bool            `json:"available"`
}

// PredictRequest carries arbitrary inputs. Inputs is the canonical payload (a map of named inputs to typed values supporting tensors, strings, byte blobs, and nested structs); Parameters carries adapter-specific tuning (max_new_tokens, temperature, top_k, etc.).
type PredictRequest struct {
	RequestID    string            `json:"request_id,omitempty"`
	ModelName    string            `json:"model_name"`
	ModelVersion string            `json:"model_version,omitempty"`
	Inputs       map[string]Value  `json:"inputs"`
	Parameters   map[string]any    `json:"parameters,omitempty"`
	Options      map[string]any    `json:"options,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

// PredictResponse is the normalized response. Outputs is the canonical payload; Usage is optional token / compute accounting.
type PredictResponse struct {
	Outputs  map[string]Value  `json:"outputs"`
	Model    ai.Model          `json:"model"`
	Status   PredictStatus     `json:"status"`
	Usage    Usage             `json:"usage,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// PredictStatus reports normalized serving status.
type PredictStatus string

const (
	StatusSuccess        PredictStatus = "success"
	StatusPartialSuccess PredictStatus = "partial_success"
	StatusError          PredictStatus = "error"
)

// Value is a typed serving input/output. Use TextValue, BytesValue, TensorValue, and JSONValue to build values; the Kind field is set by the constructor.
type Value struct {
	Kind   ValueKind       `json:"kind"`
	Text   string          `json:"text,omitempty"`
	Bytes  []byte          `json:"bytes,omitempty"`
	Tensor *Tensor         `json:"tensor,omitempty"`
	JSON   json.RawMessage `json:"json,omitempty"`
}

// ValueKind identifies the active Value payload field.
type ValueKind string

const (
	KindText   ValueKind = "text"
	KindBytes  ValueKind = "bytes"
	KindTensor ValueKind = "tensor"
	KindJSON   ValueKind = "json"
)

// Tensor is a typed numeric tensor (KServe v2 / Triton style).
type Tensor struct {
	DType string  `json:"dtype"`
	Shape []int64 `json:"shape"`
	Data  any     `json:"data"`
}

// Usage reports token and compute accounting when the serving runtime exposes it.
type Usage = ai.Usage

// TextValue creates a text Value.
func TextValue(text string) Value { return Value{Kind: KindText, Text: text} }

// BytesValue creates a byte-blob Value and copies the input slice.
func BytesValue(data []byte) Value {
	copied := append([]byte(nil), data...)
	return Value{Kind: KindBytes, Bytes: copied}
}

// TensorValue creates a tensor Value.
func TensorValue(t Tensor) Value { return Value{Kind: KindTensor, Tensor: &t} }

// JSONValue creates a structured JSON Value and copies the input bytes.
func JSONValue(raw json.RawMessage) Value {
	copied := append(json.RawMessage(nil), raw...)
	return Value{Kind: KindJSON, JSON: copied}
}
