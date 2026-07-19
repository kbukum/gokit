package echo_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/kbukum/gokit/component"
	"github.com/kbukum/gokit/inference"
	"github.com/kbukum/gokit/inference/echo"
)

func TestEchoPredictAndRegister(t *testing.T) {
	reg := inference.NewRegistry()
	if err := echo.Register(reg); err != nil {
		t.Fatal(err)
	}
	runtime, err := reg.Build(echo.Kind, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := runtime.Predict(context.Background(), inference.PredictRequest{ModelName: "echo-model", Inputs: map[string]inference.Value{"text": inference.TextValue("hello"), "json": inference.JSONValue(json.RawMessage(`{"ok":true}`))}})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Status != inference.StatusSuccess || resp.Model.Name != "echo-model" {
		t.Fatalf("resp=%+v", resp)
	}
	if resp.Outputs["text"].Text != "hello" || string(resp.Outputs["json"].JSON) != `{"ok":true}` {
		t.Fatalf("outputs=%+v", resp.Outputs)
	}
	if runtime.Descriptor().Name != echo.Kind {
		t.Fatalf("descriptor=%+v", runtime.Descriptor())
	}
}

func TestEchoLifecycleHealthAndExecute(t *testing.T) {
	t.Parallel()

	runtime := &echo.Echo{}
	if runtime.Name() != echo.Kind {
		t.Fatalf("Name = %q", runtime.Name())
	}
	if !runtime.IsAvailable(context.Background()) {
		t.Fatal("echo should always be available")
	}
	if health := runtime.Health(context.Background()); health.Status != component.StatusDegraded || !strings.Contains(health.Message, "not started") {
		t.Fatalf("initial Health = %+v", health)
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if health := runtime.Health(context.Background()); health.Status != component.StatusHealthy || health.Message != "ready" {
		t.Fatalf("started Health = %+v", health)
	}

	resp, err := runtime.Execute(context.Background(), inference.PredictRequest{
		Inputs: map[string]inference.Value{"text": inference.TextValue("hello")},
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if resp.Model.Name != echo.Kind || resp.Outputs["text"].Text != "hello" {
		t.Fatalf("Execute response = %+v", resp)
	}
	if health := runtime.Health(context.Background()); health.Status != component.StatusHealthy || !strings.Contains(health.Message, "last_call=") {
		t.Fatalf("post-call Health = %+v", health)
	}
	if err := runtime.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if health := runtime.Health(context.Background()); health.Status != component.StatusDegraded {
		t.Fatalf("stopped Health = %+v", health)
	}
}

func TestEchoPredictClonesMutableValues(t *testing.T) {
	t.Parallel()

	runtime := &echo.Echo{}
	rawBytes := []byte("abc")
	rawJSON := json.RawMessage(`{"ok":true}`)
	shape := []int64{1, 2}
	resp, err := runtime.Predict(context.Background(), inference.PredictRequest{
		ModelName:    "model",
		ModelVersion: "v1",
		Inputs: map[string]inference.Value{
			"bytes": inference.BytesValue(rawBytes),
			"json":  inference.JSONValue(rawJSON),
			"tensor": inference.TensorValue(inference.Tensor{
				DType: "FP32",
				Shape: shape,
				Data:  []float32{1, 2},
			}),
			"text": inference.TextValue("same"),
		},
	})
	if err != nil {
		t.Fatalf("Predict: %v", err)
	}
	rawBytes[0] = 'z'
	rawJSON[1] = 'X'
	shape[0] = 9
	if string(resp.Outputs["bytes"].Bytes) != "abc" {
		t.Fatalf("bytes output mutated: %q", resp.Outputs["bytes"].Bytes)
	}
	if string(resp.Outputs["json"].JSON) != `{"ok":true}` {
		t.Fatalf("json output mutated: %s", resp.Outputs["json"].JSON)
	}
	if resp.Outputs["tensor"].Tensor.Shape[0] != 1 {
		t.Fatalf("tensor shape mutated: %+v", resp.Outputs["tensor"].Tensor.Shape)
	}
	if resp.Model.Name != "model" || resp.Model.Version != "v1" {
		t.Fatalf("model = %+v", resp.Model)
	}
}

func TestEchoPredictNilTensorPassesThrough(t *testing.T) {
	t.Parallel()

	runtime := &echo.Echo{}
	resp, err := runtime.Predict(context.Background(), inference.PredictRequest{
		Inputs: map[string]inference.Value{"nil": {Kind: inference.KindTensor}},
	})
	if err != nil {
		t.Fatalf("Predict: %v", err)
	}
	if resp.Outputs["nil"].Kind != inference.KindTensor || resp.Outputs["nil"].Tensor != nil {
		t.Fatalf("nil tensor output = %+v", resp.Outputs["nil"])
	}
}
