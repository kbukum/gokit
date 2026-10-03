package observability

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// RuntimeMetricsHandler exposes Go runtime and process gauges through an instance-owned registry, without global collector registration.
func RuntimeMetricsHandler() (http.Handler, error) {
	registry := prometheus.NewRegistry()
	for _, collector := range []prometheus.Collector{collectors.NewGoCollector(), collectors.NewProcessCollector(collectors.ProcessCollectorOpts{})} {
		if err := registry.Register(collector); err != nil {
			return nil, err
		}
	}
	return promhttp.HandlerFor(registry, promhttp.HandlerOpts{}), nil
}

// PrometheusHandler returns the HTTP handler that exposes Prometheus metrics.
func PrometheusHandler() http.Handler {
	return promhttp.Handler()
}

// RegisterPrometheusEndpoint registers a metrics endpoint on mux.
func RegisterPrometheusEndpoint(mux *http.ServeMux, path string) {
	mux.Handle(path, PrometheusHandler())
}
