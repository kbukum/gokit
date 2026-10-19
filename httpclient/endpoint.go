package httpclient

import (
	"errors"
	"net/http"
	"net/url"

	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/security"
)

// endpointTransport checks every outgoing request against the policy's origin grants without DNS; the policy's DialContext checks resolved addresses on each new connection.
type endpointTransport struct {
	policy *security.EndpointPolicy
	next   http.RoundTripper
}

func (t *endpointTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	checked, err := checkedEndpointRequest(t.policy, req)
	if err != nil {
		return nil, err
	}
	return t.next.RoundTrip(checked)
}

func (t *endpointTransport) CloseIdleConnections() {
	if closer, ok := t.next.(interface{ CloseIdleConnections() }); ok {
		closer.CloseIdleConnections()
	}
}

// checkedEndpointRequest normalizes the request's origin and path against the policy grants. A query built from [Request.Query] is kept; the policy itself never sees it, so raw query text in a path is still refused.
func checkedEndpointRequest(policy *security.EndpointPolicy, req *http.Request) (*http.Request, error) {
	if req == nil || req.URL == nil || (req.Host != "" && req.Host != req.URL.Host) {
		return nil, apperrors.InvalidInput("endpoint", "A request with its original authority is required")
	}
	target := *req.URL
	query := target.RawQuery
	target.RawQuery, target.ForceQuery = "", false
	normalized, err := policy.Normalize(target.String())
	if err != nil {
		return nil, err
	}
	u, err := url.Parse(normalized)
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	u.RawQuery = query
	checked := req.Clone(req.Context())
	checked.URL, checked.Host = u, u.Host
	return checked, nil
}

func refuseEndpointRedirect(*http.Request, []*http.Request) error {
	return apperrors.Forbidden("Endpoint redirects are not permitted")
}

// redactURLError removes the query from a transport error's URL, so query values never reach error text or logs.
func redactURLError(err error) error {
	var urlErr *url.Error
	if !errors.As(err, &urlErr) {
		return err
	}
	if u, parseErr := url.Parse(urlErr.URL); parseErr == nil && (u.RawQuery != "" || u.ForceQuery) {
		u.RawQuery, u.ForceQuery = "", false
		urlErr.URL = u.String()
	} else if parseErr != nil {
		urlErr.URL = "[redacted]"
	}
	return err
}
