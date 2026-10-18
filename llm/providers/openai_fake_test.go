package providers_test

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/kbukum/gokit/ai/chat"
	"github.com/kbukum/gokit/httpclient"
	"github.com/kbukum/gokit/llm"
	"github.com/kbukum/gokit/llm/providers/openai"
	"github.com/kbukum/gokit/llm/testutil"
)

// TestOpenAIDialectAgainstFakeServer proves the shared fake speaks the wire format the real OpenAI dialect reads.
func TestOpenAIDialectAgainstFakeServer(t *testing.T) {
	t.Parallel()
	fake := testutil.NewOpenAIServer()
	srv := httptest.NewServer(fake)
	defer srv.Close()

	adapter, err := llm.NewWithDialect(&openai.Dialect{}, llm.Config{
		BaseURL: srv.URL, Model: "m-1",
		Auth: &httpclient.AuthConfig{Type: httpclient.AuthBearer, Token: "token"},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := adapter.Close(context.Background()); err != nil {
			t.Error(err)
		}
	})
	provider := llm.NewProvider(adapter, "m-1")
	req := llm.CompletionRequest{Messages: []chat.Message{chat.User("two words")}}

	events, err := provider.Stream(t.Context(), req)
	if err != nil {
		t.Fatal(err)
	}
	var text string
	var done *llm.CompletionResponse
	for e := range events {
		switch e := e.(type) {
		case llm.TextDelta:
			text += e.Text
		case llm.StreamError:
			t.Fatal(e.Err)
		case llm.MessageComplete:
			done = &e.Response
		}
	}
	if text != testutil.EchoPrefix+"two words" || done == nil || done.Usage.InputTokens != 2 || done.Usage.OutputTokens != 3 {
		t.Fatalf("stream text %q, response %+v", text, done)
	}

	resp, err := provider.Execute(t.Context(), req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Text() != testutil.EchoPrefix+"two words" || resp.Usage.InputTokens != 2 || resp.Usage.OutputTokens != 3 {
		t.Fatalf("response %+v", resp)
	}
	if n := fake.Calls("m-1"); n != 2 {
		t.Fatalf("calls = %d, want 2", n)
	}
}
