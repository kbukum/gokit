package validation

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"

	appErrors "github.com/kbukum/gokit/errors"
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

func TestStructValidateAcronymFieldNames(t *testing.T) {
	type TriggerBot struct {
		UserID     string `validate:"required"`
		MeetingURL string `validate:"required,url"`
	}

	err := Validate(TriggerBot{UserID: "", MeetingURL: "not-a-url"})
	if err == nil {
		t.Fatal("expected validation error")
	}
	errStr := err.Error()
	// Without json tags, raw Go field names are used
	if !strings.Contains(errStr, "UserID") {
		t.Errorf("expected 'UserID' in error, got %q", errStr)
	}
	if !strings.Contains(errStr, "MeetingURL") {
		t.Errorf("expected 'MeetingURL' in error, got %q", errStr)
	}
}

// ---------------------------------------------------------------------------
// Table-driven tests for all built-in validators
// ---------------------------------------------------------------------------

func TestRequired_TableDriven(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{"non-empty string", "hello", false},
		{"empty string", "", true},
		{"whitespace only", "   ", true},
		{"tab only", "\t", true},
		{"newline only", "\n", true},
		{"mixed whitespace", " \t\n ", true},
		{"value with leading space", " hello", false},
		{"unicode value", "こんにちは", false},
		{"single char", "a", false},
		{"emoji", "😀", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			v := New()
			v.Required("field", tt.value)
			if got := v.HasErrors(); got != tt.wantErr {
				t.Errorf("Required(%q): HasErrors() = %v, want %v", tt.value, got, tt.wantErr)
			}
		})
	}
}

func TestMinLength_TableDriven(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		value   string
		minLen  int
		wantErr bool
	}{
		{"exact min", "abc", 3, false},
		{"above min", "abcd", 3, false},
		{"below min", "ab", 3, true},
		{"empty string min=0", "", 0, false},
		{"empty string min=1", "", 1, true},
		{"unicode bytes count", "日本語", 3, false}, // len() counts bytes, 9 bytes >= 3
		{"unicode short byte", "a", 2, true},
		{"min=0 always passes", "anything", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			v := New()
			v.MinLength("f", tt.value, tt.minLen)
			if got := v.HasErrors(); got != tt.wantErr {
				t.Errorf("MinLength(%q, %d): HasErrors() = %v, want %v",
					tt.value, tt.minLen, got, tt.wantErr)
			}
		})
	}
}

func TestMaxLength_TableDriven(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		value   string
		maxLen  int
		wantErr bool
	}{
		{"within limit", "abc", 5, false},
		{"exact limit", "abc", 3, false},
		{"over limit", "abcdef", 3, true},
		{"empty string", "", 0, false},
		{"empty string max=5", "", 5, false},
		{"unicode bytes exceed", "日本", 3, true}, // 6 bytes > 3
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			v := New()
			v.MaxLength("f", tt.value, tt.maxLen)
			if got := v.HasErrors(); got != tt.wantErr {
				t.Errorf("MaxLength(%q, %d): HasErrors() = %v, want %v",
					tt.value, tt.maxLen, got, tt.wantErr)
			}
		})
	}
}

func TestMin_TableDriven(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		value   int
		minVal  int
		wantErr bool
	}{
		{"above min", 10, 5, false},
		{"at min", 5, 5, false},
		{"below min", 4, 5, true},
		{"zero vs zero", 0, 0, false},
		{"negative below", -1, 0, true},
		{"negative at negative min", -5, -5, false},
		{"large value", 1 << 30, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			v := New()
			v.Min("f", tt.value, tt.minVal)
			if got := v.HasErrors(); got != tt.wantErr {
				t.Errorf("Min(%d, %d): HasErrors() = %v, want %v",
					tt.value, tt.minVal, got, tt.wantErr)
			}
		})
	}
}

func TestMax_TableDriven(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		value   int
		maxVal  int
		wantErr bool
	}{
		{"below max", 3, 10, false},
		{"at max", 10, 10, false},
		{"above max", 11, 10, true},
		{"zero at zero", 0, 0, false},
		{"negative above max", -3, -5, true},
		{"negative below max", -10, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			v := New()
			v.Max("f", tt.value, tt.maxVal)
			if got := v.HasErrors(); got != tt.wantErr {
				t.Errorf("Max(%d, %d): HasErrors() = %v, want %v",
					tt.value, tt.maxVal, got, tt.wantErr)
			}
		})
	}
}

func TestRange_TableDriven(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		value   int
		minVal  int
		maxVal  int
		wantErr bool
	}{
		{"in range", 50, 1, 100, false},
		{"at min boundary", 1, 1, 100, false},
		{"at max boundary", 100, 1, 100, false},
		{"below range", 0, 1, 100, true},
		{"above range", 101, 1, 100, true},
		{"negative range valid", -5, -10, 0, false},
		{"negative range invalid", -11, -10, 0, true},
		{"single value range", 5, 5, 5, false},
		{"single value range miss", 6, 5, 5, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			v := New()
			v.Range("f", tt.value, tt.minVal, tt.maxVal)
			if got := v.HasErrors(); got != tt.wantErr {
				t.Errorf("Range(%d, %d, %d): HasErrors() = %v, want %v",
					tt.value, tt.minVal, tt.maxVal, got, tt.wantErr)
			}
		})
	}
}

func TestPattern_TableDriven(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		value   string
		pattern string
		wantErr bool
	}{
		{"matches alphanumeric", "abc123", `^[a-z0-9]+$`, false},
		{"does not match", "ABC", `^[a-z]+$`, true},
		{"empty value skips", "", `^[a-z]+$`, false},
		{"email-like pattern", "a@b.com", `^.+@.+\..+$`, false},
		{"partial match rejected", "123abc", `^[a-z]+$`, true},
		{"invalid regex reports error", "test", `[invalid`, true},
		{"unicode pattern", "日本語", `^[\p{Han}\p{Hiragana}\p{Katakana}]+$`, false},
		{"dot matches char", "a", `.`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			v := New()
			v.Pattern("f", tt.value, tt.pattern)
			if got := v.HasErrors(); got != tt.wantErr {
				t.Errorf("Pattern(%q, %q): HasErrors() = %v, want %v",
					tt.value, tt.pattern, got, tt.wantErr)
			}
		})
	}
}

func TestOneOf_TableDriven(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		value   string
		allowed []string
		wantErr bool
	}{
		{"valid value", "active", []string{"active", "inactive"}, false},
		{"invalid value", "deleted", []string{"active", "inactive"}, true},
		{"empty value skips", "", []string{"a", "b"}, false},
		{"single allowed", "only", []string{"only"}, false},
		{"case sensitive match", "Active", []string{"active"}, true},
		{"empty allowed list", "any", []string{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			v := New()
			v.OneOf("f", tt.value, tt.allowed)
			if got := v.HasErrors(); got != tt.wantErr {
				t.Errorf("OneOf(%q, %v): HasErrors() = %v, want %v",
					tt.value, tt.allowed, got, tt.wantErr)
			}
		})
	}
}

func TestCustom_TableDriven(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		condition bool
		wantErr   bool
	}{
		{"true condition passes", true, false},
		{"false condition fails", false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			v := New()
			v.Custom(tt.condition, "f", "custom msg")
			if got := v.HasErrors(); got != tt.wantErr {
				t.Errorf("Custom(%v): HasErrors() = %v, want %v",
					tt.condition, got, tt.wantErr)
			}
		})
	}
}

func TestRequiredUUID_TableDriven(t *testing.T) {
	t.Parallel()
	validID := uuid.New().String()
	tests := []struct {
		name    string
		value   string
		wantErr bool
		wantMsg string
	}{
		{"valid uuid", validID, false, ""},
		{"empty string", "", true, "is required"},
		{"whitespace only", "  ", true, "is required"},
		{"invalid format", "not-a-uuid", true, "must be a valid UUID"},
		{"nil uuid", uuid.Nil.String(), true, "must not be empty"},
		{"partial uuid", "550e8400-e29b-41d4", true, "must be a valid UUID"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			v := New()
			v.RequiredUUID("id", tt.value)
			if got := v.HasErrors(); got != tt.wantErr {
				t.Errorf("RequiredUUID(%q): HasErrors() = %v, want %v", tt.value, got, tt.wantErr)
			}
			if tt.wantErr && tt.wantMsg != "" {
				if msg := v.Errors()[0].Message; msg != tt.wantMsg {
					t.Errorf("RequiredUUID(%q): message = %q, want %q", tt.value, msg, tt.wantMsg)
				}
			}
		})
	}
}

func TestOptionalUUID_TableDriven(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{"empty is ok", "", false},
		{"valid uuid", uuid.New().String(), false},
		{"invalid uuid", "garbage", true},
		{"nil uuid is valid format", uuid.Nil.String(), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			v := New()
			v.OptionalUUID("id", tt.value)
			if got := v.HasErrors(); got != tt.wantErr {
				t.Errorf("OptionalUUID(%q): HasErrors() = %v, want %v", tt.value, got, tt.wantErr)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Validator struct/builder pattern
// ---------------------------------------------------------------------------

func TestNew_ReturnsEmptyValidator(t *testing.T) {
	t.Parallel()
	v := New()
	if v == nil {
		t.Fatal("New() returned nil")
	}
	if v.HasErrors() {
		t.Error("new validator should have no errors")
	}
	if len(v.Errors()) != 0 {
		t.Error("new validator Errors() should be empty slice")
	}
}

func TestAddError_DirectlyAddsFieldError(t *testing.T) {
	t.Parallel()
	v := New()
	v.AddError("email", "is invalid")
	v.AddError("name", "too short")

	errs := v.Errors()
	if len(errs) != 2 {
		t.Fatalf("expected 2 errors, got %d", len(errs))
	}
	if errs[0].Field != "email" || errs[0].Message != "is invalid" {
		t.Errorf("first error = %+v, want {email, is invalid}", errs[0])
	}
	if errs[1].Field != "name" || errs[1].Message != "too short" {
		t.Errorf("second error = %+v, want {name, too short}", errs[1])
	}
}

func TestChainingReturnsSameInstance(t *testing.T) {
	t.Parallel()
	v := New()
	got := v.
		Required("a", "val").
		MinLength("b", "val", 1).
		MaxLength("c", "val", 100).
		Min("d", 5, 0).
		Max("e", 5, 10).
		Range("f", 5, 0, 10).
		Pattern("g", "abc", `^[a-z]+$`).
		OneOf("h", "x", []string{"x", "y"}).
		RequiredUUID("i", uuid.New().String()).
		OptionalUUID("j", "").
		Custom(true, "k", "ok")
	if got != v {
		t.Error("chaining should always return the same *Validator")
	}
	if v.HasErrors() {
		t.Errorf("expected no errors, got %v", v.Errors())
	}
}

// ---------------------------------------------------------------------------
// FieldError creation and properties
// ---------------------------------------------------------------------------

func TestFieldError_FieldsExposed(t *testing.T) {
	t.Parallel()
	fe := FieldError{Field: "username", Message: "is required"}
	if fe.Field != "username" {
		t.Errorf("Field = %q, want %q", fe.Field, "username")
	}
	if fe.Message != "is required" {
		t.Errorf("Message = %q, want %q", fe.Message, "is required")
	}
}

// ---------------------------------------------------------------------------
// Multiple simultaneous validation errors collected
// ---------------------------------------------------------------------------

func TestMultipleErrors_Collected(t *testing.T) {
	t.Parallel()
	v := New()
	v.Required("name", "")
	v.Required("email", "")
	v.Min("age", -1, 0)
	v.MaxLength("bio", strings.Repeat("x", 300), 255)

	errs := v.Errors()
	if len(errs) != 4 {
		t.Fatalf("expected 4 errors, got %d: %v", len(errs), errs)
	}
	fields := map[string]bool{}
	for _, e := range errs {
		fields[e.Field] = true
	}
	for _, want := range []string{"name", "email", "age", "bio"} {
		if !fields[want] {
			t.Errorf("missing error for field %q", want)
		}
	}
}

func TestValidate_MultipleErrors_AppErrorContainsAll(t *testing.T) {
	t.Parallel()
	v := New()
	v.Required("first_name", "")
	v.Required("last_name", "")
	v.Min("score", -10, 0)

	appErr := v.Validate()
	if appErr == nil {
		t.Fatal("expected AppError")
	}
	msg := appErr.Message
	for _, want := range []string{"first_name", "last_name", "score"} {
		if !strings.Contains(msg, want) {
			t.Errorf("AppError message should contain %q, got %q", want, msg)
		}
	}
	fieldsRaw, ok := appErr.Details["fields"]
	if !ok {
		t.Fatal("expected 'fields' in Details")
	}
	fieldSlice, ok := fieldsRaw.([]FieldError)
	if !ok {
		t.Fatalf("fields is %T, want []FieldError", fieldsRaw)
	}
	if len(fieldSlice) != 3 {
		t.Errorf("expected 3 field errors in details, got %d", len(fieldSlice))
	}
}

func TestValidate_NoErrors_ReturnsNil(t *testing.T) {
	t.Parallel()
	v := New()
	v.Required("name", "Alice")
	if appErr := v.Validate(); appErr != nil {
		t.Errorf("expected nil, got %v", appErr)
	}
}

// ---------------------------------------------------------------------------
// Struct tag validation (Validate function)
// ---------------------------------------------------------------------------

func TestStructValidate_Email(t *testing.T) {
	t.Parallel()
	type Form struct {
		Email string `json:"email" validate:"required,email"`
	}
	tests := []struct {
		name    string
		email   string
		wantErr bool
	}{
		{"valid email", "user@example.com", false},
		{"no domain", "user@", true},
		{"empty", "", true},
		{"missing @", "userexample.com", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := Validate(Form{Email: tt.email})
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate(email=%q): err=%v, wantErr=%v", tt.email, err, tt.wantErr)
			}
		})
	}
}

func TestStructValidate_URL(t *testing.T) {
	t.Parallel()
	type Form struct {
		Link string `json:"link" validate:"required,url"`
	}
	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{"valid https", "https://example.com", false},
		{"valid http", "http://example.com/path", false},
		{"missing scheme", "example.com", true},
		{"empty", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := Validate(Form{Link: tt.url})
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate(link=%q): err=%v, wantErr=%v", tt.url, err, tt.wantErr)
			}
		})
	}
}

func TestStructValidate_UUID(t *testing.T) {
	t.Parallel()
	type Form struct {
		ID string `json:"id" validate:"required,uuid"`
	}
	tests := []struct {
		name    string
		id      string
		wantErr bool
	}{
		{"valid uuid", uuid.New().String(), false},
		{"nil uuid", uuid.Nil.String(), false}, // format is valid
		{"invalid", "not-uuid", true},
		{"empty", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := Validate(Form{ID: tt.id})
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate(id=%q): err=%v, wantErr=%v", tt.id, err, tt.wantErr)
			}
		})
	}
}

func TestStructValidate_OneOf(t *testing.T) {
	t.Parallel()
	type Form struct {
		Status string `json:"status" validate:"required,oneof=active inactive"`
	}
	tests := []struct {
		name    string
		status  string
		wantErr bool
	}{
		{"valid active", "active", false},
		{"valid inactive", "inactive", false},
		{"invalid value", "deleted", true},
		{"empty", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := Validate(Form{Status: tt.status})
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate(status=%q): err=%v, wantErr=%v", tt.status, err, tt.wantErr)
			}
		})
	}
}

func TestStructValidate_MinMax(t *testing.T) {
	t.Parallel()
	type Form struct {
		Name string `json:"name" validate:"required,min=2,max=50"`
	}
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid", "Alice", false},
		{"too short", "A", true},
		{"exact min", "Ab", false},
		{"too long", strings.Repeat("a", 51), true},
		{"exact max", strings.Repeat("a", 50), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := Validate(Form{Name: tt.input})
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate(name=%q): err=%v, wantErr=%v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestStructValidate_MultipleFieldErrors(t *testing.T) {
	t.Parallel()
	type User struct {
		Name  string `json:"name" validate:"required,min=2"`
		Email string `json:"email" validate:"required,email"`
		Age   int    `json:"age" validate:"required,min=1"`
	}

	err := Validate(User{Name: "", Email: "bad", Age: 0})
	if err == nil {
		t.Fatal("expected error")
	}
	var appErr *appErrors.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected *appErrors.AppError, got %T", err)
	}
	fields, ok := appErr.Details["fields"].([]FieldError)
	if !ok {
		t.Fatalf("expected []FieldError in details, got %T", appErr.Details["fields"])
	}
	if len(fields) < 2 {
		t.Errorf("expected at least 2 field errors, got %d", len(fields))
	}
}

func TestStructValidate_JsonTagUsedForFieldName(t *testing.T) {
	t.Parallel()
	type Form struct {
		FirstName string `json:"first_name" validate:"required"`
	}
	err := Validate(Form{FirstName: ""})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "first_name") {
		t.Errorf("expected json tag 'first_name' in error, got %q", err.Error())
	}
}

func TestStructValidate_NoJsonTag_UsesGoFieldName(t *testing.T) {
	t.Parallel()
	type Form struct {
		GoFieldName string `validate:"required"`
	}
	err := Validate(Form{GoFieldName: ""})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "GoFieldName") {
		t.Errorf("expected Go field name in error, got %q", err.Error())
	}
}

func TestStructValidate_ValidStruct_NoError(t *testing.T) {
	t.Parallel()
	type Form struct {
		Name  string `json:"name" validate:"required"`
		Email string `json:"email" validate:"required,email"`
	}
	err := Validate(Form{Name: "Alice", Email: "alice@example.com"})
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestStructValidate_NoValidationTags(t *testing.T) {
	t.Parallel()
	type Form struct {
		Name string `json:"name"`
	}
	err := Validate(Form{Name: ""})
	if err != nil {
		t.Errorf("struct with no validate tags should pass, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Standalone helper functions
// ---------------------------------------------------------------------------

func TestRequiredFunc_TableDriven(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{"non-empty", "val", false},
		{"empty", "", true},
		{"whitespace", "  ", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := Required("field", tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("Required(%q) err=%v, wantErr=%v", tt.value, err, tt.wantErr)
			}
		})
	}
}

func TestValidateUUID_TableDriven(t *testing.T) {
	t.Parallel()
	validID := uuid.New()
	tests := []struct {
		name    string
		value   string
		wantID  uuid.UUID
		wantErr bool
	}{
		{"valid", validID.String(), validID, false},
		{"empty", "", uuid.Nil, true},
		{"whitespace", "  ", uuid.Nil, true},
		{"invalid", "xyz", uuid.Nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			id, err := ValidateUUID("f", tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateUUID(%q): err=%v, wantErr=%v", tt.value, err, tt.wantErr)
			}
			if !tt.wantErr && id != tt.wantID {
				t.Errorf("ValidateUUID(%q): got %v, want %v", tt.value, id, tt.wantID)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Security: Injection via validation messages
// ---------------------------------------------------------------------------

func TestSecurity_InjectionInFieldNames(t *testing.T) {
	t.Parallel()
	maliciousField := `<script>alert("xss")</script>`
	maliciousMsg := `"; DROP TABLE users; --`
	v := New()
	v.AddError(maliciousField, maliciousMsg)

	errs := v.Errors()
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d", len(errs))
	}
	// The validator should faithfully store what it receives (no mutation).
	// Escaping is the responsibility of the output layer, but we verify
	// that the message is not silently altered or causing panics.
	if errs[0].Field != maliciousField {
		t.Errorf("field was mutated: got %q", errs[0].Field)
	}
	if errs[0].Message != maliciousMsg {
		t.Errorf("message was mutated: got %q", errs[0].Message)
	}

	appErr := v.Validate()
	if appErr == nil {
		t.Fatal("expected AppError")
	}
	// Verify AppError message includes both without panicking
	if !strings.Contains(appErr.Message, maliciousField) {
		t.Errorf("AppError.Message missing field: %q", appErr.Message)
	}
}

func TestSecurity_InjectionViaCustomValidator(t *testing.T) {
	t.Parallel()
	v := New()
	v.Custom(false, "field", `<img onerror="alert(1)" src=x>`)
	errs := v.Errors()
	if len(errs) != 1 {
		t.Fatal("expected 1 error")
	}
	if !strings.Contains(errs[0].Message, "onerror") {
		t.Error("injection payload should be stored as-is")
	}
}

func TestSecurity_ReDoSSafePattern(t *testing.T) {
	t.Parallel()
	// Go's regexp uses RE2 which is inherently safe against catastrophic backtracking.
	// This test verifies that complex input doesn't hang.
	v := New()
	longInput := strings.Repeat("a", 10000) + "!"
	v.Pattern("f", longInput, `^[a-z]+$`)
	if !v.HasErrors() {
		t.Error("expected pattern mismatch for input with trailing '!'")
	}
}

func TestSecurity_NullBytesInInput(t *testing.T) {
	t.Parallel()
	v := New()
	v.Required("f", "hello\x00world")
	if v.HasErrors() {
		t.Error("string with null byte should be considered non-empty")
	}
}

// ---------------------------------------------------------------------------
// Edge cases: empty, unicode, large inputs, zero values, nil
// ---------------------------------------------------------------------------

func TestEdge_EmptyValidator_Validate(t *testing.T) {
	t.Parallel()
	v := New()
	if appErr := v.Validate(); appErr != nil {
		t.Errorf("empty validator.Validate() should return nil, got %v", appErr)
	}
}

func TestEdge_UnicodeFieldNames(t *testing.T) {
	t.Parallel()
	v := New()
	v.Required("名前", "")
	errs := v.Errors()
	if len(errs) != 1 {
		t.Fatal("expected 1 error")
	}
	if errs[0].Field != "名前" {
		t.Errorf("field = %q, want %q", errs[0].Field, "名前")
	}
}

func TestEdge_ExtremelyLargeStringInput(t *testing.T) {
	t.Parallel()
	largeStr := strings.Repeat("a", 1_000_000)
	v := New()
	v.Required("f", largeStr)
	if v.HasErrors() {
		t.Error("large string should not be treated as empty")
	}
	v.MaxLength("f", largeStr, 100)
	if !v.HasErrors() {
		t.Error("large string should exceed max length 100")
	}
}

func TestEdge_ZeroIntValues(t *testing.T) {
	t.Parallel()
	v := New()
	v.Min("f", 0, 0)
	v.Max("f", 0, 0)
	v.Range("f", 0, 0, 0)
	if v.HasErrors() {
		t.Error("zero should pass min=0, max=0, range(0,0)")
	}
}

func TestEdge_EmptyStringLength(t *testing.T) {
	t.Parallel()
	v := New()
	v.MinLength("f", "", 0)
	v.MaxLength("f", "", 0)
	if v.HasErrors() {
		t.Error("empty string should pass minLen=0, maxLen=0")
	}
}

func TestEdge_NegativeMinLength(t *testing.T) {
	t.Parallel()
	v := New()
	// negative min length should always pass since len() >= 0 > negative
	v.MinLength("f", "", -1)
	if v.HasErrors() {
		t.Error("negative min length should pass for any string")
	}
}

func TestEdge_Pattern_EmptyPattern(t *testing.T) {
	t.Parallel()
	v := New()
	// empty pattern matches everything
	v.Pattern("f", "anything", "")
	if v.HasErrors() {
		t.Error("empty pattern should match any string")
	}
}

func TestEdge_OneOf_EmptyAllowed(t *testing.T) {
	t.Parallel()
	v := New()
	v.OneOf("f", "something", []string{})
	if !v.HasErrors() {
		t.Error("non-empty value with empty allowed list should fail")
	}
}

func TestEdge_MultiByteCharMaxLength(t *testing.T) {
	t.Parallel()
	// MaxLength uses len() which counts bytes, not runes
	v := New()
	v.MaxLength("f", "日", 1) // "日" is 3 bytes
	if !v.HasErrors() {
		t.Error("multi-byte char should exceed byte-count max of 1")
	}
}

func TestEdge_ManyErrors(t *testing.T) {
	t.Parallel()
	v := New()
	for i := 0; i < 100; i++ {
		v.AddError(fmt.Sprintf("field_%d", i), "error")
	}
	if len(v.Errors()) != 100 {
		t.Errorf("expected 100 errors, got %d", len(v.Errors()))
	}
	appErr := v.Validate()
	if appErr == nil {
		t.Fatal("expected AppError")
	}
	// All 100 fields should appear in the message
	for i := 0; i < 100; i++ {
		want := fmt.Sprintf("field_%d", i)
		if !strings.Contains(appErr.Message, want) {
			t.Errorf("message missing %q", want)
			break
		}
	}
}

// ---------------------------------------------------------------------------
// formatValidationError coverage via struct validation
// ---------------------------------------------------------------------------

func TestFormatValidationError_AllTags(t *testing.T) {
	t.Parallel()

	// Each sub-test triggers a different validation tag
	t.Run("required", func(t *testing.T) {
		t.Parallel()
		type S struct {
			F string `json:"f" validate:"required"`
		}
		err := Validate(S{})
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "is required") {
			t.Errorf("expected 'is required', got %q", err.Error())
		}
	})

	t.Run("email", func(t *testing.T) {
		t.Parallel()
		type S struct {
			F string `json:"f" validate:"email"`
		}
		err := Validate(S{F: "bad"})
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "valid email") {
			t.Errorf("expected 'valid email' message, got %q", err.Error())
		}
	})

	t.Run("min", func(t *testing.T) {
		t.Parallel()
		type S struct {
			F string `json:"f" validate:"min=5"`
		}
		err := Validate(S{F: "ab"})
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "at least 5") {
			t.Errorf("expected 'at least 5' message, got %q", err.Error())
		}
	})

	t.Run("max", func(t *testing.T) {
		t.Parallel()
		type S struct {
			F string `json:"f" validate:"max=2"`
		}
		err := Validate(S{F: "abcdef"})
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "at most 2") {
			t.Errorf("expected 'at most 2' message, got %q", err.Error())
		}
	})

	t.Run("url", func(t *testing.T) {
		t.Parallel()
		type S struct {
			F string `json:"f" validate:"url"`
		}
		err := Validate(S{F: "not-a-url"})
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "valid URL") {
			t.Errorf("expected 'valid URL' message, got %q", err.Error())
		}
	})

	t.Run("uuid", func(t *testing.T) {
		t.Parallel()
		type S struct {
			F string `json:"f" validate:"uuid"`
		}
		err := Validate(S{F: "bad"})
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "valid UUID") {
			t.Errorf("expected 'valid UUID' message, got %q", err.Error())
		}
	})

	t.Run("oneof", func(t *testing.T) {
		t.Parallel()
		type S struct {
			F string `json:"f" validate:"oneof=a b c"`
		}
		err := Validate(S{F: "z"})
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "must be one of") {
			t.Errorf("expected 'must be one of' message, got %q", err.Error())
		}
	})

	t.Run("default_tag", func(t *testing.T) {
		t.Parallel()
		type S struct {
			F string `json:"f" validate:"alpha"`
		}
		err := Validate(S{F: "123"})
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "is invalid") {
			t.Errorf("expected 'is invalid' fallback message, got %q", err.Error())
		}
	})
}

// ---------------------------------------------------------------------------
// Validate function returns *errors.AppError with correct properties
// ---------------------------------------------------------------------------

func TestValidate_AppErrorProperties(t *testing.T) {
	t.Parallel()
	v := New()
	v.Required("name", "")
	appErr := v.Validate()
	if appErr == nil {
		t.Fatal("expected error")
	}
	if appErr.Code != appErrors.ErrCodeInvalidInput {
		t.Errorf("expected code %q, got %q", appErrors.ErrCodeInvalidInput, appErr.Code)
	}
	if appErr.HTTPStatus != 422 {
		t.Errorf("expected HTTP 422, got %d", appErr.HTTPStatus)
	}
}

func TestStructValidate_ReturnsAppError(t *testing.T) {
	t.Parallel()
	type S struct {
		F string `json:"f" validate:"required"`
	}
	err := Validate(S{})
	if err == nil {
		t.Fatal("expected error")
	}
	var appErr *appErrors.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected *appErrors.AppError, got %T", err)
	}
	if appErr.Code != appErrors.ErrCodeInvalidInput {
		t.Errorf("expected INVALID_INPUT, got %q", appErr.Code)
	}
}

// ---------------------------------------------------------------------------
// Concurrent safety: multiple goroutines using the validator
// ---------------------------------------------------------------------------

func TestValidator_ConcurrentStructValidation(t *testing.T) {
	t.Parallel()
	type User struct {
		Name  string `json:"name" validate:"required,min=2"`
		Email string `json:"email" validate:"required,email"`
	}

	// Run many struct validations concurrently to detect races
	done := make(chan struct{}, 50)
	for i := 0; i < 50; i++ {
		go func(n int) {
			defer func() { done <- struct{}{} }()
			if n%2 == 0 {
				_ = Validate(User{Name: "Alice", Email: "alice@example.com"})
			} else {
				_ = Validate(User{Name: "", Email: "bad"})
			}
		}(i)
	}
	for i := 0; i < 50; i++ {
		<-done
	}
}
