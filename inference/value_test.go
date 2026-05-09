package inference_test

import (
	"encoding/json"
	"testing"

	"github.com/kbukum/gokit/inference"
)

func TestValueConstructorsAndJSONEncoding(t *testing.T) {
	t.Parallel()

	values := map[string]inference.Value{
		"text":   inference.TextValue("hello"),
		"bytes":  inference.BytesValue([]byte("abc")),
		"tensor": inference.TensorValue(inference.Tensor{DType: "FP32", Shape: []int64{1, 2}, Data: []float32{1.5, 2.5}}),
		"json":   inference.JSONValue(json.RawMessage(`{"nested":true}`)),
	}

	if values["text"].Kind != inference.KindText || values["text"].Text != "hello" {
		t.Fatalf("text value = %+v", values["text"])
	}
	if values["bytes"].Kind != inference.KindBytes || string(values["bytes"].Bytes) != "abc" {
		t.Fatalf("bytes value = %+v", values["bytes"])
	}
	if values["tensor"].Kind != inference.KindTensor || values["tensor"].Tensor.DType != "FP32" {
		t.Fatalf("tensor value = %+v", values["tensor"])
	}
	if values["json"].Kind != inference.KindJSON || string(values["json"].JSON) != `{"nested":true}` {
		t.Fatalf("json value = %+v", values["json"])
	}

	encoded, err := json.Marshal(values)
	if err != nil {
		t.Fatalf("marshal values: %v", err)
	}
	var decoded map[string]inference.Value
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("unmarshal values: %v", err)
	}
	if decoded["text"].Kind != inference.KindText || decoded["bytes"].Kind != inference.KindBytes || decoded["tensor"].Kind != inference.KindTensor || decoded["json"].Kind != inference.KindJSON {
		t.Fatalf("decoded values = %+v", decoded)
	}
}

func TestConstructorsCopyMutableInputs(t *testing.T) {
	t.Parallel()

	bytes := []byte("abc")
	bv := inference.BytesValue(bytes)
	bytes[0] = 'z'
	if string(bv.Bytes) != "abc" {
		t.Fatalf("BytesValue did not copy input: %q", bv.Bytes)
	}

	raw := json.RawMessage(`{"ok":true}`)
	jv := inference.JSONValue(raw)
	raw[1] = 'X'
	if string(jv.JSON) != `{"ok":true}` {
		t.Fatalf("JSONValue did not copy input: %s", jv.JSON)
	}
}
