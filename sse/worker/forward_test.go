package worker_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"google.golang.org/protobuf/types/known/apipb"

	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/sse"
	sseworker "github.com/kbukum/gokit/sse/worker"
	"github.com/kbukum/gokit/worker"
)

func TestForwardProtoEnvelopeAndErrors(t *testing.T) {
	t.Parallel()
	bus, err := sse.NewBus(sse.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	defer bus.Close()
	sub, err := bus.Subscribe(t.Context(), sse.SubscribeRequest{Principal: "p", Route: "task:1"})
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()
	events := make(chan worker.Event[string], 1)
	events <- worker.Event[string]{TaskID: "1", Data: "payload"}
	close(events)
	mapper := func(e worker.Event[string]) (sseworker.Publication, error) {
		return sseworker.Publication{Pattern: "task:" + e.TaskID, Message: &apipb.Method{Name: e.TaskID, RequestTypeUrl: e.Data}}, nil
	}
	if err := sseworker.Forward(t.Context(), events, bus, mapper); err != nil {
		t.Fatal(err)
	}
	frame, err := sub.Next(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if frame.Name != "google.protobuf.Method" || !strings.Contains(frame.Data, `"requestTypeUrl"`) || strings.Contains(frame.Data, "request_type_url") {
		t.Fatalf("noncanonical worker frame: %+v", frame)
	}
	failed := make(chan worker.Event[string], 1)
	failed <- worker.Event[string]{}
	want := apperrors.InvalidInput("event", "cannot map")
	if err := sseworker.Forward(t.Context(), failed, bus, func(worker.Event[string]) (sseworker.Publication, error) {
		return sseworker.Publication{}, want
	}); !errors.Is(err, want) {
		t.Fatalf("mapper error lost: %v", err)
	}
	bus.Close()
	closedBusEvents := make(chan worker.Event[string], 1)
	closedBusEvents <- worker.Event[string]{TaskID: "1"}
	if err := sseworker.Forward(t.Context(), closedBusEvents, bus, mapper); err == nil {
		t.Fatal("publication failure hidden")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := sseworker.Forward(ctx, make(chan worker.Event[string]), bus, mapper); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation lost: %v", err)
	}
}

func TestForwardRejectsTypedNilPublisher(t *testing.T) {
	t.Parallel()
	var publisher sse.Publisher = (*sse.Bus)(nil)
	mapper := func(worker.Event[string]) (sseworker.Publication, error) {
		return sseworker.Publication{}, nil
	}
	// A canceled context makes a guard regression fail as context.Canceled
	// instead of blocking on the open channel forever.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := sseworker.Forward(ctx, make(chan worker.Event[string]), publisher, mapper)
	if err == nil || errors.Is(err, context.Canceled) {
		t.Fatalf("typed-nil publisher not rejected before use: %v", err)
	}
}
