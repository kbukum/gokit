package util

import (
	"strings"
	"testing"

	"github.com/google/uuid"
)

// ── ValidateUUID extended ───────────────────────────────────────────────

func TestValidateUUID_V4(t *testing.T) {
	id := uuid.New()
	parsed, err := ValidateUUID("test_id", id.String())
	if err != nil {
		t.Fatalf("expected no error for valid v4 UUID, got %v", err)
	}
	if parsed != id {
		t.Errorf("expected %s, got %s", id, parsed)
	}
}

func TestValidateUUID_UpperCase(t *testing.T) {
	upper := "550E8400-E29B-41D4-A716-446655440000"
	_, err := ValidateUUID("id", upper)
	if err != nil {
		t.Fatalf("expected no error for uppercase UUID, got %v", err)
	}
}

func TestValidateUUID_NilUUID(t *testing.T) {
	nilUUID := "00000000-0000-0000-0000-000000000000"
	id, err := ValidateUUID("id", nilUUID)
	if err != nil {
		t.Fatalf("expected no error for nil UUID, got %v", err)
	}
	if id != uuid.Nil {
		t.Errorf("expected nil UUID, got %s", id)
	}
}

func TestValidateUUID_FieldNameInError(t *testing.T) {
	_, err := ValidateUUID("my_custom_field", "not-a-uuid")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "my_custom_field") {
		t.Errorf("error should contain field name 'my_custom_field', got %q", err.Error())
	}
}

func TestValidateUUID_TruncatedUUID(t *testing.T) {
	_, err := ValidateUUID("id", "550e8400-e29b-41d4-a716")
	if err == nil {
		t.Fatal("expected error for truncated UUID")
	}
}

func TestValidateUUID_ExtraChars(t *testing.T) {
	_, err := ValidateUUID("id", "550e8400-e29b-41d4-a716-446655440000-extra")
	if err == nil {
		t.Fatal("expected error for UUID with extra characters")
	}
}

func TestValidateUUID_TabsAndNewlines(t *testing.T) {
	validUUID := "550e8400-e29b-41d4-a716-446655440000"
	// Only leading/trailing whitespace is trimmed — tabs should work if they're at edges
	id, err := ValidateUUID("id", "\t"+validUUID+"\n")
	if err != nil {
		t.Fatalf("expected no error for UUID with tab/newline padding, got %v", err)
	}
	if id.String() != validUUID {
		t.Errorf("expected %s, got %s", validUUID, id.String())
	}
}

// ── ValidateNonEmpty extended ───────────────────────────────────────────

func TestValidateNonEmpty_SpecialChars(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{"special characters", "!@#$%^&*()", false},
		{"unicode text", "你好", false},
		{"emoji", "🔥🚀", false},
		{"newline only", "\n", true},
		{"tab only", "\t", true},
		{"mixed whitespace", " \t\n\r ", true},
		{"null character", "\x00", false}, // null char is not whitespace
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateNonEmpty("field", tc.value)
			if (err != nil) != tc.wantErr {
				t.Errorf("ValidateNonEmpty(%q) error = %v, wantErr %v", tc.value, err, tc.wantErr)
			}
		})
	}
}

func TestValidateNonEmpty_VeryLongString(t *testing.T) {
	long := strings.Repeat("a", 100000)
	err := ValidateNonEmpty("field", long)
	if err != nil {
		t.Errorf("expected no error for very long string, got %v", err)
	}
}

func TestValidateNonEmpty_ErrorContainsFieldName(t *testing.T) {
	err := ValidateNonEmpty("email_address", "")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "email_address") {
		t.Errorf("error should contain field name 'email_address', got %q", err.Error())
	}
}
