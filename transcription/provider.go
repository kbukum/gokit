package transcription

import (
	"context"

	"github.com/kbukum/gokit/provider"
)

// Provider is the interface that transcription backends must implement.
type Provider interface {
	provider.Provider // embeds Name() and IsAvailable()

	// Transcribe sends audio for transcription and returns the result.
	Transcribe(ctx context.Context, req TranscriptionRequest) (*TranscriptionResponse, error)
}
