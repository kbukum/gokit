package logger

import (
	"time"

	"github.com/rs/zerolog"
)

// NewSampler creates a zerolog sampler from SamplingConfig.
// It uses a BurstSampler that allows an initial burst of messages per period,
// then falls back to sampling every Nth message thereafter.
func NewSampler(cfg SamplingConfig) zerolog.Sampler {
	return &zerolog.BurstSampler{
		Burst:       uint32(cfg.InitialRate),
		Period:      time.Second,
		NextSampler: &zerolog.BasicSampler{N: uint32(cfg.ThereafterRate)},
	}
}
