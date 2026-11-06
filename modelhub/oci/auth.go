package oci

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	apperr "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/modelhub/internal/fetch"
	"github.com/kbukum/gokit/security"
)

// ReasonAuthChallenge marks a registry auth challenge the client cannot follow.
const ReasonAuthChallenge = "OCI_AUTH_CHALLENGE"

const (
	defaultTokenLifetime = 60 * time.Second
	tokenSkew            = 10 * time.Second
	maxChallengeBytes    = 4 << 10
)

// challenge is the registry's bearer-token endpoint, learned from GET /v2/ or from a repository's 401.
type challenge struct {
	anonymous bool
	realm     string
	service   string
}

type token struct {
	value   string
	expires time.Time
}

// tokenCache holds a bounded number of pull tokens by scope.
type tokenCache struct {
	mu        sync.Mutex
	max       int
	tokens    map[string]token
	challenge *challenge
}

func newTokenCache(maxTokens int) *tokenCache {
	return &tokenCache{max: maxTokens, tokens: make(map[string]token)}
}

func (c *tokenCache) get(scope string, now time.Time) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	t, ok := c.tokens[scope]
	if !ok || !now.Before(t.expires) {
		delete(c.tokens, scope)
		return "", false
	}
	return t.value, true
}

func (c *tokenCache) put(scope string, t token, now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.tokens[scope]; !ok && len(c.tokens) >= c.max {
		for k, v := range c.tokens {
			if !now.Before(v.expires) {
				delete(c.tokens, k)
			}
		}
		for len(c.tokens) >= c.max {
			var oldest string
			for k, v := range c.tokens {
				if oldest == "" || v.expires.Before(c.tokens[oldest].expires) {
					oldest = k
				}
			}
			delete(c.tokens, oldest)
		}
	}
	c.tokens[scope] = t
}

func (c *tokenCache) drop(scope string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.tokens, scope)
}

func (c *tokenCache) learned() *challenge {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.challenge
}

func (c *tokenCache) learn(ch *challenge) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.challenge = ch
}

func scopeFor(repo string) string { return "repository:" + repo + ":pull" }

// authorized sends a read of repo with its pull token. On a 401 it retries once: with a fresh token when one was
// sent (expired or revoked), or after learning the repository's bearer challenge when the read went anonymously
// because GET /v2/ had not asked for a token.
func authorized[T any](ctx context.Context, c *Client, repo string, send func(headers map[string]string) (T, *fetch.Response, error)) (T, *fetch.Response, error) {
	for retried := false; ; retried = true {
		headers, err := c.authHeaders(ctx, repo)
		if err != nil {
			var zero T
			return zero, nil, err
		}
		value, resp, err := send(headers)
		if err == nil || retried || fetch.StatusOf(err) != http.StatusUnauthorized {
			return value, resp, err
		}
		if headers["Authorization"] != "" {
			c.tokens.drop(scopeFor(repo))
			continue
		}
		ch, chErr := c.parseChallenge(resp.Header("WWW-Authenticate"))
		if chErr != nil {
			return value, resp, err
		}
		c.tokens.learn(ch)
	}
}

// authHeaders returns the Authorization header for a pull of repo, fetching a token when none is cached.
func (c *Client) authHeaders(ctx context.Context, repo string) (map[string]string, error) {
	scope := scopeFor(repo)
	if value, ok := c.tokens.get(scope, c.fetch.Now()); ok {
		return map[string]string{"Authorization": "Bearer " + value}, nil
	}
	ch, err := c.learnChallenge(ctx)
	if err != nil {
		return nil, err
	}
	if ch.anonymous {
		return map[string]string{}, nil
	}
	resp, err := c.fetch.Get(ctx, fetch.Request{URL: ch.realm, Query: map[string]string{"service": ch.service, "scope": scope}, MaxBytes: maxTokenBytes})
	if err != nil {
		return nil, err
	}
	var body struct {
		Token       string `json:"token"`
		AccessToken string `json:"access_token"`
		ExpiresIn   int64  `json:"expires_in"`
	}
	if err := json.Unmarshal(resp.Body, &body); err != nil {
		return nil, apperr.InvalidFormat("token", "a registry token response").WithReason(ReasonAuthChallenge)
	}
	value := body.Token
	if value == "" {
		value = body.AccessToken
	}
	if value == "" || strings.ContainsAny(value, " \t\r\n") {
		return nil, apperr.InvalidFormat("token", "a registry token").WithReason(ReasonAuthChallenge)
	}
	lifetime := defaultTokenLifetime
	if body.ExpiresIn > 0 && body.ExpiresIn < int64(24*time.Hour/time.Second) {
		lifetime = time.Duration(body.ExpiresIn) * time.Second
	}
	now := c.fetch.Now()
	c.tokens.put(scope, token{value: value, expires: now.Add(max(lifetime-tokenSkew, lifetime/2))}, now)
	return map[string]string{"Authorization": "Bearer " + value}, nil
}

// learnChallenge reads the registry's auth challenge from GET /v2/ once per client.
func (c *Client) learnChallenge(ctx context.Context) (*challenge, error) {
	if ch := c.tokens.learned(); ch != nil {
		return ch, nil
	}
	resp, err := c.fetch.Get(ctx, fetch.Request{URL: c.registry + "/v2/", MaxBytes: maxChallengeBytes})
	var ch *challenge
	switch {
	case err == nil:
		ch = &challenge{anonymous: true}
	case fetch.StatusOf(err) == http.StatusUnauthorized:
		ch, err = c.parseChallenge(resp.Header("WWW-Authenticate"))
		if err != nil {
			return nil, err
		}
	default:
		return nil, err
	}
	c.tokens.learn(ch)
	return ch, nil
}

// parseChallenge reads a Bearer challenge. The realm must be an HTTPS URL, or HTTP only when the registry is.
func (c *Client) parseChallenge(header string) (*challenge, error) {
	refuse := apperr.InvalidFormat("www-authenticate", "a Bearer challenge").WithReason(ReasonAuthChallenge)
	if len(header) > maxChallengeBytes {
		return nil, refuse
	}
	scheme, rest, _ := strings.Cut(strings.TrimSpace(header), " ")
	if !strings.EqualFold(scheme, "Bearer") {
		return nil, refuse
	}
	params := parseParams(rest)
	realm, err := security.NormalizeEndpointURL(params["realm"])
	if err != nil || params["service"] == "" {
		return nil, refuse
	}
	if strings.HasPrefix(realm, "http://") && !strings.HasPrefix(c.registry, "http://") {
		return nil, refuse
	}
	return &challenge{realm: realm, service: params["service"]}, nil
}

// parseParams reads comma-separated key="value" pairs.
func parseParams(s string) map[string]string {
	out := map[string]string{}
	for s = strings.TrimSpace(s); s != ""; s = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(s), ",")) {
		key, rest, ok := strings.Cut(s, "=")
		if !ok {
			break
		}
		key = strings.ToLower(strings.TrimSpace(key))
		var value string
		if strings.HasPrefix(rest, `"`) {
			end := strings.Index(rest[1:], `"`)
			if end < 0 {
				break
			}
			value, s = rest[1:end+1], rest[end+2:]
		} else {
			value, s, _ = strings.Cut(rest, ",")
		}
		if _, dup := out[key]; !dup {
			out[key] = strings.TrimSpace(value)
		}
	}
	return out
}
