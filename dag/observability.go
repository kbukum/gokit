package dag

import (
	"context"
	"time"

	"github.com/kbukum/gokit/logging"
	"github.com/kbukum/gokit/observability"
)

// WithTracing wraps a Node with OpenTelemetry span creation. Each execution creates a span named "{prefix}.{nodeName}".
func WithTracing(node Node, prefix string) Node {
	return &tracingNode{inner: node, prefix: prefix}
}

type tracingNode struct {
	inner  Node
	prefix string
}

func (n *tracingNode) Name() string { return n.inner.Name() }

func (n *tracingNode) Run(ctx context.Context, state *State) (any, error) {
	spanName := n.prefix + "." + n.inner.Name()
	ctx, span := observability.StartSpan(ctx, spanName)
	defer span.End()

	observability.SetSpanAttributes(ctx, observability.StringAttribute("dag.node", n.inner.Name()))

	result, err := n.inner.Run(ctx, state)
	if err != nil {
		observability.SetSpanError(ctx, err)
	}

	return result, err
}

// WithMetrics wraps a Node with metric recording. Records operation count, duration, and errors.
func WithMetrics(node Node, metrics *observability.Metrics) Node {
	return &metricsNode{inner: node, metrics: metrics}
}

type metricsNode struct {
	inner   Node
	metrics *observability.Metrics
}

func (n *metricsNode) Name() string { return n.inner.Name() }

func (n *metricsNode) Run(ctx context.Context, state *State) (any, error) {
	start := time.Now()
	result, err := n.inner.Run(ctx, state)
	duration := time.Since(start)

	status := "ok"
	if err != nil {
		status = "error"
		n.metrics.RecordError(ctx, "execute", n.inner.Name())
	}
	n.metrics.RecordOperation(ctx, n.inner.Name(), "dag.run", status, duration)

	return result, err
}

// WithLogging wraps a Node with execution logging. Logs: node name, duration, and success/error status.
func WithLogging(node Node, log *logging.Logger) Node {
	return &loggingNode{inner: node, log: log}
}

type loggingNode struct {
	inner Node
	log   *logging.Logger
}

func (n *loggingNode) Name() string { return n.inner.Name() }

func (n *loggingNode) Run(ctx context.Context, state *State) (any, error) {
	start := time.Now()
	result, err := n.inner.Run(ctx, state)
	duration := time.Since(start)

	fields := map[string]any{
		"node":     n.inner.Name(),
		"duration": duration.String(),
	}

	if err != nil {
		fields["error"] = err.Error()
		n.log.ErrorCtx(ctx, "dag node failed", fields)
	} else {
		n.log.DebugCtx(ctx, "dag node completed", fields)
	}

	return result, err
}
