// Package provider implements a generic provider framework using Go generics
// for swappable backends with runtime switching capabilities.
//
// It provides a registry for managing multiple provider implementations with
// factory-based instantiation, availability checking, and runtime selection.
//
// # Usage
//
//	reg := provider.NewRegistry[MyProvider]()
//	reg.Register("default", myFactory)
//	p, err := reg.Get("default")
//
// This package is used by gokit's provider-pattern modules (llm, transcription,
// diarization) for pluggable backend support.
package provider
