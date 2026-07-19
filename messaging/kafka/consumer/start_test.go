package consumer

import (
	"context"
	"testing"
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
