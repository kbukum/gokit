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

func TestConstantTimeEqual(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		a, b      []byte
		wantEqual bool
	}{
		{"equal", []byte("token"), []byte("token"), true},
		{"different_same_len", []byte("token"), []byte("other"), false},
		{"length_mismatch", []byte("token"), []byte("token-longer"), false},
		{"both_empty", []byte{}, []byte{}, true},
		{"nil_and_empty", nil, []byte{}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := ConstantTimeEqual(tc.a, tc.b); got != tc.wantEqual {
				t.Errorf("ConstantTimeEqual(%q,%q) = %v, want %v", tc.a, tc.b, got, tc.wantEqual)
			}
		})
	}
}
