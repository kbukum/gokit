package testutil_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kbukum/gokit/llm/testutil"
)

func solidPNG(t *testing.T, c color.RGBA) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for y := range 8 {
		for x := range 8 {
			img.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())
}

func imageRequest(t *testing.T, model, url string) string {
	t.Helper()
	body, err := json.Marshal(map[string]any{"model": model, "messages": []any{map[string]any{
		"role": "user",
		"content": []any{
			map[string]any{"type": "text", "text": "What color is this image? Answer with one word."},
			map[string]any{"type": "image_url", "image_url": map[string]string{"url": url}},
		},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func answer(t *testing.T, r reply) string {
	t.Helper()
	var got struct {
		Choices []struct {
			Message struct{ Content string } `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal([]byte(r.body), &got); err != nil || len(got.Choices) != 1 {
		t.Fatalf("reply %d %q: %v", r.status, r.body, err)
	}
	return got.Choices[0].Message.Content
}

func TestVisionModelNamesTheImageColor(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(testutil.NewOpenAIServer())
	defer srv.Close()
	for want, c := range map[string]color.RGBA{
		"red": {255, 0, 0, 255}, "green": {0, 255, 0, 255}, "blue": {0, 0, 255, 255}, "yellow": {255, 255, 0, 255}, "black": {0, 0, 0, 255}, "white": {255, 255, 255, 255}, "unknown": {128, 128, 128, 255},
	} {
		r := postChat(t, srv.URL, "Bearer t", imageRequest(t, testutil.VisionModel, solidPNG(t, c)))
		if r.status != http.StatusOK || answer(t, r) != want {
			t.Errorf("%s: got %d %q", want, r.status, r.body)
		}
	}
}

func TestTextModelRefusesImages(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(testutil.NewOpenAIServer())
	defer srv.Close()
	r := postChat(t, srv.URL, "Bearer t", imageRequest(t, testutil.EchoModel, solidPNG(t, color.RGBA{255, 0, 0, 255})))
	if r.status != http.StatusBadRequest {
		t.Fatalf("text model answered an image: %d %q", r.status, r.body)
	}
}

func TestBadImagesAreRefused(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(testutil.NewOpenAIServer())
	defer srv.Close()
	for _, url := range []string{"https://example.com/a.png", "data:image/png;base64,!!", "data:image/png;base64," + base64.StdEncoding.EncodeToString([]byte("not a png"))} {
		if r := postChat(t, srv.URL, "Bearer t", imageRequest(t, testutil.VisionModel, url)); r.status != http.StatusBadRequest {
			t.Errorf("%q: got %d", url, r.status)
		}
	}
}

func TestTextPartsAreEchoed(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(testutil.NewOpenAIServer())
	defer srv.Close()
	body := `{"model":"m","messages":[{"role":"user","content":[{"type":"text","text":"two"},{"type":"text","text":"words"}]}]}`
	if r := postChat(t, srv.URL, "Bearer t", body); answer(t, r) != testutil.EchoPrefix+"two words" {
		t.Fatalf("got %q", r.body)
	}
}

func TestModelsAreListed(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		opts []testutil.OpenAIServerOption
		want []string
	}{
		{nil, []string{testutil.EchoModel, testutil.VisionModel}},
		{[]testutil.OpenAIServerOption{testutil.WithModels("a", "vision-b")}, []string{"a", "vision-b"}},
	} {
		srv := httptest.NewServer(testutil.NewOpenAIServer(tc.opts...))
		req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL+"/v1/models", http.NoBody)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		var got struct {
			Object string `json:"object"`
			Data   []struct {
				ID     string `json:"id"`
				Object string `json:"object"`
			} `json:"data"`
		}
		err = json.NewDecoder(resp.Body).Decode(&got)
		_ = resp.Body.Close()
		srv.Close()
		if err != nil || got.Object != "list" || len(got.Data) != len(tc.want) {
			t.Fatalf("models: %+v %v", got, err)
		}
		for i, id := range tc.want {
			if got.Data[i].ID != id || got.Data[i].Object != "model" {
				t.Fatalf("model %d: %+v", i, got.Data[i])
			}
		}
	}
}
