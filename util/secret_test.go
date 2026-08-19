package util

import (
	"encoding/json"
	"fmt"
	"testing"
)

func TestSecretStringMasks(t *testing.T) {
	t.Parallel()
	s := NewSecretString("password123")
	if got := s.String(); got != "***" {
		t.Errorf("String = %q, want ***", got)
	}
	if got := fmt.Sprintf("%#v", s); got != "SecretString(***)" {
		t.Errorf("GoString = %q", got)
	}
}

func TestSecretStringEmptyDisplaysEmpty(t *testing.T) {
	t.Parallel()
	if got := NewSecretString("").String(); got != "" {
		t.Errorf("empty String = %q", got)
	}
}

func TestSecretStringExpose(t *testing.T) {
	t.Parallel()
	if got := NewSecretString("hunter2").Expose(); got != "hunter2" {
		t.Errorf("Expose = %q", got)
	}
}

func TestSecretStringJSON(t *testing.T) {
	t.Parallel()
	data, err := json.Marshal(NewSecretString("secret"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `"***"` {
		t.Errorf("marshaled = %s", data)
	}
	var s SecretString
	if err := json.Unmarshal([]byte(`"actual_value"`), &s); err != nil {
		t.Fatal(err)
	}
	if s.Expose() != "actual_value" {
		t.Errorf("unmarshaled = %q", s.Expose())
	}
}

func TestSecretStringEqual(t *testing.T) {
	t.Parallel()
	a := NewSecretString("same-value")
	b := NewSecretString("same-value")
	c := NewSecretString("different")
	if !a.Equal(b) || a.Equal(c) {
		t.Error("constant-time equality mismatch")
	}
}
