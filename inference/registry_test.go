package inference_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/kbukum/gokit/inference"
)

type fakeRuntime struct{}

func (fakeRuntime) Name() string { return "fake" }

func (fakeRuntime) IsAvailable(context.Context) bool { return true }

func (fakeRuntime) Execute(context.Context, inference.PredictRequest) (inference.PredictResponse, error) {
	return inference.PredictResponse{}, nil
}

func (fakeRuntime) Predict(context.Context, inference.PredictRequest) (inference.PredictResponse, error) {
	return inference.PredictResponse{}, nil
}

func (fakeRuntime) Descriptor() inference.Descriptor { return inference.Descriptor{Name: "fake"} }

func TestRegistryRegisterBuildKinds(t *testing.T) {
	t.Parallel()

	reg := inference.NewRegistry()
	if got := reg.Kinds(); len(got) != 0 {
		t.Fatalf("new registry kinds = %v, want empty", got)
	}

	if err := reg.Register("fake", func(json.RawMessage) (inference.Inference, error) { return fakeRuntime{}, nil }); err != nil {
		t.Fatalf("register: %v", err)
	}
	if got := reg.Kinds(); len(got) != 1 || got[0] != "fake" {
		t.Fatalf("kinds = %v, want [fake]", got)
	}
	if _, err := reg.Build("fake", nil); err != nil {
		t.Fatalf("build: %v", err)
	}
}

func TestRegistryRejectsDuplicateAndUnknown(t *testing.T) {
	t.Parallel()

	reg := inference.NewRegistry()
	factory := func(json.RawMessage) (inference.Inference, error) { return fakeRuntime{}, nil }
	if err := reg.Register("fake", factory); err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := reg.Register("fake", factory); err == nil {
		t.Fatal("expected duplicate registration error")
	}
	_, err := reg.Build("missing", nil)
	if err == nil || !strings.Contains(err.Error(), "unknown adapter") {
		t.Fatalf("unknown build error = %v", err)
	}
}
