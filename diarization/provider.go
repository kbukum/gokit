package diarization

import (
	"context"

	"github.com/kbukum/gokit/provider"
)

// Provider is the interface that diarization backends must implement.
type Provider interface {
	provider.Provider // embeds Name() and IsAvailable()

	// Diarize sends audio for speaker diarization and returns the result.
	Diarize(ctx context.Context, req DiarizationRequest) (*DiarizationResponse, error)
}
