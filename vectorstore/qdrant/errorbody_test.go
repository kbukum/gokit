package qdrant

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestExpectStatus_BoundsErrorBody(t *testing.T) {
	t.Parallel()

	big := strings.Repeat("z", maxErrorBodyBytes*2)
	resp := &http.Response{
		StatusCode: http.StatusInternalServerError,
		Body:       io.NopCloser(bytes.NewReader([]byte(big))),
	}

	err := expectStatus(resp, "upsert")
	if err == nil {
		t.Fatal("expected an error for status 500")
	}
	// The error message embeds the body; it must not carry more than the cap.
	if n := strings.Count(err.Error(), "z"); n > maxErrorBodyBytes {
		t.Fatalf("error body carried %d bytes, want <= %d", n, maxErrorBodyBytes)
	}
}
