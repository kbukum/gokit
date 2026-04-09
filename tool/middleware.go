package tool

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/kbukum/gokit/schema"
)

// Middleware wraps a [Callable] with additional behavior.
type Middleware func(Callable) Callable

// Chain composes multiple middlewares. The first middleware in the list
// is the outermost wrapper (executed first).
func Chain(middlewares ...Middleware) Middleware {
	return func(c Callable) Callable {
		for i := len(middlewares) - 1; i >= 0; i-- {
			c = middlewares[i](c)
		}
		return c
	}
}

// Apply wraps a Callable with the given middleware chain.
func Apply(c Callable, middlewares ...Middleware) Callable {
	return Chain(middlewares...)(c)
}

// WithLogging returns middleware that logs tool calls.
func WithLogging(logger *slog.Logger) Middleware {
	return func(next Callable) Callable {
		return &loggingCallable{next: next, logger: logger}
	}
}

type loggingCallable struct {
	next   Callable
	logger *slog.Logger
}

func (l *loggingCallable) Definition() Definition { return l.next.Definition() }

func (l *loggingCallable) Validate(input json.RawMessage) schema.ValidationResult {
	return l.next.Validate(input)
}

func (l *loggingCallable) Call(ctx *Context, input json.RawMessage) (*Result, error) {
	name := l.next.Definition().Name
	start := time.Now()
	l.logger.InfoContext(ctx, "tool call started", "tool", name)

	result, err := l.next.Call(ctx, input)
	elapsed := time.Since(start)

	if err != nil {
		l.logger.ErrorContext(ctx, "tool call failed", "tool", name, "duration", elapsed, "error", err)
	} else {
		l.logger.InfoContext(ctx, "tool call completed", "tool", name, "duration", elapsed, "is_error", result.IsError)
	}
	return result, err
}

// WithTimeout returns middleware that enforces a timeout on tool calls.
func WithTimeout(d time.Duration) Middleware {
	return func(next Callable) Callable {
		return &timeoutCallable{next: next, timeout: d}
	}
}

type timeoutCallable struct {
	next    Callable
	timeout time.Duration
}

func (t *timeoutCallable) Definition() Definition { return t.next.Definition() }

func (t *timeoutCallable) Validate(input json.RawMessage) schema.ValidationResult {
	return t.next.Validate(input)
}

func (t *timeoutCallable) Call(ctx *Context, input json.RawMessage) (*Result, error) {
	newCtx, cancel := ctx.WithTimeout(t.timeout)
	defer cancel()
	return t.next.Call(newCtx, input)
}

// WithRecover returns middleware that recovers from panics in tool handlers.
func WithRecover() Middleware {
	return func(next Callable) Callable {
		return &recoverCallable{next: next}
	}
}

type recoverCallable struct {
	next Callable
}

func (r *recoverCallable) Definition() Definition { return r.next.Definition() }

func (r *recoverCallable) Validate(input json.RawMessage) schema.ValidationResult {
	return r.next.Validate(input)
}

func (r *recoverCallable) Call(ctx *Context, input json.RawMessage) (result *Result, err error) {
	defer func() {
		if v := recover(); v != nil {
			err = fmt.Errorf("tool %q panicked: %v", r.next.Definition().Name, v)
		}
	}()
	return r.next.Call(ctx, input)
}

// WithValidation returns middleware that validates input against the tool's
// schema before executing. Invalid input returns an error Result without
// calling the underlying tool.
func WithValidation() Middleware {
	return func(next Callable) Callable {
		return &validationCallable{next: next}
	}
}

type validationCallable struct {
	next Callable
}

func (v *validationCallable) Definition() Definition { return v.next.Definition() }

func (v *validationCallable) Validate(input json.RawMessage) schema.ValidationResult {
	return v.next.Validate(input)
}

func (v *validationCallable) Call(ctx *Context, input json.RawMessage) (*Result, error) {
	vr := v.next.Validate(input)
	if !vr.Valid {
		msg := "validation failed:"
		for _, e := range vr.Errors {
			msg += " " + e.Error()
		}
		return ErrorResult(msg), nil
	}
	return v.next.Call(ctx, input)
}

// WithResultLimit returns middleware that truncates result content
// exceeding the given byte limit.
func WithResultLimit(maxBytes int) Middleware {
	return func(next Callable) Callable {
		return &resultLimitCallable{next: next, maxBytes: maxBytes}
	}
}

type resultLimitCallable struct {
	next     Callable
	maxBytes int
}

func (rl *resultLimitCallable) Definition() Definition { return rl.next.Definition() }

func (rl *resultLimitCallable) Validate(input json.RawMessage) schema.ValidationResult {
	return rl.next.Validate(input)
}

func (rl *resultLimitCallable) Call(ctx *Context, input json.RawMessage) (*Result, error) {
	result, err := rl.next.Call(ctx, input)
	if err != nil || result == nil {
		return result, err
	}
	if rl.maxBytes > 0 && len(result.Content) > rl.maxBytes {
		result.Content = result.Content[:rl.maxBytes] + "\n... (truncated)"
		result.SetMeta("truncated", true)
	}
	return result, nil
}
