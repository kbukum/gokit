package bootstrap

import (
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
	s.DisplaySummary(registry, container, nil)
}

func TestDisplaySummaryZeroPort(t *testing.T) {
	s := NewSummary("svc", "1.0")
	s.SetStartupDuration(50 * time.Millisecond)
	s.TrackInfrastructure("DB", "database", "active", "localhost", 0, true)

	registry := component.NewRegistry()
	container := di.NewContainer()

	// Should not panic and should not append ":0"
	s.DisplaySummary(registry, container, nil)
}

func TestDisplaySummaryZeroDuration(t *testing.T) {
	s := NewSummary("instant-svc", "0.0.1")
	s.SetStartupDuration(0)
	s.TrackRoute("GET", "/health", "HealthHandler")

	registry := component.NewRegistry()
	container := di.NewContainer()

	// Should render 0.00s without panic
	s.DisplaySummary(registry, container, nil)
}

func TestDisplaySummaryNilContainer(t *testing.T) {
	s := NewSummary("nil-container-svc", "1.0")
	s.SetStartupDuration(10 * time.Millisecond)

	registry := component.NewRegistry()
	// nil container
	s.DisplaySummary(registry, nil, nil)
}
