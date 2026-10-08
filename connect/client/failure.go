package client

import (
	"errors"
	"io"
	"net/http"
	"strings"
)

// ErrTransport marks a failure of the HTTP exchange itself: the peer could not be reached, a gateway answered 502, 503
// or 504 instead of the RPC protocol, or the connection broke before the response ended. Connect wraps such causes,
// so errors.Is still finds the mark under any Connect code.
var ErrTransport = errors.New("connect client: transport failure")

// IsTransportFailure reports whether err came from a broken or unreachable connection rather than an answer from the
// peer. Caller cancellation, a clean end of stream, a misconfigured URL and a Connect error body are not transport
// failures.
func IsTransportFailure(err error) bool {
	return errors.Is(err, ErrTransport)
}

type transportError struct{ cause error }

func (e *transportError) Error() string   { return e.cause.Error() }
func (e *transportError) Unwrap() []error { return []error{ErrTransport, e.cause} }

// markTransport tags a network failure unless the caller abandoned the request, which says nothing about the peer.
func markTransport(req *http.Request, err error) error {
	if err == nil || err == io.EOF || req.Context().Err() != nil || IsTransportFailure(err) { //nolint:errorlint // io.EOF must stay unwrapped for readers
		return err
	}
	return &transportError{cause: err}
}

// gatewayOutage reports a 502, 503 or 504 response that carries no RPC error. connect-go builds an unmarked error from
// such a status, which would hide an outage behind a gateway. Only a Connect unary call can carry a protocol error at a
// non-200 status, as a JSON body; streams and gRPC treat every non-200 status as coming from an intermediary.
func gatewayOutage(req *http.Request, resp *http.Response) bool {
	switch resp.StatusCode {
	case http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
	default:
		return false
	}
	return !connectUnary(req) || !connectErrorBody(resp.Header.Get("Content-Type"))
}

// connectUnary reports a Connect unary request: a GET, or a POST whose content type is neither Connect streaming nor
// gRPC.
func connectUnary(req *http.Request) bool {
	contentType := strings.ToLower(req.Header.Get("Content-Type"))
	return !strings.HasPrefix(contentType, "application/connect+") && !strings.HasPrefix(contentType, "application/grpc")
}

// connectErrorBody matches the content types connect-go decodes as a Connect unary error.
func connectErrorBody(contentType string) bool {
	return contentType == "application/json" || contentType == "application/json; charset=utf-8"
}

// transportBody marks response-body read failures, which is how a connection lost mid-stream surfaces.
type transportBody struct {
	io.ReadCloser
	req *http.Request
}

func (b *transportBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	return n, markTransport(b.req, err)
}
