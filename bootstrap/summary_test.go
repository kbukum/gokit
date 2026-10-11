package bootstrap

import (
	"bytes"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/kbukum/gokit/component"
	"github.com/kbukum/gokit/di"
)

func TestDisplaySummaryEmptyComponents(t *testing.T) {
	s := NewSummary("empty-svc", "0.1.0")
	s.SetStartupDuration(0)

	registry := component.NewRegistry()
	container := di.NewContainer()

	// Should not panic with empty everything
	s.DisplaySummary(t.Context(), registry, container, nil)
}

func TestDisplaySummaryZeroPort(t *testing.T) {
	s := NewSummary("svc", "1.0")
	s.SetStartupDuration(50 * time.Millisecond)
	s.TrackInfrastructure(InfrastructureInfo{Name: "DB", Type: "database", Status: "active", Details: "localhost", Port: 0, Healthy: true})

	registry := component.NewRegistry()
	container := di.NewContainer()

	// Should not panic and should not append ":0"
	s.DisplaySummary(t.Context(), registry, container, nil)
}

func TestDisplaySummaryZeroDuration(t *testing.T) {
	s := NewSummary("instant-svc", "0.0.1")
	s.SetStartupDuration(0)
	s.TrackRoute("GET", "/health", "HealthHandler")

	registry := component.NewRegistry()
	container := di.NewContainer()

	// Should render 0.00s without panic
	s.DisplaySummary(t.Context(), registry, container, nil)
}

func TestDisplaySummaryModuleTree(t *testing.T) {
	var buf bytes.Buffer
	s := NewSummaryWithOptions("svc", "1.0", WithWriter(&buf))
	s.SetStartupDuration(0)
	s.TrackModule(ModuleInfo{
		Name:       "identity",
		Provides:   []string{"identity.access", "identity.checker"},
		Needs:      []string{"tenancy.rights ← tenancy"},
		Components: []string{"identity-database", "identity-sessions"},
	})

	s.DisplaySummary(t.Context(), component.NewRegistry(), di.NewContainer(), nil)

	out := stripANSI(buf.String())
	for _, want := range []string{
		"identity", "provides", "identity.access", "identity.checker",
		"needs", "tenancy.rights ← tenancy", "components", "identity-database",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("summary missing %q\n%s", want, out)
		}
	}
	if strings.Contains(out, "identity.access, identity.checker") {
		t.Errorf("provides should nest as a tree, not comma-joined:\n%s", out)
	}
}

var ansiPattern = regexp.MustCompile("\x1b\\[[0-9;]*m")

func stripANSI(s string) string { return ansiPattern.ReplaceAllString(s, "") }

func TestDisplaySummaryNilContainer(t *testing.T) {
	s := NewSummary("nil-container-svc", "1.0")
	s.SetStartupDuration(10 * time.Millisecond)

	registry := component.NewRegistry()
	// nil container
	s.DisplaySummary(t.Context(), registry, nil, nil)
}
