package inmem_test

import (
	"context"
	"testing"

	"github.com/kbukum/gokit/ai"
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
