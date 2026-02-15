package validation

import (
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestValidatorRequired(t *testing.T) {
	v := New()
	v.Required("name", "John")
	if v.HasErrors() {
		t.Error("expected no errors for valid input")
	}

	v2 := New()
	v2.Required("name", "")
	if !v2.HasErrors() {
		t.Error("expected error for empty required field")
	}

	v3 := New()
	v3.Required("name", "   ")
	if !v3.HasErrors() {
		t.Error("expected error for whitespace-only required field")
	}
}

func TestValidatorRequiredUUID(t *testing.T) {
	validUUID := uuid.New().String()

	v := New()
	v.RequiredUUID("id", validUUID)
	if v.HasErrors() {
		t.Errorf("expected no errors for valid UUID, got %v", v.Errors())
	}

	v2 := New()
	v2.RequiredUUID("id", "")
	if !v2.HasErrors() {
		t.Error("expected error for empty UUID")
	}

	v3 := New()
	v3.RequiredUUID("id", "not-a-uuid")
	if !v3.HasErrors() {
		t.Error("expected error for invalid UUID")
	}

	v4 := New()
	v4.RequiredUUID("id", uuid.Nil.String())
	if !v4.HasErrors() {
		t.Error("expected error for nil UUID")
	}
}

func TestValidatorOptionalUUID(t *testing.T) {
	v := New()
	v.OptionalUUID("id", "")
	if v.HasErrors() {
		t.Error("expected no error for empty optional UUID")
	}

	v2 := New()
	v2.OptionalUUID("id", uuid.New().String())
	if v2.HasErrors() {
		t.Error("expected no error for valid optional UUID")
	}

	v3 := New()
	v3.OptionalUUID("id", "bad-uuid")
	if !v3.HasErrors() {
		t.Error("expected error for invalid optional UUID")
	}
}

func TestValidatorMaxLength(t *testing.T) {
	v := New()
	v.MaxLength("desc", "short", 10)
	if v.HasErrors() {
		t.Error("expected no error for string within max length")
	}

	v2 := New()
	v2.MaxLength("desc", "this is too long", 5)
	if !v2.HasErrors() {
		t.Error("expected error for string exceeding max length")
	}
}

func TestValidatorMinLength(t *testing.T) {
	v := New()
	v.MinLength("pass", "abcdef", 6)
	if v.HasErrors() {
		t.Error("expected no error for string meeting min length")
	}

	v2 := New()
	v2.MinLength("pass", "ab", 6)
	if !v2.HasErrors() {
		t.Error("expected error for string below min length")
	}
}

func TestValidatorRange(t *testing.T) {
	v := New()
	v.Range("age", 25, 18, 100)
	if v.HasErrors() {
		t.Error("expected no error for value in range")
	}

	v2 := New()
	v2.Range("age", 5, 18, 100)
	if !v2.HasErrors() {
		t.Error("expected error for value below range")
	}

	v3 := New()
	v3.Range("age", 101, 18, 100)
	if !v3.HasErrors() {
		t.Error("expected error for value above range")
	}
}

func TestValidatorMinMax(t *testing.T) {
	v := New()
	v.Min("count", 5, 1)
	v.Max("count", 5, 10)
	if v.HasErrors() {
		t.Error("expected no errors")
	}

	v2 := New()
	v2.Min("count", 0, 1)
	if !v2.HasErrors() {
		t.Error("expected error for value below min")
	}

	v3 := New()
	v3.Max("count", 11, 10)
	if !v3.HasErrors() {
		t.Error("expected error for value above max")
	}
}

func TestValidatorPattern(t *testing.T) {
	v := New()
	v.Pattern("code", "ABC123", `^[A-Z0-9]+$`)
	if v.HasErrors() {
		t.Error("expected no error for matching pattern")
	}

	v2 := New()
	v2.Pattern("code", "abc", `^[A-Z]+$`)
	if !v2.HasErrors() {
		t.Error("expected error for non-matching pattern")
	}

	// Empty value should be skipped
	v3 := New()
	v3.Pattern("code", "", `^[A-Z]+$`)
	if v3.HasErrors() {
		t.Error("expected no error for empty value with pattern")
	}
}

func TestValidatorOneOf(t *testing.T) {
	v := New()
	v.OneOf("status", "active", []string{"active", "inactive"})
	if v.HasErrors() {
		t.Error("expected no error for valid oneOf value")
	}

	v2 := New()
	v2.OneOf("status", "unknown", []string{"active", "inactive"})
	if !v2.HasErrors() {
		t.Error("expected error for invalid oneOf value")
	}

	// Empty should be skipped
	v3 := New()
	v3.OneOf("status", "", []string{"active"})
	if v3.HasErrors() {
		t.Error("expected no error for empty oneOf value")
	}
}

func TestValidatorCustom(t *testing.T) {
	v := New()
	v.Custom(true, "field", "should pass")
	if v.HasErrors() {
		t.Error("expected no error for true condition")
	}

	v2 := New()
	v2.Custom(false, "field", "custom error")
	if !v2.HasErrors() {
		t.Error("expected error for false condition")
	}
	if v2.Errors()[0].Message != "custom error" {
		t.Errorf("expected 'custom error', got %q", v2.Errors()[0].Message)
	}
}

func TestValidatorValidate(t *testing.T) {
	v := New()
	v.Required("name", "John")
	appErr := v.Validate()
	if appErr != nil {
		t.Error("expected nil for valid input")
	}

	v2 := New()
	v2.Required("name", "")
	v2.Required("email", "")
	appErr2 := v2.Validate()
	if appErr2 == nil {
		t.Fatal("expected error")
	}
	if appErr2.Details == nil {
		t.Fatal("expected details in error")
	}
	if !strings.Contains(appErr2.Message, "name") || !strings.Contains(appErr2.Message, "email") {
		t.Errorf("expected both fields in message, got %q", appErr2.Message)
	}
}

func TestValidatorChaining(t *testing.T) {
	v := New()
	result := v.Required("name", "John").MaxLength("name", "John", 100).Min("age", 25, 18)
	if result != v {
		t.Error("expected chaining to return same validator")
	}
	if v.HasErrors() {
		t.Error("expected no errors for valid chained validation")
	}
}

func TestStructValidateValid(t *testing.T) {
	type User struct {
		Name  string `json:"name" validate:"required"`
		Email string `json:"email" validate:"required,email"`
	}

	err := Validate(User{Name: "John", Email: "john@example.com"})
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestStructValidateInvalid(t *testing.T) {
	type User struct {
		Name  string `json:"name" validate:"required"`
		Email string `json:"email" validate:"required,email"`
	}

	err := Validate(User{Name: "", Email: "not-an-email"})
	if err == nil {
		t.Fatal("expected validation error")
	}
	errStr := err.Error()
	if !strings.Contains(errStr, "name") {
		t.Errorf("expected error to mention 'name', got %q", errStr)
	}
}

func TestStructValidateMaxMin(t *testing.T) {
	type Input struct {
		Code string `json:"code" validate:"required,min=3,max=10"`
	}

	if err := Validate(Input{Code: "abc"}); err != nil {
		t.Errorf("expected valid, got %v", err)
	}

	if err := Validate(Input{Code: "ab"}); err == nil {
		t.Error("expected error for code too short")
	}
}

func TestValidateUUIDFunc(t *testing.T) {
	validUUID := uuid.New().String()
	id, err := ValidateUUID("user_id", validUUID)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if id.String() != validUUID {
		t.Errorf("expected %s, got %s", validUUID, id.String())
	}
}

func TestValidateUUIDFuncEmpty(t *testing.T) {
	_, err := ValidateUUID("user_id", "")
	if err == nil {
		t.Error("expected error for empty UUID")
	}
}

func TestValidateUUIDFuncInvalid(t *testing.T) {
	_, err := ValidateUUID("user_id", "bad")
	if err == nil {
		t.Error("expected error for invalid UUID")
	}
}

func TestRequiredFunc(t *testing.T) {
	err := Required("name", "value")
	if err != nil {
		t.Errorf("expected nil, got %v", err)
	}

	err = Required("name", "")
	if err == nil {
		t.Error("expected error for empty required field")
	}
}
