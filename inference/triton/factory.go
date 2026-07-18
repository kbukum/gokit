package triton

import (
	"encoding/json"
	"fmt"

	"github.com/kbukum/gokit/inference"
)

// Kind is the registry key for the Triton KServe v2 HTTP adapter.
const Kind = "triton"

// Factory builds a Triton provider from JSON config.
func Factory(config json.RawMessage) (inference.Inference, error) {
	var cfg Config
	if len(config) > 0 {
		if err := json.Unmarshal(config, &cfg); err != nil {
			return nil, fmt.Errorf("decode triton adapter config: %w", err)
		}
	}
	return NewProvider(cfg)
}

// Register adds the Triton adapter factory to reg.
// It performs no side effects beyond the supplied registry.
func Register(reg *inference.Registry) error {
	return reg.Register(Kind, Factory)
}
