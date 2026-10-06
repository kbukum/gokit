package client

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func h2cServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	server := httptest.NewUnstartedServer(handler)
	server.Config.Protocols = new(http.Protocols)
	server.Config.Protocols.SetUnencryptedHTTP2(true)
	server.Start()
	t.Cleanup(server.Close)
	return server
}

func h2cClient(t *testing.T) *http.Client {
	t.Helper()
	client, err := NewHTTPClient(Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.CloseIdleConnections)
	return client
}

// fail performs a request expected to fail before any response.
func fail(ctx context.Context, t *testing.T, client *http.Client, url string) error {
	t.Helper()
	resp, err := get(ctx, t, client, url)
	if resp != nil {
		resp.Body.Close()
	}
	return err
}

func get(ctx context.Context, t *testing.T, client *http.Client, url string) (*http.Response, error) {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	return client.Do(req)
}

func TestTransportFailureMarksInterruptedBody(t *testing.T) {
	t.Parallel()
	server := h2cServer(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "partial")
		http.NewResponseController(w).Flush()
		panic(http.ErrAbortHandler)
	})
	resp, err := get(t.Context(), t, h2cClient(t), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if string(body) != "partial" || !IsTransportFailure(err) || !errors.Is(err, ErrTransport) || !strings.Contains(err.Error(), "stream error") {
		t.Fatalf("interrupted body = %q, %v", body, err)
	}
}

func TestTransportFailureLeavesCleanEndUnmarked(t *testing.T) {
	t.Parallel()
	server := h2cServer(t, func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "ok") })
	resp, err := get(t.Context(), t, h2cClient(t), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	buf := make([]byte, 8)
	var readErr error
	for readErr == nil {
		_, readErr = resp.Body.Read(buf)
	}
	if readErr != io.EOF || IsTransportFailure(readErr) || IsTransportFailure(nil) {
		t.Fatalf("clean end = %v", readErr)
	}
}

func TestTransportFailureMarksUnreachablePeer(t *testing.T) {
	t.Parallel()
	server := h2cServer(t, func(http.ResponseWriter, *http.Request) {})
	url := server.URL
	server.Close()
	if err := fail(t.Context(), t, h2cClient(t), url); !IsTransportFailure(err) {
		t.Fatalf("unreachable peer = %v", err)
	}
}

func TestTransportFailureExcludesCallerCancellationAndConfiguration(t *testing.T) {
	t.Parallel()
	server := h2cServer(t, func(http.ResponseWriter, *http.Request) {})
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	client := h2cClient(t)
	if err := fail(ctx, t, client, server.URL); IsTransportFailure(err) || !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled request = %v", err)
	}
	if err := fail(t.Context(), t, client, "https://example.invalid"); err == nil || IsTransportFailure(err) {
		t.Fatalf("scheme mismatch = %v", err)
	}
}
