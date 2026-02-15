// Package diarization defines the diarization provider interface and common
// types for interacting with speaker diarization backends.
package diarization

import (
	"context"

	"github.com/skillsenselab/gokit/provider"
)

// Provider is the interface that diarization backends must implement.
type Provider interface {
	provider.Provider // embeds Name() and IsAvailable()

	// Diarize sends audio for speaker diarization and returns the result.
	Diarize(ctx context.Context, req DiarizationRequest) (*DiarizationResponse, error)
}
