package middleware

import "net/http"

// statusWriter wraps http.ResponseWriter to capture the status code.
// It delegates Flush and Unwrap so HTTP/2 streaming and gRPC work correctly.
type statusWriter struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func newStatusWriter(w http.ResponseWriter) *statusWriter {
	return &statusWriter{ResponseWriter: w, status: http.StatusOK}
}

func (sw *statusWriter) WriteHeader(code int) {
	if !sw.wroteHeader {
		sw.status = code
		sw.wroteHeader = true
	}
	sw.ResponseWriter.WriteHeader(code)
}

func (sw *statusWriter) Write(b []byte) (int, error) {
	if !sw.wroteHeader {
		sw.wroteHeader = true
	}
	return sw.ResponseWriter.Write(b)
}

// Flush implements http.Flusher, required for streaming, SSE, and gRPC responses.
func (sw *statusWriter) Flush() {
	if f, ok := sw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Unwrap returns the underlying ResponseWriter so http.ResponseController
// (Go 1.20+) can discover optional interfaces on the original writer.
func (sw *statusWriter) Unwrap() http.ResponseWriter {
	return sw.ResponseWriter
}
