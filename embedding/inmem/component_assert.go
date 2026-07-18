package inmem

import "github.com/kbukum/gokit/component"

// Compile-time guarantee: Provider implements component.Component per locked decision D12 (NATIVE COMPONENT).
var _ component.Component = (*Provider)(nil)
