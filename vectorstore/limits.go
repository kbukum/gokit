package vectorstore

import (
	"encoding/json"
	"fmt"
)

// Default safety bounds for vector operations. They cap resource use at every trust
// boundary so a single request cannot exhaust memory or overwhelm a backend. Values
// match the cross-kit defaults.
const (
	// DefaultMaxSearchLimit bounds the number of results a single search may request.
	DefaultMaxSearchLimit = 1000
	// DefaultMaxVectorDimensions bounds a collection's vector dimensionality.
	DefaultMaxVectorDimensions = 32768
	// DefaultMaxPayloadFields bounds the number of fields on a point payload.
	DefaultMaxPayloadFields = 128
	// DefaultMaxPayloadBytes bounds the approximate serialized size of a payload.
	DefaultMaxPayloadBytes = 64 * 1024
	// DefaultMaxFilterConditions bounds the number of must-match filter conditions.
	DefaultMaxFilterConditions = 32
)

// VectorStoreLimits captures the safety bounds shared by every vectorstore backend.
// A zero-valued field adopts the corresponding default via ApplyDefaults.
type VectorStoreLimits struct {
	MaxSearchLimit      int `mapstructure:"max_search_limit" json:"max_search_limit" yaml:"max_search_limit"`
	MaxVectorDimensions int `mapstructure:"max_vector_dimensions" json:"max_vector_dimensions" yaml:"max_vector_dimensions"`
	MaxPayloadFields    int `mapstructure:"max_payload_fields" json:"max_payload_fields" yaml:"max_payload_fields"`
	MaxPayloadBytes     int `mapstructure:"max_payload_bytes" json:"max_payload_bytes" yaml:"max_payload_bytes"`
	MaxFilterConditions int `mapstructure:"max_filter_conditions" json:"max_filter_conditions" yaml:"max_filter_conditions"`
}

// DefaultLimits returns the default safety bounds.
func DefaultLimits() VectorStoreLimits {
	return VectorStoreLimits{
		MaxSearchLimit:      DefaultMaxSearchLimit,
		MaxVectorDimensions: DefaultMaxVectorDimensions,
		MaxPayloadFields:    DefaultMaxPayloadFields,
		MaxPayloadBytes:     DefaultMaxPayloadBytes,
		MaxFilterConditions: DefaultMaxFilterConditions,
	}
}

// ApplyDefaults fills any zero-valued bound with its default.
func (l *VectorStoreLimits) ApplyDefaults() {
	d := DefaultLimits()
	if l.MaxSearchLimit == 0 {
		l.MaxSearchLimit = d.MaxSearchLimit
	}
	if l.MaxVectorDimensions == 0 {
		l.MaxVectorDimensions = d.MaxVectorDimensions
	}
	if l.MaxPayloadFields == 0 {
		l.MaxPayloadFields = d.MaxPayloadFields
	}
	if l.MaxPayloadBytes == 0 {
		l.MaxPayloadBytes = d.MaxPayloadBytes
	}
	if l.MaxFilterConditions == 0 {
		l.MaxFilterConditions = d.MaxFilterConditions
	}
}

// LimitExceededError reports that a value violated a configured safety bound.
type LimitExceededError struct {
	Limit  string
	Max    int
	Actual int
}

func (e *LimitExceededError) Error() string {
	return fmt.Sprintf("vectorstore: %s %d exceeds limit %d", e.Limit, e.Actual, e.Max)
}

// ValidateDimensions checks that a collection's dimensionality is within bounds.
func (l VectorStoreLimits) ValidateDimensions(dimensions int) error {
	if dimensions <= 0 {
		return fmt.Errorf("vectorstore: dimensions must be positive, got %d", dimensions)
	}
	if dimensions > l.MaxVectorDimensions {
		return &LimitExceededError{Limit: "vector dimensions", Max: l.MaxVectorDimensions, Actual: dimensions}
	}
	return nil
}

// ValidateSearchLimit checks a search limit. A zero limit is allowed and yields an
// empty result; a negative limit is rejected.
func (l VectorStoreLimits) ValidateSearchLimit(limit int) error {
	if limit < 0 {
		return fmt.Errorf("vectorstore: search limit must be non-negative, got %d", limit)
	}
	if limit > l.MaxSearchLimit {
		return &LimitExceededError{Limit: "search limit", Max: l.MaxSearchLimit, Actual: limit}
	}
	return nil
}

// Validate checks a payload's field count and approximate serialized size against the
// limits. A nil payload is valid.
func (p *PointPayload) Validate(l VectorStoreLimits) error {
	if p == nil || len(p.Fields) == 0 {
		return nil
	}
	if len(p.Fields) > l.MaxPayloadFields {
		return &LimitExceededError{Limit: "payload fields", Max: l.MaxPayloadFields, Actual: len(p.Fields)}
	}
	encoded, err := json.Marshal(p.Fields)
	if err != nil {
		return fmt.Errorf("vectorstore: encode payload for size check: %w", err)
	}
	if len(encoded) > l.MaxPayloadBytes {
		return &LimitExceededError{Limit: "payload bytes", Max: l.MaxPayloadBytes, Actual: len(encoded)}
	}
	return nil
}

// Validate checks a filter's condition count against the limits. A nil filter is valid.
func (f *SearchFilter) Validate(l VectorStoreLimits) error {
	if f == nil || len(f.Must) == 0 {
		return nil
	}
	if len(f.Must) > l.MaxFilterConditions {
		return &LimitExceededError{Limit: "filter conditions", Max: l.MaxFilterConditions, Actual: len(f.Must)}
	}
	return nil
}
