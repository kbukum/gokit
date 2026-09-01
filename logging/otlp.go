package logging

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"go.opentelemetry.io/otel/attribute"
	otellog "go.opentelemetry.io/otel/log"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"

	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"

	apperrors "github.com/kbukum/gokit/errors"
)

const otlpLoggerName = "github.com/kbukum/gokit/logging"

// OTLPProvider owns the OpenTelemetry LoggerProvider used by the OTLP sink and
// exposes its lifecycle so the facade can flush and shut it down.
type OTLPProvider struct {
	provider *sdklog.LoggerProvider
	logger   otellog.Logger
}

// OTLPProviderConfig configures an OTLP log provider and its resource attributes.
type OTLPProviderConfig struct {
	Exporter    OTLPConfig
	ServiceName string
	Environment string
	Version     string
}

// NewOTLPProvider creates and starts an OTLP log provider.
func NewOTLPProvider(cfg OTLPProviderConfig) (*OTLPProvider, error) {
	ctx := context.Background()

	exporter, err := newLogExporter(ctx, cfg.Exporter)
	if err != nil {
		return nil, fmt.Errorf("creating OTLP log exporter: %w", err)
	}
	res, err := newLogResource(cfg.ServiceName, cfg.Environment, cfg.Version)
	if err != nil {
		return nil, fmt.Errorf("creating OTLP log resource: %w", err)
	}

	provider := sdklog.NewLoggerProvider(
		sdklog.WithProcessor(sdklog.NewBatchProcessor(exporter)),
		sdklog.WithResource(res),
	)
	return &OTLPProvider{
		provider: provider,
		logger:   provider.Logger(otlpLoggerName),
	}, nil
}

// Shutdown flushes and stops the provider.
func (p *OTLPProvider) Shutdown(ctx context.Context) error {
	if p == nil || p.provider == nil {
		return nil
	}
	return p.provider.Shutdown(ctx)
}

// otlpHandler is an [slog.Handler] that emits records to the OTLP collector. It
// is one branch of the fanout, so OTLP export is a first-class sink rather than
// a side channel bolted onto the primary writer.
type otlpHandler struct {
	provider *OTLPProvider
	lv       slog.Leveler
	attrs    []slog.Attr
	groups   []string
}

// newOTLPHandler wraps an OTLP provider as a level-gated slog sink.
func newOTLPHandler(provider *OTLPProvider, lv slog.Leveler) slog.Handler {
	return &otlpHandler{provider: provider, lv: lv}
}

func (h *otlpHandler) Enabled(_ context.Context, level slog.Level) bool {
	if h.provider == nil || h.provider.logger == nil {
		return false
	}
	return level >= h.lv.Level()
}

func (h *otlpHandler) Handle(ctx context.Context, rec slog.Record) error {
	var out otellog.Record
	out.SetTimestamp(rec.Time)
	out.SetSeverity(mapSeverity(rec.Level))
	out.SetSeverityText(strings.ToUpper(LevelName(rec.Level)))
	out.SetBody(attribute.StringValue(rec.Message))

	// Bound attrs were already group-qualified when WithAttrs captured them;
	// record attrs are qualified here against the currently open groups.
	for _, a := range h.attrs {
		out.AddAttributes(toOTLPKeyValue(a.Key, a.Value.Resolve().Any()))
	}
	prefix := h.groupPrefix()
	rec.Attrs(func(a slog.Attr) bool {
		out.AddAttributes(toOTLPKeyValue(prefix+a.Key, a.Value.Resolve().Any()))
		return true
	})

	h.provider.logger.Emit(ctx, out)
	return nil
}

func (h *otlpHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	next := *h
	next.attrs = append(append([]slog.Attr(nil), h.attrs...), h.qualify(attrs)...)
	return &next
}

func (h *otlpHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	next := *h
	next.groups = append(append([]string(nil), h.groups...), name)
	return &next
}

// groupPrefix is the dotted prefix for the currently open groups, or "" when
// no group is open.
func (h *otlpHandler) groupPrefix() string {
	if len(h.groups) == 0 {
		return ""
	}
	return strings.Join(h.groups, ".") + "."
}

// qualify prefixes attribute keys with the currently open groups, so attributes
// bound under a group export with the same dotted keys as the other sinks.
func (h *otlpHandler) qualify(attrs []slog.Attr) []slog.Attr {
	prefix := h.groupPrefix()
	if prefix == "" {
		return attrs
	}
	out := make([]slog.Attr, len(attrs))
	for i, a := range attrs {
		out[i] = slog.Attr{Key: prefix + a.Key, Value: a.Value}
	}
	return out
}

// mapSeverity converts a level to an OTel severity.
func mapSeverity(level slog.Level) otellog.Severity {
	switch LevelName(level) {
	case "trace":
		return otellog.SeverityTrace
	case "debug":
		return otellog.SeverityDebug
	case "info":
		return otellog.SeverityInfo
	case "warn":
		return otellog.SeverityWarn
	case "error":
		return otellog.SeverityError
	case "fatal":
		return otellog.SeverityFatal
	default:
		return otellog.SeverityUndefined
	}
}

// toOTLPKeyValue converts a key/value pair to an OTel attribute KeyValue.
func toOTLPKeyValue(key string, value any) attribute.KeyValue {
	switch v := value.(type) {
	case string:
		return attribute.String(key, v)
	case bool:
		return attribute.Bool(key, v)
	case int:
		return attribute.Int(key, v)
	case int64:
		return attribute.Int64(key, v)
	case float64:
		return attribute.Float64(key, v)
	default:
		return attribute.String(key, fmt.Sprintf("%v", v))
	}
}

func newLogExporter(ctx context.Context, cfg OTLPConfig) (sdklog.Exporter, error) {
	if strings.EqualFold(cfg.Protocol, "http") {
		return newHTTPLogExporter(ctx, cfg)
	}
	return newGRPCLogExporter(ctx, cfg)
}

func newGRPCLogExporter(ctx context.Context, cfg OTLPConfig) (*otlploggrpc.Exporter, error) {
	endpoint, err := resolveOTLPEndpoint(cfg)
	if err != nil {
		return nil, err
	}
	opts := []otlploggrpc.Option{otlploggrpc.WithEndpointURL(endpoint)}
	if len(cfg.Headers) > 0 {
		opts = append(opts, otlploggrpc.WithHeaders(cfg.Headers))
	}
	return otlploggrpc.New(ctx, opts...)
}

func newHTTPLogExporter(ctx context.Context, cfg OTLPConfig) (*otlploghttp.Exporter, error) {
	endpoint, err := resolveOTLPEndpoint(cfg)
	if err != nil {
		return nil, err
	}
	opts := []otlploghttp.Option{otlploghttp.WithEndpointURL(endpoint)}
	if len(cfg.Headers) > 0 {
		opts = append(opts, otlploghttp.WithHeaders(cfg.Headers))
	}
	return otlploghttp.New(ctx, opts...)
}

// resolveOTLPEndpoint applies the insecure transport policy to the configured
// endpoint. This helper makes the endpoint URL scheme the single source of
// transport security so Insecure stays authoritative and cannot silently
// disagree with a per-exporter TLS toggle: a scheme-less endpoint is prefixed
// with https:// (secure by default) or http:// when Insecure is set, so
// plaintext is always an explicit opt-in. An explicit scheme is normalized
// case-insensitively; one that contradicts the flag is rejected rather than
// silently honored: https:// with Insecure, and http:// without Insecure (which
// would ship records and headers in plaintext despite the secure default). Any
// other explicit scheme is rejected.
func resolveOTLPEndpoint(cfg OTLPConfig) (string, error) {
	endpoint := strings.TrimSpace(cfg.Endpoint)
	if idx := strings.Index(endpoint, "://"); idx >= 0 {
		scheme := strings.ToLower(endpoint[:idx])
		rest := endpoint[idx:]
		switch scheme {
		case "https":
			if cfg.Insecure {
				return "", apperrors.InvalidInput("logging.otlp.insecure",
					fmt.Sprintf("insecure=true contradicts TLS endpoint %q", endpoint))
			}
			return scheme + rest, nil
		case "http":
			if !cfg.Insecure {
				return "", apperrors.InvalidInput("logging.otlp.insecure",
					fmt.Sprintf("plaintext endpoint %q requires insecure=true; use https:// for secure transport", endpoint))
			}
			return scheme + rest, nil
		default:
			return "", apperrors.InvalidInput("logging.otlp.endpoint",
				fmt.Sprintf("unsupported endpoint scheme %q; use http:// or https://", scheme))
		}
	}
	scheme := "https://"
	if cfg.Insecure {
		scheme = "http://"
	}
	return scheme + endpoint, nil
}

func newLogResource(serviceName, environment, version string) (*resource.Resource, error) {
	return resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(serviceName),
			semconv.ServiceVersion(version),
			semconv.DeploymentEnvironmentNameKey.String(environment),
		),
	)
}
