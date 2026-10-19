package client

import (
	"errors"
	"time"

	"connectrpc.com/connect"
)

// InterceptorOption adds an interceptor to [Interceptors].
type InterceptorOption func(*interceptorOptions)

type interceptorOptions struct {
	availability    *Availability
	availabilitySet bool
	unary           *time.Duration
	firstMessage    *time.Duration
	firstOpts       []FirstMessageOption
}

// WithAvailability records call outcomes in availability. It goes first, so a timeout counts as an outage.
func WithAvailability(availability *Availability) InterceptorOption {
	return func(o *interceptorOptions) { o.availability, o.availabilitySet = availability, true }
}

// WithUnaryTimeout bounds each unary call through [UnaryTimeoutInterceptor]. The limit must be positive; leave the option out for no timeout.
func WithUnaryTimeout(limit time.Duration) InterceptorOption {
	return func(o *interceptorOptions) { o.unary = &limit }
}

// WithFirstMessageTimeout bounds the wait for a stream's first message through [FirstMessageTimeoutInterceptor]. The limit must be positive; leave the option out for no timeout.
func WithFirstMessageTimeout(limit time.Duration, opts ...FirstMessageOption) InterceptorOption {
	return func(o *interceptorOptions) {
		o.firstMessage = &limit
		o.firstOpts = opts
	}
}

// Interceptors returns the interceptors the options ask for: availability first, then the unary and first-message timeouts. Each part is optional, so a client may use timeouts alone. Pass the result to connect.WithInterceptors after any tracing or header interceptors.
func Interceptors(opts ...InterceptorOption) ([]connect.Interceptor, error) {
	var o interceptorOptions
	for _, opt := range opts {
		if opt == nil {
			return nil, errors.New("connect client: nil interceptor option")
		}
		opt(&o)
	}
	var chain []connect.Interceptor
	if o.availabilitySet {
		if o.availability == nil {
			return nil, errors.New("connect client: availability is nil")
		}
		chain = append(chain, o.availability)
	}
	if o.unary != nil {
		unary, err := UnaryTimeoutInterceptor(*o.unary)
		if err != nil {
			return nil, err
		}
		chain = append(chain, unary)
	}
	if o.firstMessage != nil {
		first, err := FirstMessageTimeoutInterceptor(*o.firstMessage, o.firstOpts...)
		if err != nil {
			return nil, err
		}
		chain = append(chain, first)
	}
	return chain, nil
}
