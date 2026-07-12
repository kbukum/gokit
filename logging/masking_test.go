package logging

import (
	"bytes"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/rs/zerolog"
)

func newTestMasker() *DefaultMasker {
	return NewDefaultMasker(MaskingConfig{
		Enabled:     true,
		Replacement: "***REDACTED***",
	})
}

func TestMaskValue_FieldNames(t *testing.T) {
	m := newTestMasker()

	tests := []struct {
		name string
		key  string
		val  string
		want string
	}{
		{"password", "password", "mysecretpw", "***REDACTED***"},
		{"secret", "secret", "s3cr3t", "***REDACTED***"},
		{"token", "token", "tok_abc123", "***REDACTED***"},
		{"api_key", "api_key", "key-value", "***REDACTED***"},
		{"apikey", "apikey", "key-value", "***REDACTED***"},
		{"api-key", "api-key", "key-value", "***REDACTED***"},
		{"authorization", "authorization", "Bearer xyz", "***REDACTED***"},
		{"auth_token", "auth_token", "tok", "***REDACTED***"},
		{"access_token", "access_token", "tok", "***REDACTED***"},
		{"refresh_token", "refresh_token", "tok", "***REDACTED***"},
		{"private_key", "private_key", "-----BEGIN RSA", "***REDACTED***"},
		{"ssn", "ssn", "123-45-6789", "***REDACTED***"},
		{"credit_card", "credit_card", "4111111111111111", "***REDACTED***"},
		{"card_number", "card_number", "4111111111111111", "***REDACTED***"},
		{"cvv", "cvv", "123", "***REDACTED***"},
		{"pin", "pin", "9876", "***REDACTED***"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := m.MaskValue(tc.key, tc.val)
			if got != tc.want {
				t.Errorf("MaskValue(%q, %q) = %q, want %q", tc.key, tc.val, got, tc.want)
			}
		})
	}
}

func TestMaskValue_CaseInsensitiveFieldNames(t *testing.T) {
	m := newTestMasker()

	tests := []struct {
		name string
		key  string
	}{
		{"Password", "Password"},
		{"PASSWORD", "PASSWORD"},
		{"Api_Key", "Api_Key"},
		{"TOKEN", "TOKEN"},
		{"Authorization", "Authorization"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := m.MaskValue(tc.key, "some-value")
			if got != "***REDACTED***" {
				t.Errorf("MaskValue(%q, ...) = %q, want ***REDACTED***", tc.key, got)
			}
		})
	}
}

func TestMaskValue_Email(t *testing.T) {
	m := newTestMasker()

	tests := []struct {
		name  string
		value string
		want  string
	}{
		{"simple", "user@example.com", "***@***.***"},
		{"in text", "contact user@example.com for help", "contact ***@***.*** for help"},
		{"complex local", "first.last+tag@sub.domain.org", "***@***.***"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := m.MaskValue("email_field", tc.value)
			if got != tc.want {
				t.Errorf("MaskValue(email_field, %q) = %q, want %q", tc.value, got, tc.want)
			}
		})
	}
}

func TestMaskValue_CreditCard(t *testing.T) {
	m := newTestMasker()

	tests := []struct {
		name  string
		value string
		want  string
	}{
		{"no separators", "4111111111111111", "****-****-****-1111"},
		{"dashes", "4111-1111-1111-1111", "****-****-****-1111"},
		{"spaces", "4111 1111 1111 1111", "****-****-****-1111"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := m.MaskValue("data", tc.value)
			if got != tc.want {
				t.Errorf("MaskValue(data, %q) = %q, want %q", tc.value, got, tc.want)
			}
		})
	}
}

func TestMaskValue_SSN(t *testing.T) {
	m := newTestMasker()

	tests := []struct {
		name  string
		value string
		want  string
	}{
		{"with dashes", "123-45-6789", "***-**-****"},
		{"no dashes", "123456789", "***-**-****"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := m.MaskValue("data", tc.value)
			if got != tc.want {
				t.Errorf("MaskValue(data, %q) = %q, want %q", tc.value, got, tc.want)
			}
		})
	}
}

func TestMaskValue_JWT(t *testing.T) {
	m := newTestMasker()

	jwt := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiaWF0IjoxNTE2MjM5MDIyfQ.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c"
	got := m.MaskValue("header", jwt)
	if got != "[JWT_REDACTED]" {
		t.Errorf("MaskValue for JWT = %q, want [JWT_REDACTED]", got)
	}
}

func TestMaskValue_BearerToken(t *testing.T) {
	m := newTestMasker()

	tests := []struct {
		name  string
		value string
		want  string
	}{
		{"uppercase", "Bearer abc123.xyz", "Bearer [REDACTED]"},
		{"lowercase", "bearer abc123", "Bearer [REDACTED]"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := m.MaskValue("auth_header", tc.value)
			if got != tc.want {
				t.Errorf("MaskValue(auth_header, %q) = %q, want %q", tc.value, got, tc.want)
			}
		})
	}
}

func TestMaskValue_AWSKey(t *testing.T) {
	m := newTestMasker()

	got := m.MaskValue("key", "AKIAIOSFODNN7EXAMPLE")
	if got != "[AWS_KEY_REDACTED]" {
		t.Errorf("MaskValue for AWS key = %q, want [AWS_KEY_REDACTED]", got)
	}
}

func TestMaskValue_HexSecret(t *testing.T) {
	m := newTestMasker()

	hex := "abcdef0123456789abcdef0123456789" // 32 chars
	got := m.MaskValue("data", hex)
	if got != "[HEX_REDACTED]" {
		t.Errorf("MaskValue for hex = %q, want [HEX_REDACTED]", got)
	}
}

func TestMaskValue_NonSensitivePassThrough(t *testing.T) {
	m := newTestMasker()

	tests := []struct {
		name string
		key  string
		val  string
	}{
		{"plain text", "message", "hello world"},
		{"numeric", "count", "42"},
		{"url", "endpoint", "https://example.com/api/v1"},
		{"status", "status", "ok"},
		{"empty", "data", ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := m.MaskValue(tc.key, tc.val)
			if got != tc.val {
				t.Errorf("MaskValue(%q, %q) = %q, expected passthrough", tc.key, tc.val, got)
			}
		})
	}
}

func TestMaskValue_CustomFieldNames(t *testing.T) {
	m := NewDefaultMasker(MaskingConfig{
		Enabled:     true,
		FieldNames:  []string{"custom_secret", "my_token"},
		Replacement: "***REDACTED***",
	})

	tests := []struct {
		name string
		key  string
		want string
	}{
		{"custom field", "custom_secret", "***REDACTED***"},
		{"another custom", "my_token", "***REDACTED***"},
		{"default still works", "password", "***REDACTED***"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := m.MaskValue(tc.key, "some-value")
			if got != tc.want {
				t.Errorf("MaskValue(%q, ...) = %q, want %q", tc.key, got, tc.want)
			}
		})
	}
}

func TestMaskValue_CustomValuePatterns(t *testing.T) {
	m := NewDefaultMasker(MaskingConfig{
		Enabled:       true,
		ValuePatterns: []string{`secret_\d+`},
		Replacement:   "[MASKED]",
	})

	got := m.MaskValue("data", "the secret_12345 is here")
	if !strings.Contains(got, "[MASKED]") {
		t.Errorf("expected custom pattern to mask, got %q", got)
	}
	if strings.Contains(got, "secret_12345") {
		t.Error("expected secret_12345 to be masked")
	}
}

func TestMaskValue_PreserveLast(t *testing.T) {
	m := NewDefaultMasker(MaskingConfig{
		Enabled:      true,
		Replacement:  "***REDACTED***",
		PreserveLast: 4,
	})

	got := m.MaskValue("password", "mysecretpassword")
	if got != "***REDACTED***word" {
		t.Errorf("expected partial mask, got %q", got)
	}

	// Short value (shorter than PreserveLast) should still be fully replaced.
	got = m.MaskValue("pin", "12")
	if got != "***REDACTED***" {
		t.Errorf("expected full mask for short value, got %q", got)
	}
}

func TestMaskFields(t *testing.T) {
	m := newTestMasker()

	fields := map[string]interface{}{
		"password": "secret123",
		"username": "john",
		"email":    "john@example.com",
		"count":    42,
		"status":   "ok",
	}

	masked := m.MaskFields(fields)

	if masked["password"] != "***REDACTED***" {
		t.Errorf("password should be masked, got %v", masked["password"])
	}
	if masked["username"] != "john" {
		t.Errorf("username should pass through, got %v", masked["username"])
	}
	if masked["email"] != "***@***.***" {
		t.Errorf("email should be masked, got %v", masked["email"])
	}
	if masked["count"] != 42 {
		t.Errorf("count should be original value, got %v", masked["count"])
	}
	if masked["status"] != "ok" {
		t.Errorf("status should pass through, got %v", masked["status"])
	}
}

func TestMaskFields_Nil(t *testing.T) {
	m := newTestMasker()
	if m.MaskFields(nil) != nil {
		t.Error("expected nil for nil input")
	}
}

func TestMaskingDisabled(t *testing.T) {
	cfg := &Config{
		Level:  "debug",
		Format: "json",
		Output: "stdout",
		Masking: MaskingConfig{
			Enabled: false,
		},
	}
	l := New(cfg, "test")
	if l.masker != nil {
		t.Error("expected masker to be nil when masking is disabled")
	}
}

func TestMaskingEnabled_Integration(t *testing.T) {
	var buf bytes.Buffer
	zl := zerolog.New(&buf).Level(zerolog.DebugLevel)

	cfg := MaskingConfig{
		Enabled:     true,
		Replacement: "***REDACTED***",
	}
	m := NewDefaultMasker(cfg)

	l := &Logger{
		logger:  zl,
		service: "test",
		masker:  m,
	}

	l.Info("login attempt", map[string]interface{}{
		"username": "alice",
		"password": "secret123",
		"email":    "alice@example.com",
	})

	var entry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("failed to parse log output: %v", err)
	}

	if entry["password"] != "***REDACTED***" {
		t.Errorf("password should be masked in log, got %v", entry["password"])
	}
	if entry["username"] != "alice" {
		t.Errorf("username should pass through, got %v", entry["username"])
	}
	if entry["email"] != "***@***.***" {
		t.Errorf("email should be masked in log, got %v", entry["email"])
	}
}

func TestWithMasker(t *testing.T) {
	l := NewDefault("test")
	if l.masker == nil {
		// ApplyDefaults enables masking, but NewDefault doesn't call ApplyDefaults.
		// Just create a logger without masking to test WithMasker.
		cfg := &Config{
			Level:  "info",
			Format: "json",
			Output: "stdout",
			Masking: MaskingConfig{
				Enabled: false,
			},
		}
		l = New(cfg, "test")
	}

	m := newTestMasker()
	l2 := l.WithMasker(m)

	if l2.masker == nil {
		t.Error("expected masker to be set via WithMasker")
	}
	if l2.service != l.service {
		t.Error("expected service to be preserved")
	}
}

func TestMaskerPropagation(t *testing.T) {
	m := newTestMasker()
	cfg := &Config{
		Level:  "info",
		Format: "json",
		Output: "stdout",
		Masking: MaskingConfig{
			Enabled: false,
		},
	}
	l := New(cfg, "test")
	l = l.WithMasker(m)

	// WithComponent should propagate masker
	cl := l.WithComponent("handler")
	if cl.masker == nil {
		t.Error("expected masker to propagate through WithComponent")
	}

	// WithFields should propagate masker
	fl := l.WithFields(map[string]interface{}{"key": "val"})
	if fl.masker == nil {
		t.Error("expected masker to propagate through WithFields")
	}

	// WithError should propagate masker
	el := l.WithError(nil)
	if el.masker == nil {
		t.Error("expected masker to propagate through WithError")
	}
}

func TestMaskerConcurrentAccess(t *testing.T) {
	m := newTestMasker()
	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = m.MaskValue("password", "secret123")
			_ = m.MaskValue("data", "user@example.com")
			_ = m.MaskValue("info", "4111111111111111")
			_ = m.MaskValue("msg", "hello world")
			_ = m.MaskFields(map[string]interface{}{
				"token": "abc",
				"name":  "bob",
			})
		}()
	}

	wg.Wait()
}

func TestConfigApplyDefaults_Masking(t *testing.T) {
	cfg := Config{}
	cfg.ApplyDefaults()

	if !cfg.Masking.Enabled {
		t.Error("expected Masking.Enabled to default to true")
	}
	if cfg.Masking.Replacement != "***REDACTED***" {
		t.Errorf("expected default replacement, got %q", cfg.Masking.Replacement)
	}
}
