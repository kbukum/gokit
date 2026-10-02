package wire

import (
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/kbukum/gokit/contracttest/golden"
	"github.com/kbukum/gokit/sse"
)

func TestSSEPublishedFailureFrames(t *testing.T) {
	t.Parallel()
	fixtures, err := LoadFixtures()
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range WireCases() {
		t.Run(c.Name, func(t *testing.T) {
			t.Parallel()
			bus, err := sse.NewBus(sse.DefaultLimits())
			if err != nil {
				t.Fatal(err)
			}
			defer bus.Close()
			sub, err := bus.Subscribe(t.Context(), sse.SubscribeRequest{Principal: "fixture", Route: "fixture"})
			if err != nil {
				t.Fatal(err)
			}
			defer sub.Close()
			if err := sub.Fail(c.Err); err != nil {
				t.Fatal(err)
			}
			frame, err := sub.Next(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			if frame.Wire() != fixtures[c.Name].SSEFrame {
				t.Fatalf("SSE fixture drift: %q", frame.Wire())
			}
			decoded, err := sse.NewDecoder(strings.NewReader(frame.Wire())).Next()
			if err != nil {
				t.Fatal(err)
			}
			if decoded.Event != "failure" || decoded.ID != "" {
				t.Fatal("failure advanced cursor")
			}
			expected, err := json.Marshal(c.Err.ToFailure())
			if err != nil {
				t.Fatal(err)
			}
			golden.AssertJSON(t, decoded.Data, string(expected))
			if _, err := sub.Next(t.Context()); !errors.Is(err, io.EOF) {
				t.Fatalf("failure did not close: %v", err)
			}
		})
	}
}
