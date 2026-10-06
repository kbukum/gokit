package client

import (
	"errors"
	"io"
	"net/http"
)

// ErrTransport marks a failure of the HTTP exchange itself: the peer could not be reached, or the connection broke
// before the response ended. Connect wraps such causes, so errors.Is still finds the mark under any Connect code.
var ErrTransport = errors.New("connect client: transport failure")

// IsTransportFailure reports whether err came from a broken or unreachable connection rather than an answer from the
// peer. Caller cancellation, a clean end of stream and a misconfigured URL are not transport failures.
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

// transportBody marks response-body read failures, which is how a connection lost mid-stream surfaces.
type transportBody struct {
	io.ReadCloser
	req *http.Request
}

func (b *transportBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	return n, markTransport(b.req, err)
}
