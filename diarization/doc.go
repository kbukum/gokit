// Package diarization defines the provider interface and common types
// for interacting with speaker diarization backends.
//
// It follows gokit's provider pattern with a pluggable registry for
// runtime-selectable backends.
//
// # Backends
//
//   - diarization/pyannote: Pyannote-based speaker diarization
//
// # Usage
//
//	reg := diarization.NewRegistry()
//	reg.Register("pyannote", pyannoteProvider)
//	result, err := reg.Get("pyannote").Diarize(ctx, req)
package diarization
