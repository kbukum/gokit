package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

func TestResolveOTLPEndpointPolicy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		endpoint string
		insecure bool
		want     string
		wantErr  bool
	}{
		{"scheme-less secure by default", "collector:4317", false, "https://collector:4317", false},
		{"scheme-less insecure opt-in", "collector:4317", true, "http://collector:4317", false},
		{"explicit http requires insecure", "http://collector:4317", false, "", true},
		{"explicit http with insecure preserved", "http://collector:4317", true, "http://collector:4317", false},
		{"explicit https stays secure", "https://collector:4317", false, "https://collector:4317", false},
		{"https with insecure is rejected", "https://collector:4317", true, "", true},
		{"uppercase https normalized", "HTTPS://collector:4317", false, "https://collector:4317", false},
		{"mixed-case http normalized", "HtTp://collector:4317", true, "http://collector:4317", false},
		{"unsupported scheme rejected", "grpc://collector:4317", false, "", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := resolveOTLPEndpoint(OTLPConfig{Endpoint: tc.endpoint, Insecure: tc.insecure})
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for endpoint=%q insecure=%v", tc.endpoint, tc.insecure)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("resolveOTLPEndpoint(%q, insecure=%v) = %q, want %q", tc.endpoint, tc.insecure, got, tc.want)
			}
		})
	}
}

func TestOTLPProviderConstruction(t *testing.T) {
	t.Parallel()

	for _, proto := range []string{"grpc", "http"} {
		p, err := NewOTLPProvider(OTLPProviderConfig{
			Exporter:    OTLPConfig{Endpoint: "localhost:4317", Protocol: proto, Insecure: true},
			ServiceName: "svc",
			Environment: "test",
			Version:     "1.0.0",
		})
		if err != nil {
			t.Fatalf("NewOTLPProvider(%s): %v", proto, err)
		}
		if err := p.Shutdown(context.Background()); err != nil {
			t.Errorf("Shutdown(%s): %v", proto, err)
		}
	}
}

// TestDerivationPropagatesThroughFullPipeline exercises WithAttrs/WithGroup on
// every middleware layer at once (module levels, sampling, masking, context,
// fanout) plus a bring-your-own sink, by deriving loggers and logging within a
// group. It guards against a layer dropping bound attributes or groups.
func TestDerivationPropagatesThroughFullPipeline(t *testing.T) {
	t.Parallel()

	var primary, byo bytes.Buffer
	cfg := &Config{
		Level:        "debug",
		Format:       "json",
		Output:       OutputStdout(),
		Timestamp:    true,
		Masking:      MaskingConfig{Enabled: true, Replacement: "***"},
		Sampling:     SamplingConfig{Enabled: true, InitialRate: 100, ThereafterRate: 1},
		ModuleLevels: map[string]string{"auth": "debug"},
	}
	l, err := New(cfg, "svc", WithWriter(&primary), WithHandler(slog.NewJSONHandler(&byo, nil)))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if l.Handler() == nil {
		t.Fatal("Handler() should expose the root handler")
	}

	derived := l.WithComponent("auth").WithFields(map[string]any{"tenant": "acme"})
	derived.Slog().Info("nested", slog.Group("data", slog.String("password", "secret")))

	for name, b := range map[string]*bytes.Buffer{"primary": &primary, "byo": &byo} {
		var m map[string]any
		if err := json.Unmarshal(bytes.TrimSpace(b.Bytes()), &m); err != nil {
			t.Fatalf("%s: unmarshal %q: %v", name, b.String(), err)
		}
		if m[FieldComponent] != "auth" || m["tenant"] != "acme" {
			t.Errorf("%s: bound attrs lost: %v", name, m)
		}
		group, ok := m["data"].(map[string]any)
		if !ok {
			t.Fatalf("%s: expected nested group, got %v", name, m["data"])
		}
		if group["password"] == "secret" {
			t.Errorf("%s: nested secret not masked: %v", name, group)
		}
	}
}

func TestMaskingMasksBoundAttrs(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	cfg := &Config{Level: "info", Format: "json", Output: OutputStdout(), Masking: MaskingConfig{Enabled: true, Replacement: "***"}}
	l, err := New(cfg, "svc", WithWriter(&buf))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	l.WithFields(map[string]any{"password": "hunter2"}).Info("m")

	if strings.Contains(buf.String(), "hunter2") {
		t.Errorf("bound sensitive attr should be masked, got %q", buf.String())
	}
}
