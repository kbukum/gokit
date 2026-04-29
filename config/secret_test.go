package config

import (
	"encoding/json"
	"fmt"
	"testing"
)

func TestSecretStringMasksDisplay(t *testing.T) {
	s := NewSecretString("password123")
	if got := s.String(); got != "***" {
		t.Errorf("String() = %q, want %q", got, "***")
	}
	if got := fmt.Sprintf("%#v", s); got != "SecretString{***}" {
		t.Errorf("GoString() = %q, want %q", got, "SecretString{***}")
	}
}

func TestSecretStringEmptyIsTransparent(t *testing.T) {
	s := NewSecretString("")
	if got := s.String(); got != "" {
		t.Errorf("String() of empty = %q, want empty", got)
	}
	if !s.IsEmpty() {
		t.Error("IsEmpty() should be true for empty secret")
	}
}

func TestSecretStringValueReturnsPlaintext(t *testing.T) {
	s := NewSecretString("hunter2")
	if got := s.Value(); got != "hunter2" {
		t.Errorf("Value() = %q, want %q", got, "hunter2")
	}
}

func TestSecretStringJSONMasked(t *testing.T) {
	s := NewSecretString("secret")
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `"***"` {
		t.Errorf("MarshalJSON = %s, want %q", data, "***")
	}
}

func TestSecretStringJSONUnmarshalRestoresValue(t *testing.T) {
	var s SecretString
	if err := json.Unmarshal([]byte(`"actual_value"`), &s); err != nil {
		t.Fatal(err)
	}
	if s.Value() != "actual_value" {
		t.Errorf("Value() after unmarshal = %q, want %q", s.Value(), "actual_value")
	}
}

func TestSecretStringTextMarshal(t *testing.T) {
	s := NewSecretString("secret")
	data, err := s.MarshalText()
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "***" {
		t.Errorf("MarshalText = %q, want %q", string(data), "***")
	}
}

func TestSecretStringTextUnmarshal(t *testing.T) {
	var s SecretString
	if err := s.UnmarshalText([]byte("plaintext")); err != nil {
		t.Fatal(err)
	}
	if s.Value() != "plaintext" {
		t.Errorf("Value() after UnmarshalText = %q, want %q", s.Value(), "plaintext")
	}
}
