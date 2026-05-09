// Package echo provides the lean default inference adapter for tests.
package echo

import (
	"context"
	"encoding/json"

	"github.com/kbukum/gokit/ai"
	"github.com/kbukum/gokit/inference"
)

const Kind = "echo"

// Echo returns inputs unchanged as outputs.
type Echo struct{}

// Factory builds an Echo adapter.
func Factory(json.RawMessage) (inference.Inference, error) { return &Echo{}, nil }

// Register adds the echo adapter to reg explicitly.
func Register(reg *inference.Registry) error { return reg.Register(Kind, Factory) }

// Predict returns request inputs unchanged.
func (e *Echo) Predict(_ context.Context, req inference.PredictRequest) (inference.PredictResponse, error) {
	outputs := make(map[string]inference.Value, len(req.Inputs))
	for name, value := range req.Inputs {
		outputs[name] = cloneValue(value)
	}
	return inference.PredictResponse{Outputs: outputs, Model: model(req), Status: inference.StatusSuccess, Usage: ai.Usage{}}, nil
}

// Descriptor documents the in-memory echo adapter.
func (e *Echo) Descriptor() inference.Descriptor {
	return inference.Descriptor{
		Name:            Kind,
		Description:     "in-memory echo inference adapter",
		ServingProtocol: "in-memory",
		Available:       true,
	}
}

func cloneValue(value inference.Value) inference.Value {
	switch value.Kind {
	case inference.KindBytes:
		return inference.BytesValue(value.Bytes)
	case inference.KindJSON:
		return inference.JSONValue(value.JSON)
	case inference.KindTensor:
		if value.Tensor != nil {
			shape := make([]int64, len(value.Tensor.Shape))
			copy(shape, value.Tensor.Shape)
			return inference.TensorValue(inference.Tensor{
				DType: value.Tensor.DType,
				Shape: shape,
				Data:  value.Tensor.Data,
			})
		}
		return value
	default:
		return value
	}
}

func model(req inference.PredictRequest) ai.Model {
	m := ai.Model{Name: req.ModelName, Version: req.ModelVersion, Provider: ai.ProviderCustom}
	if m.Name == "" {
		m.Name = Kind
	}
	return m
}

var _ inference.Inference = (*Echo)(nil)
