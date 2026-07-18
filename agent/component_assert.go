package agent

import "github.com/kbukum/gokit/component"

// Compile-time guarantee:
// Agent implements component.Component per locked decision D12 (NATIVE COMPONENT).
var _ component.Component = (*Agent)(nil)
