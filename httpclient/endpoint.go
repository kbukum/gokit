package httpclient

import (
	"errors"
	"net/http"
	"net/url"
	"slices"
	"strings"

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

const maxEndpointRedirects = 10

// redirectSafeHeaders are the only request headers carried to a redirect target on another origin; credentials and
// custom auth headers are dropped.
var redirectSafeHeaders = [...]string{"Accept", "Range", "User-Agent"}

// endpointRedirect follows up to limit GET or HEAD redirects. endpointTransport checks each hop against the policy.
// Headers are kept while every hop stays on the first request's exact origin; from the first hop that leaves it, only
// redirectSafeHeaders are carried.
func endpointRedirect(limit int) func(*http.Request, []*http.Request) error {
	return func(req *http.Request, via []*http.Request) error {
		if limit == 0 || len(via) > limit || len(via) == 0 {
			return apperrors.Forbidden("Endpoint redirects are not permitted")
		}
		if method := via[0].Method; method != http.MethodGet && method != http.MethodHead {
			return apperrors.Forbidden("Endpoint redirects are permitted only for reads")
		}
		if sameOriginChain(req, via) {
			return nil
		}
		for name := range req.Header {
			if !slices.Contains(redirectSafeHeaders[:], name) {
				req.Header.Del(name)
			}
		}
		return nil
	}
}

func sameOriginChain(req *http.Request, via []*http.Request) bool {
	origin := originOf(via[0].URL)
	for _, hop := range via[1:] {
		if originOf(hop.URL) != origin {
			return false
		}
	}
	return originOf(req.URL) == origin
}

func originOf(u *url.URL) string {
	port := u.Port()
	if port == "" {
		switch strings.ToLower(u.Scheme) {
		case "https":
			port = "443"
		case "http":
			port = "80"
		}
	}
	return strings.ToLower(u.Scheme) + "://" + strings.ToLower(u.Hostname()) + ":" + port
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
