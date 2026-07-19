package triton

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/kbukum/gokit/ai"
	"github.com/kbukum/gokit/httpclient"
	"github.com/kbukum/gokit/inference"
)

type kserveRequest struct {
	ID         string         `json:"id,omitempty"`
	Inputs     []kserveTensor `json:"inputs"`
	Parameters map[string]any `json:"parameters,omitempty"`
}

type kserveResponse struct {
	ModelName    string         `json:"model_name,omitempty"`
	ModelVersion string         `json:"model_version,omitempty"`
	Outputs      []kserveTensor `json:"outputs"`
	Parameters   map[string]any `json:"parameters,omitempty"`
}

type kserveTensor struct {
	Name     string          `json:"name"`
	Shape    []int64         `json:"shape"`
	Datatype string          `json:"datatype"`
	Data     json.RawMessage `json:"data,omitempty"`
}

func encodeRequest(req inference.PredictRequest) (kserveRequest, error) {
	inputs := make([]kserveTensor, 0, len(req.Inputs))
	var joined error
	for name, value := range req.Inputs {
		input, err := encodeInput(name, value)
		if err != nil {
			joined = errors.Join(joined, err)
			continue
		}
		inputs = append(inputs, input)
	}
	if joined != nil {
		return kserveRequest{}, joined
	}
	parameters := make(map[string]any, len(req.Parameters)+len(req.Options))
	for key, value := range req.Parameters {
		parameters[key] = value
	}
	for key, value := range req.Options {
		parameters[key] = value
	}
	if len(parameters) == 0 {
		parameters = nil
	}
	return kserveRequest{ID: req.RequestID, Inputs: inputs, Parameters: parameters}, nil
}

func encodeInput(name string, value inference.Value) (kserveTensor, error) {
	switch value.Kind {
	case inference.KindText:
		return tensor(tensorSpec{name: name, dtype: "BYTES", shape: []int64{1}, data: []string{value.Text}})
	case inference.KindBytes:
		return tensor(tensorSpec{name: name, dtype: "BYTES", shape: []int64{1}, data: []string{base64.StdEncoding.EncodeToString(value.Bytes)}})
	case inference.KindTensor:
		if value.Tensor == nil {
			return kserveTensor{}, fmt.Errorf("triton: input %q tensor is nil", name)
		}
		data, err := normalizeTensorData(value.Tensor.DType, value.Tensor.Data)
		if err != nil {
			return kserveTensor{}, fmt.Errorf("triton: input %q: %w", name, err)
		}
		return tensor(tensorSpec{name: name, dtype: value.Tensor.DType, shape: value.Tensor.Shape, data: data})
	case inference.KindJSON:
		return kserveTensor{Name: name, Shape: []int64{1}, Datatype: "JSON", Data: append(json.RawMessage(nil), value.JSON...)}, nil
	default:
		return kserveTensor{}, fmt.Errorf("triton: input %q has unsupported kind %q", name, value.Kind)
	}
}

type tensorSpec struct {
	name  string
	dtype string
	shape []int64
	data  any
}

func tensor(spec tensorSpec) (kserveTensor, error) {
	encoded, err := json.Marshal(spec.data)
	if err != nil {
		return kserveTensor{}, fmt.Errorf("marshal tensor %q data: %w", spec.name, err)
	}
	return kserveTensor{Name: spec.name, Shape: append([]int64(nil), spec.shape...), Datatype: spec.dtype, Data: encoded}, nil
}

func normalizeTensorData(dtype string, data any) (any, error) {
	switch strings.ToUpper(dtype) {
	case "FP32":
		switch v := data.(type) {
		case []float32, []float64:
			return v, nil
		default:
			return nil, fmt.Errorf("FP32 data must be []float32 or []float64")
		}
	case "INT64":
		switch v := data.(type) {
		case []int64:
			return v, nil
		case []int:
			out := make([]int64, len(v))
			for i, n := range v {
				out[i] = int64(n)
			}
			return out, nil
		default:
			return nil, fmt.Errorf("INT64 data must be []int64 or []int")
		}
	case "BYTES":
		switch v := data.(type) {
		case []string, [][]byte:
			return v, nil
		case []byte:
			return []string{base64.StdEncoding.EncodeToString(v)}, nil
		default:
			return nil, fmt.Errorf("BYTES data must be []string, [][]byte, or []byte")
		}
	default:
		return data, nil
	}
}

func decodeResponse(resp *httpclient.Response) (inference.PredictResponse, error) {
	var payload kserveResponse
	if err := json.Unmarshal(resp.Body, &payload); err != nil {
		return inference.PredictResponse{}, fmt.Errorf("triton: decode KServe v2 response: %w", err)
	}
	outputs := make(map[string]inference.Value, len(payload.Outputs))
	var joined error
	for _, output := range payload.Outputs {
		value, err := decodeOutput(output)
		if err != nil {
			joined = errors.Join(joined, err)
			continue
		}
		outputs[output.Name] = value
	}
	if joined != nil {
		return inference.PredictResponse{}, joined
	}
	modelName := firstNonEmpty(resp.Headers["model_name"], resp.Headers["model-name"], payload.ModelName)
	modelVersion := firstNonEmpty(resp.Headers["model_version"], resp.Headers["model-version"], payload.ModelVersion)
	metadata := map[string]string{}
	if modelName != "" {
		metadata["model_name"] = modelName
	}
	if modelVersion != "" {
		metadata["model_version"] = modelVersion
	}
	return inference.PredictResponse{
		Outputs:  outputs,
		Model:    ai.Model{Name: modelName, Version: modelVersion, Provider: ai.ProviderTriton},
		Status:   inference.StatusSuccess,
		Metadata: metadata,
	}, nil
}

func decodeOutput(output kserveTensor) (inference.Value, error) {
	switch strings.ToUpper(output.Datatype) {
	case "FP32":
		var data []float32
		if err := json.Unmarshal(output.Data, &data); err != nil {
			return inference.Value{}, fmt.Errorf("triton: output %q FP32 decode: %w", output.Name, err)
		}
		return inference.TensorValue(inference.Tensor{DType: "FP32", Shape: output.Shape, Data: data}), nil
	case "INT64":
		var data []int64
		if err := json.Unmarshal(output.Data, &data); err != nil {
			return inference.Value{}, fmt.Errorf("triton: output %q INT64 decode: %w", output.Name, err)
		}
		return inference.TensorValue(inference.Tensor{DType: "INT64", Shape: output.Shape, Data: data}), nil
	case "BYTES":
		var data []string
		if err := json.Unmarshal(output.Data, &data); err != nil {
			return inference.Value{}, fmt.Errorf("triton: output %q BYTES decode: %w", output.Name, err)
		}
		return inference.TensorValue(inference.Tensor{DType: "BYTES", Shape: output.Shape, Data: data}), nil
	case "JSON":
		return inference.JSONValue(output.Data), nil
	default:
		var data any
		if len(output.Data) > 0 {
			if err := json.Unmarshal(output.Data, &data); err != nil {
				return inference.Value{}, fmt.Errorf("triton: output %q decode: %w", output.Name, err)
			}
		}
		return inference.TensorValue(inference.Tensor{DType: output.Datatype, Shape: output.Shape, Data: data}), nil
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
