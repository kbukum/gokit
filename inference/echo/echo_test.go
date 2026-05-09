package echo_test

import (
	"context"
	"encoding/json"
	"testing"

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
