package llm

import "github.com/kbukum/gokit/component"

// Compile-time guarantees: AdapterProvider implements component.Component
// per locked decision D12 (NATIVE COMPONENT).
var _ component.Component = (*AdapterProvider)(nil)
