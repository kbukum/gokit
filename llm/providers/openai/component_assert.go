package openai

import "github.com/kbukum/gokit/component"

// Compile-time guarantee: EmbeddingProvider implements component.Component
// per locked decision D12 (NATIVE COMPONENT).
var _ component.Component = (*EmbeddingProvider)(nil)
