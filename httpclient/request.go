package httpclient

import (
	"io"
	"net/http"

	"github.com/kbukum/gokit/httpclient/sse"
)

// Request describes an outbound HTTP request.
type Request struct {
	// Method is the HTTP method (GET, POST, PUT, PATCH, DELETE, etc).
	Method string
	// Path is appended to the client's BaseURL. Can be a full URL if BaseURL is empty.
	Path string
	// Headers are request-specific headers (merged with client defaults).
	Headers map[string]string
	// Query are URL query parameters.
	Query map[string]string
	// Body is the request body. Accepts io.Reader, []byte, string, or any value
	// that will be JSON-encoded.
	Body any
	// Auth overrides the client-level auth for this request.
	Auth *AuthConfig
}

// Response is the result of an HTTP request.
type Response struct {
	// StatusCode is the HTTP status code.
	StatusCode int
	// Headers are the response headers.
	Headers map[string]string
	// Body is the raw response body.
	Body []byte
}

// IsSuccess returns true if the status code is 2xx.
func (r *Response) IsSuccess() bool {
	return r.StatusCode >= 200 && r.StatusCode < 300
}

// IsError returns true if the status code is 4xx or 5xx.
func (r *Response) IsError() bool {
	return r.StatusCode >= 400
}

// StreamResponse wraps a streaming HTTP response.
type StreamResponse struct {
	// StatusCode is the HTTP status code.
	StatusCode int
	// Headers are the response headers.
	Headers map[string]string
	// SSE is the Server-Sent Events reader (for text/event-stream responses).
	SSE sse.Reader
	// Body is the raw streaming body (for non-SSE streams).
	Body io.ReadCloser
	// rawResp holds the original response for cleanup.
	rawResp *http.Response
}

// Close releases all resources associated with the stream.
func (r *StreamResponse) Close() error {
	if r.SSE != nil {
		return r.SSE.Close()
	}
	if r.Body != nil {
		return r.Body.Close()
	}
	if r.rawResp != nil && r.rawResp.Body != nil {
		return r.rawResp.Body.Close()
	}
	return nil
}
