package ai

import "errors"

var (
	ErrRateLimited           = errors.New("ai: rate limited")
	ErrContextLengthExceeded = errors.New("ai: context length exceeded")
	ErrContentFilter         = errors.New("ai: content filtered")
	ErrModelOverloaded       = errors.New("ai: model overloaded")
	ErrModelNotFound         = errors.New("ai: model not found")
	ErrInvalidRequest        = errors.New("ai: invalid request")
)
