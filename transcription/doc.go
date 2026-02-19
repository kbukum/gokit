// Package transcription defines the provider interface and common types
// for interacting with speech-to-text backends.
//
// It follows gokit's provider pattern with a pluggable registry for
// runtime-selectable backends.
//
// # Backends
//
//   - transcription/whisper: OpenAI Whisper speech-to-text
//
// # Usage
//
//	reg := transcription.NewRegistry()
//	reg.Register("whisper", whisperProvider)
//	result, err := reg.Get("whisper").Transcribe(ctx, req)
package transcription
