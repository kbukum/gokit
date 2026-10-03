package server

import (
	"go.opentelemetry.io/otel/metric"

	"github.com/kbukum/gokit/server/middleware"
)

// Instrument enables HTTP metrics with a composition-owned meter. Configure before ApplyMiddleware or ApplyDefaults.
func (s *Server) Instrument(meter metric.Meter) error {
	instrument, err := middleware.PrometheusMetrics("gokit.server", meter)
	if err != nil {
		return err
	}
	s.metricsMW = instrument
	return nil
}
