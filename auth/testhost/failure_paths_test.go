package testhost

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kbukum/gokit/auth"
	"github.com/kbukum/gokit/auth/authctx"
	"github.com/kbukum/gokit/auth/session"
	"github.com/kbukum/gokit/codec"
	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/logging"
)

func TestControlledStoreRejectsAllOperationsDuringOutage(t *testing.T) {
	t.Parallel()
	host, _, _ := liveHost(t)
	for _, unavailable := range []bool{true, false} {
		host.store.unavailable.Store(unavailable)
		ctx := t.Context()
		if !unavailable {
			canceled, cancel := context.WithCancel(ctx)
			cancel()
			ctx = canceled
		}
		_, lookupErr := host.store.Lookup(ctx, "missing")
		_, revokeErr := host.store.Revoke(ctx, "missing")
		_, cleanupErr := host.store.Cleanup(ctx, host.clock.Now(), 1)
		for _, err := range []error{
			host.store.Create(ctx, session.Record{}),
			host.store.Rotate(ctx, "missing", session.Record{}),
			host.store.Relogin(ctx, "missing", session.Record{}),
			lookupErr, revokeErr, cleanupErr,
		} {
			if err == nil || (!unavailable && !errors.Is(err, context.Canceled)) {
				t.Fatalf("store operation did not preserve outage/cancellation: %v", err)
			}
		}
	}
	if removed, err := host.store.Cleanup(t.Context(), host.clock.Now(), 1); err != nil || removed != 0 {
		t.Fatalf("healthy cleanup = %d, %v", removed, err)
	}
}

func TestClosedStorageFailsReadinessAndReset(t *testing.T) {
	t.Parallel()
	host, _, fixture := liveHost(t)
	if err := host.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
	for _, handler := range []http.HandlerFunc{host.ready, host.reset} {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/", http.NoBody)
		req.Header.Set("X-Test-Control", fixture.ControlToken)
		rec := httptest.NewRecorder()
		handler(rec, req)
		if rec.Code < 500 {
			t.Fatalf("closed storage returned %d", rec.Code)
		}
	}
	rec := httptest.NewRecorder()
	host.scenario(rec, httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/", http.NoBody))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unprivileged scenario returned %d", rec.Code)
	}
}

func TestProtectedStreamFailsClosed(t *testing.T) {
	t.Parallel()
	host, _, _ := liveHost(t)
	for _, tc := range []struct {
		name      string
		principal *auth.Principal
		host      *Host
		status    int
	}{
		{"missing identity", nil, host, http.StatusUnauthorized},
		{"unknown session", &auth.Principal{Subject: "fixture-user", Credential: auth.Session, Reference: "missing"}, host, http.StatusUnauthorized},
		{"forbidden principal", &auth.Principal{Subject: "other", Kind: auth.User, Credential: auth.APIKey, Restrictions: auth.Restrictions{Mode: auth.Unrestricted}}, host, http.StatusForbidden},
		{"missing bus", &auth.Principal{Subject: "fixture-user", Kind: auth.User, Credential: auth.APIKey, Restrictions: auth.Restrictions{Mode: auth.Unrestricted}}, &Host{log: host.log}, http.StatusUnprocessableEntity},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, EventsPath, http.NoBody)
			if tc.principal != nil {
				req = req.WithContext(authctx.Set(req.Context(), *tc.principal))
			}
			rec := httptest.NewRecorder()
			tc.host.stream(rec, req)
			if rec.Code != tc.status {
				t.Fatalf("stream = %d, want %d", rec.Code, tc.status)
			}
		})
	}
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, OperationPath, http.NoBody)
	req.Header.Set("Content-Type", "text/plain")
	rec := httptest.NewRecorder()
	host.server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized || rec.Header().Get("Content-Type") != "application/problem+json" {
		t.Fatalf("unsupported RPC transport rejection = %d %v", rec.Code, rec.Header())
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := (fixturePolicy{}).Authorize(ctx, auth.Principal{}, "resource-a", nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("policy cancellation = %v", err)
	}
	if err := (fixturePolicy{}).Authorize(t.Context(), auth.Principal{Subject: "other"}, "resource-a", nil); err == nil {
		t.Fatal("policy accepted a different principal")
	}
}

type failingResponse struct{ headers http.Header }

func (w failingResponse) Header() http.Header { return w.headers }
func (failingResponse) WriteHeader(int)       {}
func (failingResponse) Write([]byte) (int, error) {
	return 0, errors.New("disconnected response")
}

func TestFixtureWritersReportEncodingAndWriteFailures(t *testing.T) {
	t.Parallel()
	var logs bytes.Buffer
	log, err := logging.New(&logging.Config{Level: "error", Format: "json"}, "fixture-test", logging.WithWriter(&logs))
	if err != nil {
		t.Fatal(err)
	}
	h := &Host{log: log}
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", http.NoBody)
	w := failingResponse{headers: make(http.Header)}
	h.writeJSON(w, req, Ready{})
	h.writeError(w, req, apperrors.Unauthorized(""))
	h.state(w, req)
	rec := httptest.NewRecorder()
	h.writeError(rec, req, apperrors.InvalidInput("", "").WithDetails(map[string]any{"unsupported": func() {}}))
	if rec.Code != http.StatusInternalServerError || strings.Count(logs.String(), "\"level\"") != 4 {
		t.Fatalf("writer failures not reported: status=%d logs=%s", rec.Code, &logs)
	}
	if err := h.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestStatusFenceBoundsBuffersAndHonorsCancellation(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		body   string
		cancel bool
		status int
	}{
		{"empty", "", false, http.StatusOK},
		{"over budget", strings.Repeat("x", 4097), false, http.StatusInternalServerError},
		{"canceled", "discarded", true, http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := &Host{log: logging.NewDefault("fixture-test")}
			hold := &heldStatus{release: make(chan struct{})}
			h.status.Store(hold)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			handler := h.fenceStatus(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if tc.body != "" {
					_, _ = w.Write([]byte(tc.body))
				}
				if tc.cancel {
					cancel()
				} else {
					hold.close()
				}
			}))
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, httptest.NewRequestWithContext(ctx, http.MethodGet, "/auth/session", http.NoBody))
			if rec.Code != tc.status || h.status.Load() != nil || (tc.cancel && rec.Body.Len() != 0) {
				t.Fatalf("held response = %d, body bytes=%d, retained=%v", rec.Code, rec.Body.Len(), h.status.Load() != nil)
			}
		})
	}
}

func TestFixtureLoadRejectsInvalidJSONAndKey(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "fixture.json")
	if _, err := LoadFixture(path); err == nil {
		t.Fatal("missing fixture accepted")
	}
	fixture, err := NewFixture()
	if err != nil {
		t.Fatal(err)
	}
	fixture.APIKey = "invalid"
	data, err := codec.Encode(codec.CompactJSON(), fixture)
	if err != nil {
		t.Fatal(err)
	}
	for _, data := range []string{"invalid json", data} {
		if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadFixture(path); err == nil {
			t.Fatal("invalid fixture accepted")
		}
	}
	if h, err := New(t.Context(), hostConfig(t), fixture); err == nil {
		_ = h.Close(t.Context())
		t.Fatal("invalid key allowed host startup")
	}
}
