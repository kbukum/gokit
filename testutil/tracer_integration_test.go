//go:build integration

package testutil_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/observability"
	"github.com/kbukum/gokit/testutil"
)

func TestInitTracerSafeNativeExportFailure(t *testing.T) {
	t.Parallel()
	collector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := io.Copy(io.Discard, io.LimitReader(r.Body, 65536)); err != nil {
			t.Error(err)
			return
		}
		w.WriteHeader(http.StatusBadRequest)
		if _, err := io.WriteString(w, "synthetic-private-collector"); err != nil {
			t.Error(err)
		}
	}))
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), time.Second)
		defer cancel()
		if err := testutil.CloseHTTPServer(ctx, collector); err != nil {
			t.Error(err)
		}
	})
	cfg := observability.DefaultTracerConfig("native-lower-level")
	cfg.Endpoint = strings.TrimPrefix(collector.URL, "http://")
	cfg.ExportTimeout = time.Second
	cfg.BatchTimeout = time.Hour
	tp, err := observability.InitTracer(t.Context(), &cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), time.Second)
		defer cancel()
		if err := tp.Shutdown(ctx); err != nil {
			t.Error(err)
		}
	})
	_, span := tp.Tracer("native").Start(t.Context(), "export")
	span.End()
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	err = tp.ForceFlush(ctx)
	var failure *apperrors.AppError
	if !errors.As(err, &failure) || errors.Unwrap(failure) == nil {
		t.Fatal("collector failure was hidden or lost its cause")
	}
	if strings.Contains(err.Error(), collector.URL) || strings.Contains(err.Error(), "synthetic-private-collector") {
		t.Fatal("collector URL or response body entered printable diagnostics")
	}
}
