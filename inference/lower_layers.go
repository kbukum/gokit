package inference

import (
	"context"

	"github.com/kbukum/gokit/ai/semconv"
	"github.com/kbukum/gokit/httpclient"
	"github.com/kbukum/gokit/resilience"
)

// HTTPDoer is the lower-layer HTTP seam adapters consume. Implementations should normally be *httpclient.Adapter configured by constructors.
type HTTPDoer interface {
	Do(context.Context, httpclient.Request) (*httpclient.Response, error)
}

// RetryPolicy names the resilience retry policy accepted by network adapters.
type RetryPolicy = resilience.RetryConfig

// GenAIInferenceSpanName is the canonical OTel GenAI span name for inference requests.
const GenAIInferenceSpanName = semconv.OpInferenceRequest
