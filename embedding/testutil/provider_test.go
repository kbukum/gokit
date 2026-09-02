package testutil_test

import (
	"context"
	"errors"
	"testing"

	"github.com/kbukum/gokit/embedding"
	"github.com/kbukum/gokit/embedding/testutil"
)

func textReq(texts ...string) embedding.EmbedRequest {
	inputs := make([]embedding.EmbedInput, len(texts))
	for i, t := range texts {
		inputs[i] = embedding.Text{Text: t}
	}
	return embedding.EmbedRequest{Inputs: inputs}
}

func TestFakeProviderDeterministic(t *testing.T) {
	t.Parallel()

	p := testutil.NewFakeProvider(testutil.WithDimensions(4))
	a, err := p.Execute(context.Background(), textReq("hello"))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	b, err := p.Execute(context.Background(), textReq("hello"))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(a.Embeddings[0].Vector) != 4 {
		t.Fatalf("dimensions = %d, want 4", len(a.Embeddings[0].Vector))
	}
	for i := range a.Embeddings[0].Vector {
		if a.Embeddings[0].Vector[i] != b.Embeddings[0].Vector[i] {
			t.Fatalf("same text produced different vectors at %d", i)
		}
	}
}

func TestFakeProviderInjectedError(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("boom")
	p := testutil.NewFakeProvider(testutil.WithError(sentinel))
	if _, err := p.EmbedBatch(context.Background(), []embedding.EmbedRequest{textReq("x")}); !errors.Is(err, sentinel) {
		t.Fatalf("EmbedBatch error = %v, want %v", err, sentinel)
	}
}

func TestFakeProviderPinnedVectorDimensions(t *testing.T) {
	t.Parallel()

	p := testutil.NewFakeProvider(
		testutil.WithVector("short", []float32{1, 0}),
		testutil.WithVector("long", []float32{1, 0, 0, 0}),
	)
	resp, err := p.Execute(context.Background(), textReq("short", "long"))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := len(resp.Embeddings[0].Vector); got != 2 {
		t.Errorf("pinned short len = %d, want 2", got)
	}
	if got := len(resp.Embeddings[1].Vector); got != 4 {
		t.Errorf("pinned long len = %d, want 4", got)
	}
}

func TestFakeProviderBlockUntilCancel(t *testing.T) {
	t.Parallel()

	p := testutil.NewFakeProvider(testutil.WithBlockUntilCancel())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := p.Execute(ctx, textReq("x")); !errors.Is(err, context.Canceled) {
		t.Fatalf("Execute error = %v, want context.Canceled", err)
	}
}

func TestFakeProviderWithVectorClonesInput(t *testing.T) {
	t.Parallel()

	vec := []float32{1, 2}
	p := testutil.NewFakeProvider(testutil.WithVector("k", vec))
	vec[0] = 99 // mutate caller slice after registration

	resp, err := p.Execute(context.Background(), embedding.EmbedRequest{
		Inputs: []embedding.EmbedInput{embedding.Text{Text: "k"}},
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := resp.Embeddings[0].Vector[0]; got != 1 {
		t.Errorf("pinned vector[0] = %v, want 1 (input mutation must not leak)", got)
	}
	// Mutating a returned vector must not corrupt later responses.
	resp.Embeddings[0].Vector[0] = 42
	resp2, _ := p.Execute(context.Background(), embedding.EmbedRequest{
		Inputs: []embedding.EmbedInput{embedding.Text{Text: "k"}},
	})
	if got := resp2.Embeddings[0].Vector[0]; got != 1 {
		t.Errorf("pinned vector[0] = %v after response mutation, want 1", got)
	}
}

func TestFakeProviderRejectsUnsupportedInput(t *testing.T) {
	t.Parallel()

	// The reusable double must reject non-text modalities exactly like the real
	// inmem provider, so it cannot mask a consumer that sends an unsupported input.
	for _, tc := range []struct {
		name  string
		input embedding.EmbedInput
	}{
		{"image", embedding.Image{URL: "http://x"}},
		{"audio", embedding.Audio{URL: "http://x"}},
		{"video", embedding.Video{URL: "http://x"}},
		{"nil_text_pointer", (*embedding.Text)(nil)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			p := testutil.NewFakeProvider()
			if _, err := p.Execute(context.Background(), embedding.EmbedRequest{
				Inputs: []embedding.EmbedInput{tc.input},
			}); err == nil {
				t.Fatalf("Execute(%s) = nil error, want unsupported-input error", tc.name)
			}
		})
	}
}

func TestFakeProviderCountsCanceledCalls(t *testing.T) {
	t.Parallel()

	p := testutil.NewFakeProvider()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := p.Execute(ctx, embedding.EmbedRequest{
		Inputs: []embedding.EmbedInput{embedding.Text{Text: "x"}},
	}); err == nil {
		t.Fatal("expected cancellation error, got nil")
	}
	if p.Calls() != 1 {
		t.Errorf("Calls() = %d, want 1 (canceled calls must be counted)", p.Calls())
	}
}
