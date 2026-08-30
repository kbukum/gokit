package testutil

import (
	"errors"
	"io"
	"net/http"
)

// CapturedRequest is the request recorded by [FakeHTTPServer] for assertions.
// It is a value snapshot taken when the request is served, safe to read after
// the client call returns.
type CapturedRequest struct {
	// Method is the HTTP method, e.g. "GET".
	Method string
	// Path is the request URL path, e.g. "/v1/health".
	Path string
	// RawQuery is the unparsed URL query string, e.g. "page=2".
	RawQuery string
	// Header holds the request headers as received.
	Header http.Header
	// Body is the fully-read request body.
	Body []byte
}

// HeaderValue returns the first value of name, matched case-insensitively,
// or the empty string when the header is absent.
func (c *CapturedRequest) HeaderValue(name string) string {
	return c.Header.Get(name)
}

// captureRequest snapshots r (with its already-read body) into a CapturedRequest.
func captureRequest(r *http.Request, body []byte) *CapturedRequest {
	return &CapturedRequest{
		Method:   r.Method,
		Path:     r.URL.Path,
		RawQuery: r.URL.RawQuery,
		Header:   r.Header.Clone(),
		Body:     body,
	}
}

// readBody fully reads and closes the request body, joining any read error with
// the error from closing the body so neither is silently dropped.
func readBody(r *http.Request) (body []byte, err error) {
	if r.Body == nil {
		return nil, nil
	}
	defer func() {
		err = errors.Join(err, r.Body.Close())
	}()
	return io.ReadAll(r.Body)
}
