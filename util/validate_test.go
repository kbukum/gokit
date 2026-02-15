package util

import (
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestValidateUUID(t *testing.T) {
	validUUID := "550e8400-e29b-41d4-a716-446655440000"
	id, err := ValidateUUID("user_id", validUUID)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if id.String() != validUUID {
		t.Errorf("expected %s, got %s", validUUID, id.String())
	}
}

func TestValidateUUIDEmpty(t *testing.T) {
	_, err := ValidateUUID("user_id", "")
	if err == nil {
		t.Fatal("expected error for empty UUID")
	}
	if !strings.Contains(err.Error(), "cannot be empty") {
		t.Errorf("expected 'cannot be empty' in error, got %q", err.Error())
	}
}

func TestValidateUUIDWhitespace(t *testing.T) {
	_, err := ValidateUUID("user_id", "   ")
	if err == nil {
		t.Fatal("expected error for whitespace-only UUID")
	}
}

func TestValidateUUIDInvalid(t *testing.T) {
	_, err := ValidateUUID("user_id", "not-a-uuid")
	if err == nil {
		t.Fatal("expected error for invalid UUID")
	}
	if !strings.Contains(err.Error(), "invalid UUID") {
		t.Errorf("expected 'invalid UUID' in error, got %q", err.Error())
	}
}

func TestValidateUUIDTrimsWhitespace(t *testing.T) {
	validUUID := "550e8400-e29b-41d4-a716-446655440000"
	id, err := ValidateUUID("id", "  "+validUUID+"  ")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if id != uuid.MustParse(validUUID) {
		t.Errorf("expected trimmed UUID to parse correctly")
	}
}

func TestValidateNonEmpty(t *testing.T) {
	tests := []struct {
		name    string
		field   string
		value   string
		wantErr bool
	}{
		{"valid", "name", "John", false},
		{"empty", "name", "", true},
		{"whitespace only", "name", "   ", true},
		{"with whitespace padding", "name", " John ", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateNonEmpty(tc.field, tc.value)
			if (err != nil) != tc.wantErr {
				t.Errorf("ValidateNonEmpty(%q, %q) error = %v, wantErr %v", tc.field, tc.value, err, tc.wantErr)
			}
			if err != nil && !strings.Contains(err.Error(), tc.field) {
				t.Errorf("error should contain field name %q, got %q", tc.field, err.Error())
			}
		})
	}
}
