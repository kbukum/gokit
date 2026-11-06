package fetch

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	apperr "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/httpclient"
	"github.com/kbukum/gokit/resilience"
	"github.com/kbukum/gokit/util"
)

// ReasonResponseTooLarge marks a response that exceeded its read bound.
const ReasonResponseTooLarge = "RESPONSE_TOO_LARGE"

// maxRetryAfter caps a server's Retry-After hint; the retry budget still bounds the total wait.
const maxRetryAfter = 5 * time.Minute

// Config configures a [Client].
type Config struct {
	// HTTP sends every request. It must have an endpoint policy granting the source hosts, no base URL, and no retry
	// policy: the client is the single retry owner.
	HTTP *httpclient.Adapter
	// Clock drives retry backoff and dates Retry-After hints. Nil uses the monotonic system clock.
	Clock util.TimerClock
	// Retry bounds retries of idempotent reads. The zero value uses [DefaultRetry]. RetryIf, MinimumDelay and Clock
	// are owned by the client and overwritten.
	Retry resilience.RetryConfig
}

// DefaultRetry is three attempts with jittered exponential backoff inside a 30 second budget.
func DefaultRetry() resilience.RetryConfig {
	return resilience.RetryConfig{
		MaxAttempts:    3,
		InitialBackoff: 200 * time.Millisecond,
		MaxBackoff:     5 * time.Second,
		MaxElapsedTime: 30 * time.Second,
		Strategy:       resilience.ExponentialBackoff,
		BackoffFactor:  2,
		Jitter:         0.2,
	}
}

// Client performs bounded idempotent reads for one named source.
type Client struct {
	service string
	http    *httpclient.Adapter
	clock   util.TimerClock
	retry   resilience.RetryConfig
}

// Request is one idempotent GET.
type Request struct {
	// URL is the absolute request URL without a query.
	URL string
	// Query holds the query parameters.
	Query map[string]string
	// Headers holds request headers.
	Headers map[string]string
	// MaxBytes bounds the buffered body of [Client.Get].
	MaxBytes int64
}

// Response is a buffered answer.
type Response struct {
	Status  int
	Body    []byte
	headers map[string]string
}

// Header returns one response header value.
func (r *Response) Header(name string) string {
	if r == nil {
		return ""
	}
	return r.headers[http.CanonicalHeaderKey(name)]
}

// New validates cfg and builds a client for the named source.
func New(service string, cfg Config) (*Client, error) {
	if cfg.HTTP == nil {
		return nil, apperr.MissingField("http")
	}
	hc := cfg.HTTP.GetConfig()
	if hc.BaseURL != "" {
		return nil, apperr.InvalidInput("http", "The adapter must not have a base URL")
	}
	if hc.EndpointPolicy == nil {
		// Sources hand out follow-up URLs (token realms, redirects, CDN links); the policy confines them to granted hosts.
		return nil, apperr.InvalidInput("http", "The adapter must have an endpoint policy granting the source hosts")
	}
	if hc.ResiliencePolicy != nil && hc.ResiliencePolicy.Retry != nil {
		return nil, apperr.InvalidInput("http", "The adapter must not retry; the source client owns retries")
	}
	clock := cfg.Clock
	if util.IsNil(clock) {
		clock = util.MonotonicClock{}
	}
	retry := cfg.Retry
	if retry.MaxAttempts == 0 {
		retry = DefaultRetry()
	}
	if err := retry.Validate(); err != nil {
		return nil, err
	}
	retry.Clock = clock
	retry.RetryIf = isRetryable
	retry.MinimumDelay = retryAfter
	return &Client{service: service, http: cfg.HTTP, clock: clock, retry: retry}, nil
}

// Now reads the client's clock.
func (c *Client) Now() time.Time { return c.clock.Now() }

// Get performs a retried GET and reads at most req.MaxBytes of body, stopping as soon as the bound is passed. On an
// HTTP failure the response is returned alongside the error, headers only, so callers can read its headers.
func (c *Client) Get(ctx context.Context, req Request) (*Response, error) {
	if req.MaxBytes <= 0 {
		return nil, apperr.InvalidInput("max_bytes", "must be positive")
	}
	var last *Response
	resp, err := resilience.Retry(ctx, c.retry, func() (*Response, error) {
		stream, err := c.http.DoStream(ctx, httpclient.Request{Method: http.MethodGet, Path: req.URL, Query: req.Query, Headers: req.Headers})
		if err != nil {
			last = streamStatus(err)
			return nil, c.classify(ctx, err, last)
		}
		defer func() { _ = stream.Close() }()
		last = &Response{Status: stream.StatusCode, headers: canonical(stream.Headers)}
		if stream.Body == nil {
			return nil, apperr.InvalidFormat("body", "a byte stream").WithDetail("service", c.service)
		}
		body, err := io.ReadAll(io.LimitReader(stream.Body, req.MaxBytes+1))
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return nil, ctxErr
			}
			return nil, apperr.ConnectionFailed(c.service).WithCause(err).WithRetryable(true)
		}
		if int64(len(body)) > req.MaxBytes {
			return nil, tooLarge(req.MaxBytes)
		}
		return &Response{Status: last.Status, Body: body, headers: last.headers}, nil
	})
	if err != nil {
		return last, unwrapExhausted(err)
	}
	return resp, nil
}

// Head performs a retried HEAD and returns the status and headers. On an HTTP failure the response is returned
// alongside the error.
func (c *Client) Head(ctx context.Context, req Request) (*Response, error) {
	var last *Response
	resp, err := resilience.Retry(ctx, c.retry, func() (*Response, error) {
		raw, err := c.http.Do(ctx, httpclient.Request{Method: http.MethodHead, Path: req.URL, Query: req.Query, Headers: req.Headers})
		if raw != nil {
			last = &Response{Status: raw.StatusCode, headers: canonical(raw.Headers)}
		}
		if err != nil {
			return nil, c.classify(ctx, err, last)
		}
		return last, nil
	})
	if err != nil {
		return last, unwrapExhausted(err)
	}
	return resp, nil
}

// canonical keys flattened headers by their canonical names.
func canonical(h map[string]string) map[string]string {
	out := make(map[string]string, len(h))
	for k, v := range h {
		out[http.CanonicalHeaderKey(k)] = v
	}
	return out
}

// Open performs a retried ranged GET for the first n bytes and returns a body that yields at most n bytes, even
// when the server ignores the range. The caller must close it. On an HTTP status error it also returns the last
// response, headers only, so callers can read challenges.
func (c *Client) Open(ctx context.Context, req Request, n int64) (io.ReadCloser, *Response, error) {
	if n <= 0 {
		return nil, nil, apperr.InvalidInput("prefix", "must be positive")
	}
	headers := make(map[string]string, len(req.Headers)+1)
	for k, v := range req.Headers {
		headers[k] = v
	}
	headers["Range"] = "bytes=0-" + strconv.FormatInt(n-1, 10)
	var last *Response
	body, err := resilience.Retry(ctx, c.retry, func() (io.ReadCloser, error) {
		stream, err := c.http.DoStream(ctx, httpclient.Request{Method: http.MethodGet, Path: req.URL, Query: req.Query, Headers: headers})
		if err != nil {
			last = streamStatus(err)
			return nil, c.classify(ctx, err, last)
		}
		last = nil
		if stream.Body == nil {
			_ = stream.Close()
			return nil, apperr.InvalidFormat("body", "a byte stream").WithDetail("service", c.service)
		}
		return prefix{Reader: io.LimitReader(stream.Body, n), stream: stream}, nil
	})
	if err != nil {
		return nil, last, unwrapExhausted(err)
	}
	return body, nil, nil
}

// streamStatus keeps the headers of a streaming request's status error, or returns nil.
func streamStatus(err error) *Response {
	httpErr, ok := errors.AsType[*httpclient.Error](err)
	if !ok || httpErr.StatusCode == 0 {
		return nil
	}
	headers := make(map[string]string, len(httpErr.Header))
	for k, v := range httpErr.Header {
		if len(v) > 0 {
			headers[http.CanonicalHeaderKey(k)] = v[0]
		}
	}
	return &Response{Status: httpErr.StatusCode, headers: headers}
}

type prefix struct {
	io.Reader
	stream *httpclient.StreamResponse
}

func (p prefix) Close() error { return p.stream.Close() }

// StatusOf returns the HTTP status recorded on a source error, or 0.
func StatusOf(err error) int {
	app, ok := apperr.AsAppError(err)
	if !ok {
		return 0
	}
	status, _ := app.Details["status"].(int)
	return status
}

func (c *Client) classify(ctx context.Context, err error, resp *Response) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	var httpErr *httpclient.Error
	if !errors.As(err, &httpErr) {
		return err
	}
	// A policy refusal, such as an ungranted redirect target, is final and keeps its own type.
	var refusal *apperr.AppError
	if errors.As(httpErr.Err, &refusal) {
		return refusal
	}
	switch {
	case httpErr.StatusCode > 0:
		return c.statusError(httpErr.StatusCode, resp)
	case httpErr.Code == httpclient.ErrCodeTimeout:
		return apperr.Timeout(c.service).WithCause(httpErr.Err)
	case httpErr.Code == httpclient.ErrCodeResponseTooLarge:
		return tooLarge(0)
	case httpErr.Code == httpclient.ErrCodeConnection:
		return apperr.ConnectionFailed(c.service).WithCause(httpErr.Err)
	default:
		return apperr.ExternalServiceError(c.service, errors.New(httpErr.Message))
	}
}

func (c *Client) statusError(status int, resp *Response) *apperr.AppError {
	var e *apperr.AppError
	switch {
	case status == http.StatusNotFound:
		e = apperr.NotFound(c.service+" resource", "")
	case status == http.StatusUnauthorized:
		e = apperr.Unauthorized(c.service + " requires authorization")
	case status == http.StatusForbidden:
		e = apperr.Forbidden(c.service + " refused access")
	case status == http.StatusTooManyRequests:
		e = apperr.RateLimited()
	case status == http.StatusRequestTimeout || status == http.StatusBadGateway || status == http.StatusServiceUnavailable || status == http.StatusGatewayTimeout:
		e = apperr.ServiceUnavailable(c.service)
	case status >= http.StatusInternalServerError:
		e = apperr.ExternalServiceError(c.service, nil).WithRetryable(true)
	default:
		e = apperr.ExternalServiceError(c.service, nil)
	}
	e = e.WithDetail("status", status)
	if e.Retryable && resp != nil {
		if d := parseRetryAfter(resp.Header("Retry-After"), c.clock.Now()); d > 0 {
			e = e.WithRetryAfter(d)
		}
	}
	return e
}

func tooLarge(limit int64) *apperr.AppError {
	e := apperr.New(apperr.ErrCodeExternalService, "The source response exceeded its size bound.").WithReason(ReasonResponseTooLarge)
	if limit > 0 {
		e = e.WithDetail("max_bytes", limit)
	}
	return e
}

func parseRetryAfter(value string, now time.Time) time.Duration {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	var d time.Duration
	if secs, err := strconv.ParseInt(value, 10, 64); err == nil {
		if secs <= 0 {
			return 0
		}
		if secs > int64(maxRetryAfter/time.Second) {
			return maxRetryAfter
		}
		d = time.Duration(secs) * time.Second
	} else if at, err := http.ParseTime(value); err == nil {
		d = at.Sub(now)
	}
	return min(max(d, 0), maxRetryAfter)
}

func isRetryable(err error) bool {
	app, ok := apperr.AsAppError(err)
	return ok && app.Retryable
}

func retryAfter(err error) (time.Duration, error) {
	if app, ok := apperr.AsAppError(err); ok {
		return app.RetryAfter, nil
	}
	return 0, nil
}

// unwrapExhausted returns the last attempt's typed error when retries ran out, keeping the retry outcome visible.
func unwrapExhausted(err error) error {
	if !errors.Is(err, resilience.ErrMaxRetriesExceeded) {
		return err
	}
	if app, ok := apperr.AsAppError(errors.Unwrap(err)); ok {
		return app.WithDetail("retries_exhausted", true)
	}
	return fmt.Errorf("source retries exhausted: %w", err)
}
