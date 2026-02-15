// Package observability provides OpenTelemetry tracing and metrics integration
// for comprehensive service observability.
//
// Tracing:
//
//	tp, err := observability.InitTracer(ctx, observability.DefaultTracerConfig("my-service"))
//	defer tp.Shutdown(ctx)
//
//	ctx, span := observability.StartSpan(ctx, "my.operation")
//	defer span.End()
//
// Metrics:
//
//	mp, err := observability.InitMeter(ctx, observability.DefaultMeterConfig("my-service"))
//	defer mp.Shutdown(ctx)
//
//	metrics, err := observability.NewMetrics(observability.Meter("my-service"))
//	metrics.RecordRequestEnd(ctx, "my-service", "GET /users", "ok", duration)
//
// Health Checks:
//
//	health := observability.NewServiceHealth("my-service", "1.0.0")
//	health.AddComponent(checker.CheckHealth(ctx))
package observability
