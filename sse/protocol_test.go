package sse

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/kbukum/gokit/contracttest/golden"
	apperrors "github.com/kbukum/gokit/errors"
)

func TestPublishedProtocolFrames(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile("testdata/protocol.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Version int    `json:"version"`
		Epoch   string `json:"epoch"`
		Cases   []struct {
			Name   string `json:"name"`
			Cursor string `json:"cursor"`
			Status int    `json:"status"`
			Wire   string `json:"wire"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.Version != 1 {
		t.Fatal("unknown protocol version")
	}
	for _, c := range fixture.Cases {
		t.Run(c.Name, func(t *testing.T) {
			t.Parallel()
			b := testBus(t, smallLimits())
			b.epoch = fixture.Epoch
			publish(t, b, "bob", "secret")
			publish(t, b, "alice", "visible")
			if c.Name == "evicted" {
				publish(t, b, "bob", "evicts")
			}
			s, err := b.Subscribe(t.Context(), SubscribeRequest{Principal: "alice", Route: "alice", Cursor: c.Cursor})
			if c.Status != 200 {
				if err == nil || apperrors.Normalize(err).HTTPStatus() != c.Status || b.Stats().AllocatedQueues != 0 {
					t.Fatalf("rejection: %v stats=%+v", err, b.Stats())
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			var event Event
			if c.Name == "connected" {
				event, err = controlEvent("connected", s.Connected())
				if err != nil {
					t.Fatal(err)
				}
			} else {
				if c.Name == "overflow" {
					publish(t, b, "alice", "one")
					publish(t, b, "alice", "two")
				}
				event = next(t, s)
			}
			got, err := NewDecoder(strings.NewReader(event.Wire())).Next()
			if err != nil {
				t.Fatal(err)
			}
			want, err := NewDecoder(strings.NewReader(c.Wire)).Next()
			if err != nil {
				t.Fatal(err)
			}
			if got.Event != want.Event || got.ID != want.ID {
				t.Fatalf("frame metadata: %+v != %+v", got, want)
			}
			golden.AssertJSON(t, got.Data, string(want.Data))
		})
	}
}
