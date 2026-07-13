package bootstrap

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/kbukum/gokit/config"
)

// failValidationConfig returns an error from Validate().
type failValidationConfig struct {
	config.ServiceConfig
	valErr error
}

func (f *failValidationConfig) Validate() error { return f.valErr }

func TestConfigValidationFailureRecovery(t *testing.T) {
	cfg := &failValidationConfig{
		ServiceConfig: config.ServiceConfig{
			Name:        "test-svc",
			Environment: "development",
		},
		valErr: fmt.Errorf("port out of range"),
	}
	_, err := NewApp(cfg)
	if err == nil {
		t.Fatal("expected error from config validation")
	}
	if !strings.Contains(err.Error(), "config validation") {
		t.Errorf("expected 'config validation' prefix, got %q", err.Error())
	}
	if !strings.Contains(err.Error(), "port out of range") {
		t.Errorf("expected wrapped validation error, got %q", err.Error())
	}
}

func TestConfigValidationWithWrappedError(t *testing.T) {
	inner := fmt.Errorf("inner cause")
	cfg := &failValidationConfig{
		ServiceConfig: config.ServiceConfig{
			Name:        "test-svc",
			Environment: "development",
		},
		valErr: fmt.Errorf("validation: %w", inner),
	}
	_, err := NewApp(cfg)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, inner) {
		t.Error("expected wrapped error to be unwrappable")
	}
}
