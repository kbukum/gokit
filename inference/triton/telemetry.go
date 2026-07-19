package triton

import (
	"context"

	"github.com/kbukum/gokit/ai/semconv"
	"github.com/kbukum/gokit/inference"
	"github.com/kbukum/gokit/observability"
)

func startSpan(ctx context.Context, operationName string, attrs ...observability.SpanAttribute) (context.Context, *observability.Span) {
	baseAttrs := make([]observability.SpanAttribute, 0, 2+len(attrs))
	baseAttrs = append(baseAttrs,
		observability.StringAttribute(semconv.GenAISystem, Kind),
		observability.StringAttribute(semconv.GenAIOperationName, operationName),
	)
	baseAttrs = append(baseAttrs, attrs...)
	return observability.StartNamedSpan(ctx, "github.com/kbukum/gokit/inference/triton", semconv.OpInferenceRequest,
		observability.WithSpanKind(observability.SpanKindClient),
		observability.WithSpanAttributes(baseAttrs...),
	)
}

func operation(inference.PredictRequest) string {
	return semconv.OpInferenceRequest
}

func modelAttributes(req inference.PredictRequest) []observability.SpanAttribute {
	attrs := []observability.SpanAttribute{
		observability.StringAttribute(semconv.GenAIRequestModel, req.ModelName),
	}
	if req.ModelVersion != "" {
		attrs = append(attrs, observability.StringAttribute(semconv.GenAIRequestModelVersion, req.ModelVersion))
	}
	return attrs
}

func usageAttributes(usage inference.Usage) []observability.SpanAttribute {
	return []observability.SpanAttribute{
		observability.IntAttribute(semconv.GenAIUsageInputTokens, usage.InputTokens),
		observability.IntAttribute(semconv.GenAIUsageOutputTokens, usage.OutputTokens),
		observability.IntAttribute(semconv.GenAIUsageCachedTokens, usage.CachedTokens),
		observability.IntAttribute(semconv.GenAIUsageReasoningTokens, usage.ReasoningTokens),
	}
}
