package inference

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/kbukum/gokit/ai"
)

func TestOAICompatPredictMapsResponse(t *testing.T) {
	t.Parallel()

	var gotMethod, gotPath string
	var gotBody map[string]any
	exec := func(_ context.Context, method, path string, body any) ([]byte, error) {
		gotMethod = method
		gotPath = path
		var ok bool
		gotBody, ok = body.(map[string]any)
		if !ok {
			t.Fatalf("body type = %T", body)
		}
		return []byte(`{"choices":[{"text":"hello","finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":3,"total_tokens":5}}`), nil
	}

	resp, err := OAICompatPredict(context.Background(), "vllm", exec, PredictRequest{
		ModelName:  "demo",
		Inputs:     map[string]Value{"prompt": TextValue("hi")},
		Parameters: map[string]any{"temperature": 0.25},
	})
	if err != nil {
		t.Fatalf("OAICompatPredict: %v", err)
	}
	if gotMethod != http.MethodPost || gotPath != "/v1/completions" {
		t.Fatalf("request = %s %s", gotMethod, gotPath)
	}
	if gotBody["model"] != "demo" || gotBody["prompt"] != "hi" || gotBody["temperature"] != 0.25 {
		t.Fatalf("body = %#v", gotBody)
	}
	if resp.Outputs["text"].Text != "hello" || resp.Status != StatusSuccess {
		t.Fatalf("response = %+v", resp)
	}
	if resp.Model.Name != "demo" || resp.Model.Provider != ai.ProviderCustom {
		t.Fatalf("model = %+v", resp.Model)
	}
	if resp.Usage.InputTokens != 2 || resp.Usage.OutputTokens != 3 {
		t.Fatalf("usage = %+v", resp.Usage)
	}
	if resp.Metadata["finish_reason"] != "stop" {
		t.Fatalf("metadata = %+v", resp.Metadata)
	}
}

func TestOAICompatPredictAcceptsTextAliasAndEmptyFinishReason(t *testing.T) {
	t.Parallel()

	exec := func(context.Context, string, string, any) ([]byte, error) {
		return []byte(`{"choices":[{"text":"alias","finish_reason":"  "}],"usage":{}}`), nil
	}

	resp, err := OAICompatPredict(context.Background(), "tgi", exec, PredictRequest{
		ModelName: "demo",
		Inputs:    map[string]Value{"text": TextValue("hi")},
	})
	if err != nil {
		t.Fatalf("OAICompatPredict: %v", err)
	}
	if resp.Outputs["text"].Text != "alias" {
		t.Fatalf("outputs = %+v", resp.Outputs)
	}
	if len(resp.Metadata) != 0 {
		t.Fatalf("metadata = %+v", resp.Metadata)
	}
}

func TestOAICompatPredictErrors(t *testing.T) {
	t.Parallel()

	boom := errors.New("boom")
	cases := []struct {
		name    string
		req     PredictRequest
		exec    OAICompatExecuteFunc
		wantErr string
	}{
		{
			name:    "missing prompt",
			req:     PredictRequest{ModelName: "demo"},
			exec:    func(context.Context, string, string, any) ([]byte, error) { return nil, nil },
			wantErr: "missing required input",
		},
		{
			name:    "wrong prompt kind",
			req:     PredictRequest{ModelName: "demo", Inputs: map[string]Value{"prompt": BytesValue([]byte("x"))}},
			exec:    func(context.Context, string, string, any) ([]byte, error) { return nil, nil },
			wantErr: "must be Text",
		},
		{
			name:    "missing model",
			req:     PredictRequest{Inputs: map[string]Value{"prompt": TextValue("hi")}},
			exec:    func(context.Context, string, string, any) ([]byte, error) { return nil, nil },
			wantErr: "model name is required",
		},
		{
			name:    "transport",
			req:     PredictRequest{ModelName: "demo", Inputs: map[string]Value{"prompt": TextValue("hi")}},
			exec:    func(context.Context, string, string, any) ([]byte, error) { return nil, boom },
			wantErr: "completions request failed",
		},
		{
			name:    "malformed json",
			req:     PredictRequest{ModelName: "demo", Inputs: map[string]Value{"prompt": TextValue("hi")}},
			exec:    func(context.Context, string, string, any) ([]byte, error) { return []byte(`{`), nil },
			wantErr: "parse response",
		},
		{
			name:    "no choices",
			req:     PredictRequest{ModelName: "demo", Inputs: map[string]Value{"prompt": TextValue("hi")}},
			exec:    func(context.Context, string, string, any) ([]byte, error) { return []byte(`{"choices":[]}`), nil },
			wantErr: "no choices",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := OAICompatPredict(context.Background(), "vllm", tc.exec, tc.req)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %v, want %q", err, tc.wantErr)
			}
		})
	}
}

func FuzzOAICompatPredictResponse(f *testing.F) {
	f.Add(`{"choices":[{"text":"ok"}]}`)
	f.Add(`{"choices":[]}`)
	f.Add(`{bad}`)
	f.Fuzz(func(t *testing.T, payload string) {
		exec := func(context.Context, string, string, any) ([]byte, error) {
			return []byte(payload), nil
		}
		_, _ = OAICompatPredict(context.Background(), "vllm", exec, PredictRequest{
			ModelName: "demo",
			Inputs:    map[string]Value{"prompt": TextValue("hi")},
		})
	})
}

func TestOAICompatPredictJSONRoundTrip(t *testing.T) {
	t.Parallel()

	exec := func(_ context.Context, _ string, _ string, body any) ([]byte, error) {
		if _, err := json.Marshal(body); err != nil {
			t.Fatalf("request body must be JSON: %v", err)
		}
		return []byte(`{"choices":[{"text":"ok"}]}`), nil
	}
	if _, err := OAICompatPredict(context.Background(), "vllm", exec, PredictRequest{
		ModelName:  "demo",
		Inputs:     map[string]Value{"prompt": TextValue("hi")},
		Parameters: map[string]any{"max_tokens": 8},
	}); err != nil {
		t.Fatalf("OAICompatPredict: %v", err)
	}
}
