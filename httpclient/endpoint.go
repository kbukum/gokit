package httpclient

import (
	"net/http"
	"net/url"

	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/security"
)

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

func checkedEndpointRequest(policy *security.EndpointPolicy, req *http.Request) (*http.Request, error) {
	if req == nil || req.URL == nil || (req.Host != "" && req.Host != req.URL.Host) {
		return nil, apperrors.InvalidInput("endpoint", "A request with its original authority is required")
	}
	normalized, err := policy.Validate(req.Context(), req.URL.String())
	if err != nil {
		return nil, err
	}
	u, err := url.Parse(normalized)
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	checked := req.Clone(req.Context())
	checked.URL, checked.Host = u, u.Host
	return checked, nil
}

func refuseEndpointRedirect(*http.Request, []*http.Request) error {
	return apperrors.Forbidden("Endpoint redirects are not permitted")
}
