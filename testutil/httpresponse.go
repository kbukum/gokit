package testutil

import "net/http"

// FakeResponse is a programmed HTTP response served by [FakeHTTPServer].
//
// Construct it with [NewFakeResponse] and shape it with the WithHeader/WithBody
// options; the zero value is not usable directly.
type FakeResponse struct {
	status  int
	headers http.Header
	body    []byte
}

// FakeResponseOption configures a [FakeResponse] at construction time.
type FakeResponseOption func(*FakeResponse)

// NewFakeResponse builds a programmed response with the given status code,
// optionally shaped by headers and a body via options.
func NewFakeResponse(status int, opts ...FakeResponseOption) *FakeResponse {
	r := &FakeResponse{status: status, headers: make(http.Header)}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// WithHeader adds a response header. It may be used multiple times, including
// repeated names to emit multi-valued headers.
func WithHeader(name, value string) FakeResponseOption {
	return func(r *FakeResponse) { r.headers.Add(name, value) }
}

// WithBody sets the response body bytes. The bytes are copied, so later
// mutation or reuse of the caller's slice cannot alter the programmed response
// or race with concurrent request handling.
func WithBody(body []byte) FakeResponseOption {
	return func(r *FakeResponse) {
		r.body = append([]byte(nil), body...)
	}
}

// WithBodyString sets the response body from a string.
func WithBodyString(body string) FakeResponseOption {
	return func(r *FakeResponse) { r.body = []byte(body) }
}

// write emits the programmed status, headers, and body to w, returning any
// error from writing the body so the caller can surface a failed response
// rather than silently dropping it.
func (r *FakeResponse) write(w http.ResponseWriter) error {
	for name, values := range r.headers {
		for _, value := range values {
			w.Header().Add(name, value)
		}
	}
	w.WriteHeader(r.status)
	_, err := w.Write(r.body)
	return err
}
