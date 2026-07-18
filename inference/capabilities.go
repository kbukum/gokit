package inference

// CapabilityHints describes optional inference-adapter hints for downstream policy
// and observability.
// Mirrors a subset of tool.Envelope at the AI layer without coupling the inference layer to the tool layer.
type CapabilityHints struct {
	SupportsStreaming bool `json:"supports_streaming,omitempty"`
	SupportsBatching  bool `json:"supports_batching,omitempty"`
	MaxBatchSize      int  `json:"max_batch_size,omitempty"`
	SupportsToolCalls bool `json:"supports_tool_calls,omitempty"`
}
