package logging

import (
	"maps"
	"testing"
)

func TestBuildDirectives(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		base    string
		modules map[string]string
		want    string
	}{
		{"base only", "info", nil, "info"},
		{"base with overrides", "info", map[string]string{"sqlx": "warn", "hyper": "error"}, "info,hyper=error,sqlx=warn"},
		{"empty base with overrides", "", map[string]string{"sqlx": "warn"}, "sqlx=warn"},
		{"whitespace base", "  info  ", nil, "info"},
		{"empty level dropped", "info", map[string]string{"sqlx": "", "db": "debug"}, "info,db=debug"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := BuildDirectives(tt.base, tt.modules); got != tt.want {
				t.Errorf("BuildDirectives(%q, %v) = %q, want %q", tt.base, tt.modules, got, tt.want)
			}
		})
	}
}

func TestParseDirectives(t *testing.T) {
	t.Parallel()

	base, modules := ParseDirectives("info,sqlx=warn,hyper=error")
	if base != "info" {
		t.Errorf("base = %q, want info", base)
	}
	want := map[string]string{"sqlx": "warn", "hyper": "error"}
	if !maps.Equal(modules, want) {
		t.Errorf("modules = %v, want %v", modules, want)
	}

	base, modules = ParseDirectives("sqlx=warn")
	if base != "" {
		t.Errorf("base = %q, want empty", base)
	}
	if !maps.Equal(modules, map[string]string{"sqlx": "warn"}) {
		t.Errorf("modules = %v", modules)
	}
}

func TestDirectivesRoundTrip(t *testing.T) {
	t.Parallel()

	cases := []struct {
		base    string
		modules map[string]string
	}{
		{"info", map[string]string{"sqlx": "warn", "hyper": "error"}},
		{"", map[string]string{"sqlx": "warn"}},
		{"debug", nil},
	}
	for _, c := range cases {
		built := BuildDirectives(c.base, c.modules)
		base, modules := ParseDirectives(built)
		if base != c.base {
			t.Errorf("round-trip base for %q = %q", built, base)
		}
		want := c.modules
		if want == nil {
			want = map[string]string{}
		}
		if !maps.Equal(modules, want) {
			t.Errorf("round-trip modules for %q = %v, want %v", built, modules, want)
		}
	}
}
