package httpclient

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"testing"
)

type finalBytesBody struct{ data []byte }

func (b *finalBytesBody) Read(p []byte) (int, error) {
	n := copy(p, b.data)
	b.data = b.data[n:]
	return n, io.EOF
}
func (*finalBytesBody) Close() error { return nil }

func TestStreamRawEOFRemainsEOF(t *testing.T) {
	c, err := New(Config{})
	if err != nil {
		t.Fatal(err)
	}
	c.httpClient.Transport = streamRoundTrip(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: &finalBytesBody{data: []byte("{\"ok\":true}\n")}}, nil
	})
	resp, err := c.DoStream(context.Background(), Request{Method: "GET", Path: "http://example.test"})
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Close()
	decoder := json.NewDecoder(resp.Body)
	var value struct {
		OK bool `json:"ok"`
	}
	if err := decoder.Decode(&value); err != nil || !value.OK {
		t.Fatalf("decode=%v value=%+v", err, value)
	}
	for range 3 {
		if err := decoder.Decode(&value); !errors.Is(err, io.EOF) {
			t.Fatalf("completed stream returned %v instead of EOF", err)
		}
	}
}
