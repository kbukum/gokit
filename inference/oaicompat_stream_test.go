package inference

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/kbukum/gokit/ai"
	"github.com/kbukum/gokit/httpclient/sse"
)

func sseReaderFrom(s string) sse.Reader {
	return sse.NewReader(io.NopCloser(strings.NewReader(s)))
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("transport failure") }

var errUnexpectedOpen = errors.New("stream opened unexpectedly")

func collectStream(t *testing.T, ch <-chan ai.StreamEvent) (string, []error) {
	t.Helper()
	var text strings.Builder
	var errs []error
	for ev := range ch {
		switch e := ev.(type) {
		case ai.TextDelta:
			text.WriteString(e.Text)
		case ai.Error:
			errs = append(errs, e.Err)
		}
	}
	return text.String(), errs
}

func TestOAICompatPredictStream_TextDeltas(t *testing.T) {
	t.Parallel()
	stream := func(_ context.Context, path string, _ any) (sse.Reader, error) {
		if path != "/v1/completions" {
			t.Fatalf("path = %q", path)
		}
		return sseReaderFrom(
			"data: {\"choices\":[{\"text\":\"Hel\"}]}\n\n" +
				"data: {\"choices\":[{\"text\":\"lo\"}]}\n\n" +
				"data: {\"choices\":[{\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":3,\"completion_tokens\":2}}\n\n" +
				"data: [DONE]\n\n",
		), nil
	}
	req := PredictRequest{ModelName: "m", Inputs: map[string]Value{"prompt": TextValue("hi")}}

	ch, err := OAICompatPredictStream(context.Background(), "vllm", stream, req)
	if err != nil {
		t.Fatalf("OAICompatPredictStream: %v", err)
	}
	got, errs := collectStream(t, ch)
	if got != "Hello" {
		t.Fatalf("text = %q, want %q", got, "Hello")
	}
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
}

func TestOAICompatPredictStream_MissingModel(t *testing.T) {
	t.Parallel()
	stream := func(context.Context, string, any) (sse.Reader, error) {
		t.Fatal("stream should not open without a model")
		return nil, errUnexpectedOpen
	}
	req := PredictRequest{Inputs: map[string]Value{"prompt": TextValue("hi")}}
	if _, err := OAICompatPredictStream(context.Background(), "vllm", stream, req); err == nil {
		t.Fatal("expected error for missing model")
	}
}

func TestOAICompatPredictStream_MissingPrompt(t *testing.T) {
	t.Parallel()
	stream := func(context.Context, string, any) (sse.Reader, error) {
		t.Fatal("stream should not open without a prompt")
		return nil, errUnexpectedOpen
	}
	req := PredictRequest{ModelName: "m"}
	if _, err := OAICompatPredictStream(context.Background(), "tgi", stream, req); err == nil {
		t.Fatal("expected error for missing prompt")
	}
}

func TestOAICompatPredictStream_OpenError(t *testing.T) {
	t.Parallel()
	stream := func(context.Context, string, any) (sse.Reader, error) {
		return nil, errors.New("boom")
	}
	req := PredictRequest{ModelName: "m", Inputs: map[string]Value{"prompt": TextValue("hi")}}
	if _, err := OAICompatPredictStream(context.Background(), "tgi", stream, req); err == nil {
		t.Fatal("expected error when stream fails to open")
	}
}

func TestOAICompatPredictStream_InvalidChunk(t *testing.T) {
	t.Parallel()
	stream := func(context.Context, string, any) (sse.Reader, error) {
		return sseReaderFrom("data: {not json}\n\n"), nil
	}
	req := PredictRequest{ModelName: "m", Inputs: map[string]Value{"prompt": TextValue("hi")}}
	ch, err := OAICompatPredictStream(context.Background(), "vllm", stream, req)
	if err != nil {
		t.Fatalf("OAICompatPredictStream: %v", err)
	}
	_, errs := collectStream(t, ch)
	if len(errs) == 0 {
		t.Fatal("expected a stream error event for invalid chunk")
	}
}

func TestOAICompatPredictStream_ContextCancel(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	stream := func(context.Context, string, any) (sse.Reader, error) {
		return sseReaderFrom("data: {\"choices\":[{\"text\":\"x\"}]}\n\n"), nil
	}
	req := PredictRequest{ModelName: "m", Inputs: map[string]Value{"prompt": TextValue("hi")}}
	ch, err := OAICompatPredictStream(ctx, "vllm", stream, req)
	if err != nil {
		t.Fatalf("OAICompatPredictStream: %v", err)
	}
	// Draining must terminate promptly even though ctx is already canceled.
	for range ch { //nolint:revive // draining
	}
}

func TestOAICompatPredictStream_ReadError(t *testing.T) {
	t.Parallel()
	stream := func(context.Context, string, any) (sse.Reader, error) {
		return sse.NewReader(io.NopCloser(errReader{})), nil
	}
	req := PredictRequest{ModelName: "m", Inputs: map[string]Value{"prompt": TextValue("hi")}}
	ch, err := OAICompatPredictStream(context.Background(), "vllm", stream, req)
	if err != nil {
		t.Fatalf("OAICompatPredictStream: %v", err)
	}
	_, errs := collectStream(t, ch)
	if len(errs) == 0 {
		t.Fatal("expected a stream error event for a transport read failure")
	}
}

func FuzzOAICompatPredictStream(f *testing.F) {
	f.Add("data: {\"choices\":[{\"text\":\"x\"}]}\n\ndata: [DONE]\n\n")
	f.Add("data: {bad}\n\n")
	f.Add("garbage\n\n")
	f.Add("")
	f.Fuzz(func(t *testing.T, sseText string) {
		stream := func(context.Context, string, any) (sse.Reader, error) {
			return sseReaderFrom(sseText), nil
		}
		req := PredictRequest{ModelName: "m", Inputs: map[string]Value{"prompt": TextValue("hi")}}
		ch, err := OAICompatPredictStream(context.Background(), "vllm", stream, req)
		if err != nil {
			return
		}
		for range ch { //nolint:revive // draining must never panic
		}
	})
}
