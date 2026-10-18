package consumer

import (
	"context"
	"errors"
	"testing"

	"github.com/kbukum/gokit/logging"
	"github.com/kbukum/gokit/messaging/kafka"
)

func TestStartConsumerRejectsNilHandler(t *testing.T) {
	t.Parallel()

	_, err := StartConsumer(context.Background(), StartConsumerConfig{
		GroupID: "g",
		Topic:   "t",
	})
	if err == nil {
		t.Fatal("StartConsumer with nil Handler: got nil error, want failure")
	}
}

func TestStartConsumerClosesOnEndedContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := StartConsumer(ctx, StartConsumerConfig{
		Config:  kafka.Config{Brokers: []string{"127.0.0.1:1"}},
		GroupID: "g",
		Topic:   "t",
		Handler: func(context.Context, []byte) error { return nil },
		Log:     logging.MustNew(&logging.Config{Level: "error"}, "test"),
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("StartConsumer = %v, want context.Canceled", err)
	}
}
