package httpclient

import (
	"context"

	"github.com/kbukum/gokit/provider"
	"github.com/kbukum/gokit/resilience"
)

// ClientProvider wraps a Client as a provider.RequestResponse[Request, *Response].
// This allows the HTTP client to participate in the provider framework —
// composable with WithResilience(), selectable via Manager, pipelineable.
//
// The existing Config.Retry/CircuitBreaker/RateLimiter still works independently.
// Use this adapter when you want provider-level composition instead.
type ClientProvider struct {
	name   string
	client *Client
}

// NewProvider wraps an existing Client as a RequestResponse provider.
func NewProvider(name string, client *Client) *ClientProvider {
	return &ClientProvider{name: name, client: client}
}

// Name returns the provider's unique name.
func (p *ClientProvider) Name() string { return p.name }

// IsAvailable checks if the client's circuit breaker (if any) is not open.
func (p *ClientProvider) IsAvailable(_ context.Context) bool {
	if p.client.cb != nil {
		return p.client.cb.State() != resilience.StateOpen
	}
	return true
}

// Execute sends an HTTP request via Do() and returns the response.
func (p *ClientProvider) Execute(ctx context.Context, input Request) (*Response, error) {
	return p.client.Do(ctx, input)
}

// Client returns the underlying *Client for advanced use cases (e.g., DoStream).
func (p *ClientProvider) Client() *Client { return p.client }

// compile-time checks
var _ provider.RequestResponse[Request, *Response] = (*ClientProvider)(nil)
