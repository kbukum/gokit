package util

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// ValidateUUID validates that value is a valid UUID string and returns the parsed UUID.
func ValidateUUID(field, value string) (uuid.UUID, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return uuid.Nil, fmt.Errorf("%s cannot be empty", field)
	}
	id, err := uuid.Parse(trimmed)
	if err != nil {
		return uuid.Nil, fmt.Errorf("%s: invalid UUID format: %w", field, err)
	}
	return id, nil
}

// ValidateNonEmpty validates that value is not empty after trimming whitespace.
func ValidateNonEmpty(field, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s cannot be empty", field)
	}
	return nil
}
