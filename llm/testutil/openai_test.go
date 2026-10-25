package testutil_test

import (
	"bufio"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/kbukum/gokit/llm/testutil"
)

type reply struct {
	status      int
	contentType string
	body        string
	events      []string
}

func postChat(t *testing.T, url, auth, body string) reply {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, url+"/v1/chat/completions", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	got := reply{status: resp.StatusCode, contentType: resp.Header.Get("Content-Type")}
	var all strings.Builder
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		all.WriteString(scanner.Text())
		if data, ok := strings.CutPrefix(scanner.Text(), "data: "); ok {
			got.events = append(got.events, data)
		}
	}
	got.body = all.String()
	return got
}

func calls(t *testing.T, url, model string) int {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url+"/calls?model="+model, http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	var got struct {
		Calls int `json:"calls"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	return got.Calls
}

const streamed = `{"model":"m-1","stream":true,"stream_options":{"include_usage":true},` +
	`"messages":[{"role":"user","content":"two words"}]}`

func TestOpenAIServerStreamsEchoWithUsageAndCounts(t *testing.T) {
	t.Parallel()
	fake := testutil.NewOpenAIServer()
	srv := httptest.NewServer(fake)
	defer srv.Close()

	got := postChat(t, srv.URL, "Bearer token", streamed)
	if got.status != http.StatusOK || got.contentType != "text/event-stream" {
		t.Fatalf("status %d, type %q", got.status, got.contentType)
	}
	joined := strings.Join(got.events, "\n")
	for _, want := range []string{
		`"content":"echo: "`, `"content":"two words"`, `"finish_reason":"stop"`,
		`"prompt_tokens":2`, `"completion_tokens":3`, `"total_tokens":5`,
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("stream lacks %s:\n%s", want, joined)
		}
	}
	if last := got.events[len(got.events)-1]; last != "[DONE]" {
		t.Errorf("last event %q", last)
	}
	if n := calls(t, srv.URL, "m-1"); n != 1 || fake.Calls("m-1") != 1 {
		t.Errorf("calls m-1 = %d/%d, want 1", n, fake.Calls("m-1"))
	}
	if n := calls(t, srv.URL, "other"); n != 0 {
		t.Errorf("calls other = %d, want 0", n)
	}
}

func TestOpenAIServerAnswersUnstreamedRequests(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(testutil.NewOpenAIServer())
	defer srv.Close()

	got := postChat(t, srv.URL, "Bearer token", `{"model":"m-1","messages":[{"role":"user","content":"two words"}]}`)
	if got.status != http.StatusOK || got.contentType != "application/json" {
		t.Fatalf("status %d, type %q", got.status, got.contentType)
	}
	var resp struct {
		Model   string `json:"model"`
		Choices []struct {
			Message struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage struct {
			Prompt     int `json:"prompt_tokens"`
			Completion int `json:"completion_tokens"`
			Total      int `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal([]byte(got.body), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Model != "m-1" || len(resp.Choices) != 1 || resp.Choices[0].Message.Content != testutil.EchoPrefix+"two words" ||
		resp.Choices[0].Message.Role != "assistant" || resp.Choices[0].FinishReason != "stop" ||
		resp.Usage.Prompt != 2 || resp.Usage.Completion != 3 || resp.Usage.Total != 5 {
		t.Fatalf("unexpected response: %s", got.body)
	}
}

func TestOpenAIServerFailingModelCountsAndFails(t *testing.T) {
	t.Parallel()
	fake := testutil.NewOpenAIServer()
	srv := httptest.NewServer(fake)
	defer srv.Close()

	model := testutil.FailModelPrefix + "m"
	body := strings.Replace(streamed, `"m-1"`, `"`+model+`"`, 1)
	if got := postChat(t, srv.URL, "Bearer token", body); got.status != http.StatusBadGateway {
		t.Fatalf("status %d, want %d", got.status, http.StatusBadGateway)
	}
	if n := fake.Calls(model); n != 1 {
		t.Errorf("calls = %d, want 1", n)
	}
}

func TestOpenAIServerRefusals(t *testing.T) {
	t.Parallel()
	fake := testutil.NewOpenAIServer()
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)

	for _, tt := range []struct {
		name, auth, body string
		want             int
	}{
		{"no bearer", "", streamed, http.StatusUnauthorized},
		{"empty bearer", "Bearer ", streamed, http.StatusUnauthorized},
		{"no model", "Bearer token", `{"stream":true,"messages":[{"role":"user","content":"x"}]}`, http.StatusBadRequest},
		{"no user message", "Bearer token", `{"model":"m-1","stream":true,"messages":[]}`, http.StatusBadRequest},
		{"not json", "Bearer token", `{`, http.StatusBadRequest},
		{"trailing data", "Bearer token", streamed + `{}`, http.StatusBadRequest},
		{"too large", "Bearer token", `{"model":"` + strings.Repeat("m", testutil.MaxOpenAIRequestBytes) + `"}`, http.StatusRequestEntityTooLarge},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := postChat(t, srv.URL, tt.auth, tt.body); got.status != tt.want {
				t.Errorf("status %d, want %d", got.status, tt.want)
			}
		})
	}
	t.Cleanup(func() {
		if n := fake.Calls("m-1"); n != 0 {
			t.Errorf("refused requests counted: %d", n)
		}
	})
}

func TestOpenAIServerBoundsCountedModels(t *testing.T) {
	t.Parallel()
	fake := testutil.NewOpenAIServer(testutil.WithMaxModels(1))
	srv := httptest.NewServer(fake)
	defer srv.Close()

	if got := postChat(t, srv.URL, "Bearer token", streamed); got.status != http.StatusOK {
		t.Fatalf("first model status %d", got.status)
	}
	other := strings.Replace(streamed, `"m-1"`, `"m-2"`, 1)
	if got := postChat(t, srv.URL, "Bearer token", other); got.status != http.StatusServiceUnavailable {
		t.Fatalf("second model status %d, want %d", got.status, http.StatusServiceUnavailable)
	}
	if got := postChat(t, srv.URL, "Bearer token", streamed); got.status != http.StatusOK || fake.Calls("m-1") != 2 {
		t.Fatalf("known model status %d, calls %d", got.status, fake.Calls("m-1"))
	}
}

func TestOpenAIServerCountsConcurrentCalls(t *testing.T) {
	t.Parallel()
	fake := testutil.NewOpenAIServer()
	srv := httptest.NewServer(fake)
	defer srv.Close()

	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() { postChat(t, srv.URL, "Bearer token", streamed) })
	}
	wg.Wait()
	if n := fake.Calls("m-1"); n != 8 {
		t.Fatalf("calls = %d, want 8", n)
	}
}

func TestOpenAIServerRoutes(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(testutil.NewOpenAIServer())
	defer srv.Close()
	for _, tt := range []struct{ method, path string }{
		{http.MethodGet, "/v1/chat/completions"},
		{http.MethodPost, "/calls?model=m"},
		{http.MethodGet, "/calls"},
		{http.MethodGet, "/other"},
	} {
		req, err := http.NewRequestWithContext(t.Context(), tt.method, srv.URL+tt.path, http.NoBody)
		if err != nil {
			t.Fatal(err)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode < 400 {
			t.Errorf("%s %s answered %d", tt.method, tt.path, resp.StatusCode)
		}
	}
}

func TestWithMaxModelsRejectsNonPositive(t *testing.T) {
	t.Parallel()
	fake := testutil.NewOpenAIServer(testutil.WithMaxModels(0))
	srv := httptest.NewServer(fake)
	defer srv.Close()
	if got := postChat(t, srv.URL, "Bearer token", streamed); got.status != http.StatusOK {
		t.Fatalf("a non-positive bound must keep the default: status %d", got.status)
	}
}

func TestOpenAIServerKeylessAnswersWithoutBearer(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(testutil.NewOpenAIServer(testutil.WithKeyless()))
	t.Cleanup(srv.Close)
	if got := postChat(t, srv.URL, "", streamed); got.status != http.StatusOK {
		t.Fatalf("keyless status %d, want %d", got.status, http.StatusOK)
	}
}
