package triton_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/kbukum/gokit/ai/semconv"
	"github.com/kbukum/gokit/component"
	"github.com/kbukum/gokit/inference"
	"github.com/kbukum/gokit/inference/triton"
)

func TestProviderPredictHappyPath(t *testing.T) {
	t.Parallel()

	var sawPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawPath = r.URL.Path
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s", r.Method)
		}
		var req struct {
			Inputs []struct {
				Name     string          `json:"name"`
				Datatype string          `json:"datatype"`
				Shape    []int64         `json:"shape"`
				Data     json.RawMessage `json:"data"`
			} `json:"inputs"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if len(req.Inputs) != 3 {
			t.Fatalf("inputs = %d, want 3", len(req.Inputs))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model_name":"demo","outputs":[{"name":"scores","datatype":"FP32","shape":[1,2],"data":[0.25,0.75]},{"name":"label","datatype":"BYTES","shape":[1],"data":["cat"]}]}`))
	}))
	defer server.Close()

	provider, err := triton.NewProvider(triton.Config{BaseURL: server.URL})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}

	resp, err := provider.Predict(context.Background(), inference.PredictRequest{
		ModelName: "demo",
		Inputs: map[string]inference.Value{
			"features": inference.TensorValue(inference.Tensor{DType: "FP32", Shape: []int64{1, 2}, Data: []float32{1, 2}}),
			"ids":      inference.TensorValue(inference.Tensor{DType: "INT64", Shape: []int64{1, 2}, Data: []int64{7, 8}}),
			"blob":     inference.BytesValue([]byte("abc")),
		},
	})
	if err != nil {
		t.Fatalf("Predict: %v", err)
	}
	if sawPath != "/v2/models/demo/infer" {
		t.Fatalf("path = %q", sawPath)
	}
	if resp.Metadata["model_name"] != "demo" {
		t.Fatalf("metadata = %+v", resp.Metadata)
	}
	scores := resp.Outputs["scores"].Tensor.Data.([]float32)
	if scores[0] != 0.25 || scores[1] != 0.75 {
		t.Fatalf("scores = %#v", scores)
	}
	label := resp.Outputs["label"].Tensor.Data.([]string)
	if label[0] != "cat" {
		t.Fatalf("label = %#v", label)
	}
}

func TestProviderPredictErrorResponse(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "model not found", http.StatusNotFound)
	}))
	defer server.Close()

	provider, err := triton.NewProvider(triton.Config{BaseURL: server.URL})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	_, err = provider.Predict(context.Background(), inference.PredictRequest{
		ModelName: "missing",
		Inputs:    map[string]inference.Value{"input": inference.TextValue("x")},
	})
	if err == nil || !strings.Contains(err.Error(), "model not found") {
		t.Fatalf("Predict error = %v", err)
	}
}

func TestProviderHealth(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/health/ready" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	provider, err := triton.NewProvider(triton.Config{BaseURL: server.URL})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	if err := provider.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	health := provider.Health(context.Background())
	if health.Status != component.StatusHealthy {
		t.Fatalf("Health = %+v", health)
	}
	desc := provider.Descriptor()
	if desc.ServingProtocol != "kserve-v2-http" || !desc.Available {
		t.Fatalf("descriptor = %+v", desc)
	}
}

func TestProviderLifecycleAvailabilityAndExecute(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v2/models/demo/infer":
			_, _ = w.Write([]byte(`{"model_name":"demo","outputs":[{"name":"label","datatype":"BYTES","shape":[1],"data":["cat"]}]}`))
		default:
			t.Fatalf("path = %q", r.URL.Path)
		}
	}))
	defer server.Close()

	provider, err := triton.NewProvider(triton.Config{BaseURL: server.URL})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	if provider.Name() != triton.Kind || !provider.IsAvailable(context.Background()) {
		t.Fatalf("provider state: name=%q available=%v", provider.Name(), provider.IsAvailable(context.Background()))
	}
	if health := provider.Health(context.Background()); health.Status != component.StatusDegraded {
		t.Fatalf("initial Health = %+v", health)
	}
	resp, err := provider.Execute(context.Background(), inference.PredictRequest{
		ModelName: "demo",
		Inputs:    map[string]inference.Value{"prompt": inference.TextValue("x")},
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if resp.Outputs["label"].Tensor.Data.([]string)[0] != "cat" {
		t.Fatalf("Execute response = %+v", resp)
	}
	if err := provider.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
}

func TestProviderPredictSpanAttributes(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	otel.SetTracerProvider(tp)
	t.Cleanup(func() {
		_ = tp.Shutdown(context.Background())
		otel.SetTracerProvider(noop.NewTracerProvider())
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model_name":"demo","outputs":[{"name":"label","datatype":"BYTES","shape":[1],"data":["cat"]}]}`))
	}))
	defer server.Close()

	provider, err := triton.NewProvider(triton.Config{BaseURL: server.URL})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}

	_, err = provider.Predict(context.Background(), inference.PredictRequest{
		ModelName: "demo",
		Inputs:    map[string]inference.Value{"input": inference.TextValue("x")},
	})
	if err != nil {
		t.Fatalf("Predict: %v", err)
	}

	spans := exporter.GetSpans()
	if len(spans) == 0 {
		t.Fatal("expected exported span")
	}
	attrs := map[string]string{}
	for _, attr := range spans[len(spans)-1].Attributes {
		if attr.Value.Type() == attribute.STRING {
			attrs[string(attr.Key)] = attr.Value.AsString()
		}
	}
	if attrs[semconv.GenAIRequestID] == "" {
		t.Fatalf("missing %s attr: %#v", semconv.GenAIRequestID, attrs)
	}
	if got := attrs[semconv.GenAIResponseModel]; got != "demo" {
		t.Fatalf("%s = %q, want demo (attrs %#v)", semconv.GenAIResponseModel, got, attrs)
	}
}

func TestProviderPredictEncodeError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("server should not be called when request encoding fails")
	}))
	defer server.Close()

	provider, err := triton.NewProvider(triton.Config{BaseURL: server.URL})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	_, err = provider.Predict(context.Background(), inference.PredictRequest{
		ModelName: "demo",
		Inputs: map[string]inference.Value{
			"bad": inference.TensorValue(inference.Tensor{DType: "FP32", Shape: []int64{1}, Data: []int{1}}),
		},
	})
	if err == nil || !strings.Contains(err.Error(), "FP32 data") {
		t.Fatalf("Predict error = %v", err)
	}
}
