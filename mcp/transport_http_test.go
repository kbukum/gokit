package mcp_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	kitMcp "github.com/kbukum/gokit/mcp"
	"github.com/kbukum/gokit/tool"
)

func TestNewStreamableHTTPOptions(t *testing.T) {
	t.Parallel()
	opts, protection, err := kitMcp.NewStreamableHTTPOptions(kitMcp.StreamableHTTPConfig{
		Stateless:      true,
		JSONResponse:   true,
		AllowedOrigins: []string{"https://app.example.com"},
	})
	if err != nil {
		t.Fatalf("NewStreamableHTTPOptions: %v", err)
	}
	if protection == nil {
		t.Fatal("expected non-nil cross-origin protection")
	}
	if !opts.Stateless || !opts.JSONResponse {
		t.Errorf("options not carried through: %+v", opts)
	}
	if opts.DisableLocalhostProtection {
		t.Error("localhost protection must default to enabled")
	}
}

func TestNewStreamableHTTPOptionsRejectsBadOrigin(t *testing.T) {
	t.Parallel()
	if _, _, err := kitMcp.NewStreamableHTTPOptions(kitMcp.StreamableHTTPConfig{
		AllowedOrigins: []string{"https://app.example.com/path"},
	}); err == nil {
		t.Fatal("expected rejection of origin with a path")
	}
}

func TestStreamableHTTPHandlerBearerAuth(t *testing.T) {
	t.Parallel()
	server, err := kitMcp.NewServer("s", "1.0.0", tool.NewRegistry())
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	handler, err := server.StreamableHTTPHandler(kitMcp.StreamableHTTPConfig{}, "secret-token")
	if err != nil {
		t.Fatalf("StreamableHTTPHandler: %v", err)
	}

	// Missing token: rejected before reaching the protocol layer.
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/mcp", http.NoBody))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("missing token: got %d want 401", rec.Code)
	}

	// Wrong token: rejected.
	rec = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/mcp", http.NoBody)
	req.Header.Set("Authorization", "Bearer nope")
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("wrong token: got %d want 401", rec.Code)
	}
}

func TestStreamableHTTPHandlerBadOrigin(t *testing.T) {
	t.Parallel()
	server, err := kitMcp.NewServer("s", "1.0.0", tool.NewRegistry())
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	if _, err := server.StreamableHTTPHandler(kitMcp.StreamableHTTPConfig{
		AllowedOrigins: []string{"ftp://bad"},
	}, ""); err == nil {
		t.Fatal("expected error for invalid allowed origin")
	}
}

func TestStreamableHTTPHandlerNoAuth(t *testing.T) {
	t.Parallel()
	server, err := kitMcp.NewServer("s", "1.0.0", tool.NewRegistry())
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	handler, err := server.StreamableHTTPHandler(kitMcp.StreamableHTTPConfig{}, "")
	if err != nil {
		t.Fatalf("StreamableHTTPHandler: %v", err)
	}
	if handler == nil {
		t.Fatal("expected a non-nil handler without bearer auth")
	}
}
