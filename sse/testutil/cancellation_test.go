package testutil_test

import (
	"context"
	"errors"
	"testing"

	"github.com/kbukum/gokit/sse"
	"github.com/kbukum/gokit/sse/testutil"
)

func TestHarnessCanceledConnection(t *testing.T) {
	t.Parallel()
	h := testutil.New(t, sse.DefaultLimits(), testConfig(sse.PublicAccess("public", "public")))
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	stream, err := h.Connect(ctx, "")
	if !errors.Is(err, context.Canceled) || stream != nil {
		if stream != nil {
			stream.Close()
		}
		t.Fatalf("canceled connection: %v", err)
	}
	if h.Bus.Stats().AllocatedQueues != 0 {
		t.Fatal("canceled request reached admission")
	}
}
