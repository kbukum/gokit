// Package transcription defines the transcription provider interface and common
// types for interacting with speech-to-text backends.
package transcription

import (
	"context"

	"github.com/skillsenselab/gokit/provider"
)

// Provider is the interface that transcription backends must implement.
type Provider interface {
	provider.Provider // embeds Name() and IsAvailable()

	// Transcribe sends audio for transcription and returns the result.
	Transcribe(ctx context.Context, req TranscriptionRequest) (*TranscriptionResponse, error)
}
