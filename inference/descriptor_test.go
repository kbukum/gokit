package inference_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/kbukum/gokit/ai"
	"github.com/kbukum/gokit/inference"
)

func TestDescriptorJSONEmitsTypedServingProtocolAndAvailable(t *testing.T) {
	t.Parallel()
	desc := inference.Descriptor{
		Name:            "triton",
		Description:     "kserve",
		ServingProtocol: inference.ServingKServeV2HTTP,
		Available:       true,
	}
	data, err := json.Marshal(desc)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got := string(data)
	if !strings.Contains(got, `"serving_protocol":"kserve_v2_http"`) {
		t.Fatalf("serving_protocol not typed snake_case: %s", got)
	}
	if !strings.Contains(got, `"available":true`) {
		t.Fatalf("available flag missing: %s", got)
	}
}

func TestPredictResponseErrorCarriesReason(t *testing.T) {
	t.Parallel()
	resp := inference.PredictResponse{
		Model:  ai.Model{Name: "m"},
		Status: inference.StatusError,
		Reason: "backend timeout",
	}
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got := string(data)
	if !strings.Contains(got, `"status":"error"`) || !strings.Contains(got, `"reason":"backend timeout"`) {
		t.Fatalf("error status/reason missing: %s", got)
	}

	ok := inference.PredictResponse{Model: ai.Model{Name: "m"}, Status: inference.StatusSuccess}
	okData, err := json.Marshal(ok)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(okData), `"reason"`) {
		t.Fatalf("success response must omit reason: %s", okData)
	}
}
