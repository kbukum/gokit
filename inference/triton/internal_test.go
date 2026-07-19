package triton

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kbukum/gokit/ai/semconv"
	"github.com/kbukum/gokit/authz"
	"github.com/kbukum/gokit/component"
	"github.com/kbukum/gokit/httpclient"
	"github.com/kbukum/gokit/inference"
	"github.com/kbukum/gokit/resilience"
)

func TestFactoryRegisterAndOptions(t *testing.T) {
	t.Parallel()

	reg := inference.NewRegistry()
	if err := Register(reg); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if _, err := reg.Build(Kind, json.RawMessage(`{"base_url":"http://127.0.0.1:1","timeout_seconds":1}`)); err != nil {
		t.Fatalf("Build: %v", err)
	}
	if _, err := Factory(json.RawMessage(`{`)); err == nil {
		t.Fatal("expected bad config error")
	}

	client, err := httpclient.New(httpclient.Config{Name: "test", Timeout: 1})
	if err != nil {
		t.Fatalf("httpclient.New: %v", err)
	}
	retry := resilience.DefaultRetryConfig()
	_, err = NewProvider(
		Config{BaseURL: "http://127.0.0.1:1"},
		WithHTTPClient(client),
		WithRetry(retry),
		WithDecider(authz.DeciderFunc(func(context.Context, authz.Request) (authz.Decision, error) {
			return authz.Decision{Allowed: true}, nil
		}), authz.Subject{ID: "subject"}),
	)
	if err != nil {
		t.Fatalf("NewProvider with options: %v", err)
	}
}

func TestNewProviderValidation(t *testing.T) {
	t.Parallel()

	if _, err := NewProvider(Config{}); err == nil {
		t.Fatal("expected missing base_url error")
	}
	if _, err := NewProvider(Config{BaseURL: "::not-a-url"}); err == nil {
		t.Fatal("expected invalid base_url error")
	}
}

func TestPredictVersionOperationAndAuthz(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/models/demo/versions/7/infer" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"outputs":[{"name":"ids","datatype":"INT64","shape":[2],"data":[4,5]},{"name":"meta","datatype":"JSON","shape":[1],"data":{"ok":true}}]}`))
	}))
	defer server.Close()

	called := false
	provider, err := NewProvider(Config{BaseURL: server.URL}, WithDecider(authz.DeciderFunc(func(_ context.Context, req authz.Request) (authz.Decision, error) {
		called = true
		if req.Resource.ID != "demo" || req.Resource.Attributes["version"] != "7" {
			t.Fatalf("authz request = %+v", req)
		}
		return authz.Decision{Allowed: true}, nil
	}), authz.Subject{ID: "alice"}))
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}

	resp, err := provider.Predict(context.Background(), inference.PredictRequest{
		ModelName:    "demo",
		ModelVersion: "7",
		Inputs: map[string]inference.Value{
			"prompt": inference.TextValue("hello"),
			"json":   inference.JSONValue(json.RawMessage(`{"pass":true}`)),
		},
		Metadata: map[string]string{semconv.GenAIOperationName: "classification"},
	})
	if err != nil {
		t.Fatalf("Predict: %v", err)
	}
	if !called {
		t.Fatal("decider was not called")
	}
	ids := resp.Outputs["ids"].Tensor.Data.([]int64)
	if ids[0] != 4 || ids[1] != 5 {
		t.Fatalf("ids = %#v", ids)
	}
	if string(resp.Outputs["meta"].JSON) != `{"ok":true}` {
		t.Fatalf("meta = %s", resp.Outputs["meta"].JSON)
	}
}

func TestAuthorizeDenialAndError(t *testing.T) {
	t.Parallel()

	deny, err := NewProvider(Config{BaseURL: "http://127.0.0.1:1"}, WithDecider(authz.DeciderFunc(func(context.Context, authz.Request) (authz.Decision, error) {
		return authz.Decision{Allowed: false, Reason: "no scope"}, nil
	}), authz.Subject{}))
	if err != nil {
		t.Fatalf("NewProvider deny: %v", err)
	}
	_, err = deny.Predict(context.Background(), inference.PredictRequest{ModelName: "m", Inputs: map[string]inference.Value{"x": inference.TextValue("x")}})
	if err == nil || !strings.Contains(err.Error(), "no scope") {
		t.Fatalf("deny err = %v", err)
	}

	boom := errors.New("boom")
	fail, err := NewProvider(Config{BaseURL: "http://127.0.0.1:1"}, WithDecider(authz.DeciderFunc(func(context.Context, authz.Request) (authz.Decision, error) {
		return authz.Decision{}, boom
	}), authz.Subject{}))
	if err != nil {
		t.Fatalf("NewProvider fail: %v", err)
	}
	_, err = fail.Predict(context.Background(), inference.PredictRequest{ModelName: "m", Inputs: map[string]inference.Value{"x": inference.TextValue("x")}})
	if !errors.Is(err, boom) {
		t.Fatalf("authz err = %v", err)
	}
}

func TestEncodingAndDecodingErrors(t *testing.T) {
	t.Parallel()

	_, err := encodeRequest(inference.PredictRequest{Inputs: map[string]inference.Value{
		"bad_tensor": inference.TensorValue(inference.Tensor{DType: "FP32", Data: []int{1}}),
		"nil_tensor": {Kind: inference.KindTensor},
		"unknown":    {Kind: inference.ValueKind("wat")},
	}})
	if err == nil || !strings.Contains(err.Error(), "bad_tensor") || !strings.Contains(err.Error(), "nil_tensor") || !strings.Contains(err.Error(), "unknown") {
		t.Fatalf("encode error = %v", err)
	}

	cases := []inference.Tensor{
		{DType: "FP32", Shape: []int64{1}, Data: []float64{1.25}},
		{DType: "INT64", Shape: []int64{2}, Data: []int{1, 2}},
		{DType: "BYTES", Shape: []int64{1}, Data: []byte("abc")},
		{DType: "FP16", Shape: []int64{1}, Data: []float32{1}},
	}
	for _, tc := range cases {
		if _, encodeErr := encodeInput("x", inference.TensorValue(tc)); encodeErr != nil {
			t.Fatalf("encodeInput(%s): %v", tc.DType, encodeErr)
		}
	}

	if _, decodeErr := decodeResponse(&httpclient.Response{Body: []byte(`not-json`)}); decodeErr == nil {
		t.Fatal("expected decode response error")
	}
	_, err = decodeResponse(&httpclient.Response{Body: []byte(`{"outputs":[{"name":"bad","datatype":"FP32","shape":[1],"data":["x"]},{"name":"bad2","datatype":"INT64","shape":[1],"data":["x"]},{"name":"bad3","datatype":"BYTES","shape":[1],"data":[1]}]}`)})
	if err == nil || !strings.Contains(err.Error(), "bad") || !strings.Contains(err.Error(), "bad2") || !strings.Contains(err.Error(), "bad3") {
		t.Fatalf("decode joined error = %v", err)
	}
	_, err = decodeOutput(kserveTensor{Name: "bad", Datatype: "OTHER", Shape: []int64{1}, Data: json.RawMessage(`{`)})
	if err == nil {
		t.Fatal("expected default output decode error")
	}
}

func TestPredictValidationAndHealthError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "not ready", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	provider, err := NewProvider(Config{BaseURL: server.URL})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	if _, err := provider.Predict(context.Background(), inference.PredictRequest{}); err == nil {
		t.Fatal("expected missing model error")
	}
	if err := provider.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	health := provider.Health(context.Background())
	if health.Status != component.StatusUnhealthy || !strings.Contains(health.Message, "not ready") {
		t.Fatalf("Health = %+v", health)
	}
}

func FuzzDecodeResponse(f *testing.F) {
	f.Add(`{"model_name":"demo","outputs":[{"name":"scores","datatype":"FP32","shape":[1],"data":[0.5]}]}`)
	f.Add(`{"outputs":[{"name":"bad","datatype":"FP32","shape":[1],"data":["x"]}]}`)
	f.Add(`{bad}`)
	f.Fuzz(func(t *testing.T, body string) {
		_, _ = decodeResponse(&httpclient.Response{Body: []byte(body)})
	})
}
