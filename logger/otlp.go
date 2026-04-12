package logger

import (
	"context"
	"fmt"
	"strings"
	"time"

	otellog "go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/sdk/resource"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	semconv "go.opentelemetry.io/otel/semconv/v1.40.0"

	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
)

const otlpLoggerName = "github.com/kbukum/gokit/logger"

// OTLPProvider manages the OpenTelemetry LoggerProvider for OTLP export.
type OTLPProvider struct {
	provider *sdklog.LoggerProvider
	logger   otellog.Logger
}

// NewOTLPProvider creates and starts an OTLP log provider.
func NewOTLPProvider(cfg OTLPConfig, serviceName, environment, version string) (*OTLPProvider, error) {
	ctx := context.Background()

	exporter, err := newLogExporter(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("creating OTLP log exporter: %w", err)
	}

	res, err := newLogResource(serviceName, environment, version)
	if err != nil {
		return nil, fmt.Errorf("creating OTLP log resource: %w", err)
	}

	processor := sdklog.NewBatchProcessor(exporter)
	provider := sdklog.NewLoggerProvider(
		sdklog.WithProcessor(processor),
		sdklog.WithResource(res),
	)

	return &OTLPProvider{
		provider: provider,
		logger:   provider.Logger(otlpLoggerName),
	}, nil
}

// Shutdown gracefully shuts down the OTLP provider, flushing pending logs.
func (p *OTLPProvider) Shutdown(ctx context.Context) error {
	if p == nil || p.provider == nil {
		return nil
	}
	return p.provider.Shutdown(ctx)
}

// EmitLog sends a log record to the OTLP collector.
func (p *OTLPProvider) EmitLog(level, message string, fields map[string]interface{}) {
	if p == nil || p.logger == nil {
		return
	}

	var rec otellog.Record
	rec.SetTimestamp(time.Now())
	rec.SetSeverity(mapSeverity(level))
	rec.SetSeverityText(strings.ToUpper(level))
	rec.SetBody(otellog.StringValue(message))

	if len(fields) > 0 {
		attrs := make([]otellog.KeyValue, 0, len(fields))
		for k, v := range fields {
			attrs = append(attrs, toOTLPKeyValue(k, v))
		}
		rec.AddAttributes(attrs...)
	}

	p.logger.Emit(context.Background(), rec)
}

// mapSeverity converts a log level string to an OTel Severity.
func mapSeverity(level string) otellog.Severity {
	switch strings.ToLower(level) {
	case "trace":
		return otellog.SeverityTrace
	case "debug":
		return otellog.SeverityDebug
	case "info":
		return otellog.SeverityInfo
	case "warn", "warning":
		return otellog.SeverityWarn
	case "error":
		return otellog.SeverityError
	case "fatal":
		return otellog.SeverityFatal
	default:
		return otellog.SeverityUndefined
	}
}

// toOTLPKeyValue converts an arbitrary key-value pair to an OTel log KeyValue.
func toOTLPKeyValue(key string, value interface{}) otellog.KeyValue {
	switch v := value.(type) {
	case string:
		return otellog.String(key, v)
	case bool:
		return otellog.Bool(key, v)
	case int:
		return otellog.Int(key, v)
	case int64:
		return otellog.Int64(key, v)
	case float64:
		return otellog.Float64(key, v)
	case []byte:
		return otellog.Bytes(key, v)
	default:
		return otellog.String(key, fmt.Sprintf("%v", v))
	}
}

// newLogExporter creates a gRPC or HTTP OTLP log exporter based on config.
func newLogExporter(ctx context.Context, cfg OTLPConfig) (sdklog.Exporter, error) {
	switch strings.ToLower(cfg.Protocol) {
	case "http":
		return newHTTPLogExporter(ctx, cfg)
	default:
		return newGRPCLogExporter(ctx, cfg)
	}
}

func newGRPCLogExporter(ctx context.Context, cfg OTLPConfig) (*otlploggrpc.Exporter, error) {
	opts := []otlploggrpc.Option{
		otlploggrpc.WithEndpoint(cfg.Endpoint),
	}
	if cfg.Insecure {
		opts = append(opts, otlploggrpc.WithInsecure())
	}
	if len(cfg.Headers) > 0 {
		opts = append(opts, otlploggrpc.WithHeaders(cfg.Headers))
	}
	return otlploggrpc.New(ctx, opts...)
}

func newHTTPLogExporter(ctx context.Context, cfg OTLPConfig) (*otlploghttp.Exporter, error) {
	opts := []otlploghttp.Option{
		otlploghttp.WithEndpoint(cfg.Endpoint),
	}
	if cfg.Insecure {
		opts = append(opts, otlploghttp.WithInsecure())
	}
	if len(cfg.Headers) > 0 {
		opts = append(opts, otlploghttp.WithHeaders(cfg.Headers))
	}
	return otlploghttp.New(ctx, opts...)
}

func newLogResource(serviceName, environment, version string) (*resource.Resource, error) {
	return resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(serviceName),
			semconv.ServiceVersion(version),
			semconv.DeploymentEnvironmentName(environment),
		),
	)
}
