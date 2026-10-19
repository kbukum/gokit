package middleware

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kbukum/gokit/logging"
)

func TestRequestLoggerLogsTheRouteByStatus(t *testing.T) {
	var output bytes.Buffer
	log, err := logging.New(&logging.Config{Level: "debug", Format: "json"}, "test", logging.WithWriter(&output))
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	for pattern, status := range map[string]int{"GET /ok/{id}": http.StatusOK, "GET /bad": http.StatusBadRequest, "GET /fail": http.StatusInternalServerError, "GET /health": http.StatusOK} {
		mux.HandleFunc(pattern, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(status) })
	}
	handler := RequestLogger(log)(mux)
	want := map[string]string{"GET /ok/{id}": "DEBUG", "GET /bad": "WARN", "GET /fail": "ERROR", "unmatched": "WARN"}
	for _, target := range []string{"/ok/private-id?token=private-query", "/bad", "/fail", "/health", "/private-missing"} {
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequestWithContext(t.Context(), http.MethodGet, target, http.NoBody))
	}
	if strings.Contains(output.String(), "private-") {
		t.Fatalf("raw request input logged: %s", output.String())
	}
	got := map[string]string{}
	for line := range strings.Lines(output.String()) {
		var record struct{ Level, Path string }
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatal(err)
		}
		got[record.Path] = record.Level
	}
	if len(got) != len(want) {
		t.Fatalf("logged routes = %v, want %v", got, want)
	}
	for path, level := range want {
		if got[path] != level {
			t.Errorf("%s logged at %q, want %q", path, got[path], level)
		}
	}
}
