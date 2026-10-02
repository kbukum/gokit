package sse_test

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/sse"
	"github.com/kbukum/gokit/sse/testutil"
)

func TestEndpointInStreamFailure(t *testing.T) {
	t.Parallel()
	h := testutil.New(t, sse.DefaultLimits(), handlerConfig())
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	stream := h.MustConnect(t, ctx, "")
	defer stream.Close()
	stream.SkipConnected(t)
	failure := apperrors.ServiceUnavailable("provider").WithRetryAfter(1500 * time.Millisecond).WithReason("BUSY")
	if err := h.Bus.Fail(ctx, "public", failure); err != nil {
		t.Fatal(err)
	}
	frame := stream.Require(t, "failure")
	if !strings.Contains(string(frame.Data), `"retryAfter":1.5`) || !strings.Contains(string(frame.Data), `"reason":"BUSY"`) {
		t.Fatalf("failure lost vocabulary: %s", frame.Data)
	}
	if _, err := stream.Next(); !errors.Is(err, io.EOF) {
		t.Fatalf("failure did not close stream: %v", err)
	}
}
