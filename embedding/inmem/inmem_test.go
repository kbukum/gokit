package inmem_test

import (
	"context"
	"testing"

	"github.com/kbukum/gokit/ai"
	"github.com/kbukum/gokit/component"
	"github.com/kbukum/gokit/embedding"
	"github.com/kbukum/gokit/embedding/inmem"
)

func TestProviderDeterministicTextEmbeddings(t *testing.T) {
	p := inmem.New(4)
	req := embedding.EmbedRequest{Model: ai.Model{Name: "test", Provider: ai.ProviderCustom}, Inputs: []embedding.EmbedInput{embedding.Text{Text: "hello"}}}
	first, err := p.Execute(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	second, err := p.Execute(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if first.Embedding.Dimensions != 4 || len(first.Embedding.Vector) != 4 {
		t.Fatalf("embedding=%+v", first.Embedding)
	}
	for i := range first.Embedding.Vector {
		if first.Embedding.Vector[i] != second.Embedding.Vector[i] {
			t.Fatalf("not deterministic")
		}
	}
}

func TestProviderBatchAndUnsupportedInput(t *testing.T) {
	p := inmem.New(2)
	responses, err := p.EmbedBatch(context.Background(), []embedding.EmbedRequest{{Inputs: []embedding.EmbedInput{embedding.Text{Text: "a"}}}, {Inputs: []embedding.EmbedInput{embedding.Text{Text: "b"}}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(responses) != 2 || responses[1].Embedding.Index != 0 {
		t.Fatalf("responses=%+v", responses)
	}
	_, err = p.Execute(context.Background(), embedding.EmbedRequest{Inputs: []embedding.EmbedInput{embedding.Image{URL: "x"}}})
	if err == nil {
		t.Fatal("expected unsupported input error")
	}
}

func TestNewClampsNonPositiveDimensions(t *testing.T) {
	p := inmem.New(0)
	resp, err := p.Execute(context.Background(), embedding.EmbedRequest{
		Inputs: []embedding.EmbedInput{embedding.Text{Text: "x"}},
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if resp.Embedding.Dimensions != 8 {
		t.Fatalf("default dimensions = %d, want 8", resp.Embedding.Dimensions)
	}
}

func TestProviderMetadata(t *testing.T) {
	p := inmem.New(4)
	if p.Name() != "embedding-inmem" {
		t.Fatalf("Name = %q", p.Name())
	}
	if !p.IsAvailable(context.Background()) {
		t.Fatal("IsAvailable should be true")
	}
}

func TestExecuteEmptyInputs(t *testing.T) {
	p := inmem.New(4)
	resp, err := p.Execute(context.Background(), embedding.EmbedRequest{})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(resp.Embeddings) != 0 || len(resp.Embedding.Vector) != 0 {
		t.Fatalf("empty inputs should yield no embeddings, got %+v", resp)
	}
	if resp.Model.Name != "inmem-embedding" || resp.Model.Provider != ai.ProviderCustom {
		t.Fatalf("model defaults = %+v", resp.Model)
	}
}

func TestExecuteNilPointerTextRejected(t *testing.T) {
	p := inmem.New(2)
	var nilText *embedding.Text
	_, err := p.Execute(context.Background(), embedding.EmbedRequest{
		Inputs: []embedding.EmbedInput{nilText},
	})
	if err == nil {
		t.Fatal("expected error for nil *Text input")
	}
}

func TestModelEchoPreservesProvidedName(t *testing.T) {
	p := inmem.New(2)
	resp, err := p.Execute(context.Background(), embedding.EmbedRequest{
		Model:  ai.Model{Name: "custom", Provider: ai.ProviderOpenAI},
		Inputs: []embedding.EmbedInput{embedding.Text{Text: "x"}},
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if resp.Model.Name != "custom" || resp.Model.Provider != ai.ProviderOpenAI {
		t.Fatalf("model echo = %+v, want custom/openai", resp.Model)
	}
}

func TestLifecycleStartStopHealth(t *testing.T) {
	ctx := context.Background()
	p := inmem.New(4)

	// Before Start: degraded.
	if h := p.Health(ctx); h.Status != component.StatusDegraded {
		t.Fatalf("pre-start health = %+v, want degraded", h)
	}

	if err := p.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// After Start, before any call: healthy with "ready".
	h := p.Health(ctx)
	if h.Status != component.StatusHealthy || h.Message != "ready" {
		t.Fatalf("post-start health = %+v, want healthy/ready", h)
	}

	// A successful Execute records the last call, reflected in health.
	if _, err := p.Execute(ctx, embedding.EmbedRequest{
		Inputs: []embedding.EmbedInput{embedding.Text{Text: "x"}},
	}); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	h = p.Health(ctx)
	if h.Status != component.StatusHealthy || h.Message == "ready" {
		t.Fatalf("health after call = %+v, want last_call message", h)
	}

	if err := p.Stop(ctx); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if h := p.Health(ctx); h.Status != component.StatusDegraded {
		t.Fatalf("post-stop health = %+v, want degraded", h)
	}
}

func TestProviderAcceptsPointerTextInput(t *testing.T) {
	p := inmem.New(2)
	input := &embedding.Text{Text: "hello"}
	resp, err := p.Execute(context.Background(), embedding.EmbedRequest{Inputs: []embedding.EmbedInput{input}})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(resp.Embedding.Vector) != 2 {
		t.Fatalf("embedding=%+v", resp.Embedding)
	}
}
