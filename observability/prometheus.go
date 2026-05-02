package observability

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// PrometheusHandler returns the HTTP handler that exposes Prometheus metrics.
func PrometheusHandler() http.Handler {
	return promhttp.Handler()
}

// RegisterPrometheusEndpoint registers a metrics endpoint on mux.
func RegisterPrometheusEndpoint(mux *http.ServeMux, path string) {
	mux.Handle(path, PrometheusHandler())
}
