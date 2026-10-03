package memory

import (
	"context"
	"errors"
	"slices"
	"testing"
	"testing/synctest"
	"time"

	"github.com/kbukum/gokit/messaging"
)

func TestFailedDeliveryRetriesWithoutQueueDeadlockOrFanout(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		broker := NewBrokerWithBuffer(1)
		defer broker.Close()
		consumer := broker.consumer("topic", messaging.CommitAfterHandlerSuccess)
		other := broker.Consumer("topic")
		producer := broker.Producer()
		if err := producer.Send(t.Context(), messaging.Message{Topic: "topic", Key: "first"}); err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(t.Context())
		if err := other.Consume(ctx, func(context.Context, messaging.Message) error { cancel(); return nil }); !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
		cause := errors.New("handler failed")
		ctx, cancel = context.WithTimeout(t.Context(), time.Millisecond)
		err := consumer.Consume(ctx, func(context.Context, messaging.Message) error {
			if err := producer.Send(t.Context(), messaging.Message{Topic: "topic", Key: "second"}); err != nil {
				t.Fatal(err)
			}
			return cause
		})
		if !errors.Is(err, cause) || ctx.Err() != nil {
			t.Errorf("failed delivery blocked until timeout: %v", err)
		}
		cancel()
		ctx, cancel = context.WithTimeout(t.Context(), time.Millisecond)
		defer cancel()
		var seen []string
		err = consumer.Consume(ctx, func(_ context.Context, msg messaging.Message) error {
			seen = append(seen, msg.Key)
			if len(seen) == 2 {
				cancel()
			}
			return nil
		})
		if !errors.Is(err, context.Canceled) || !slices.Equal(seen, []string{"first", "second"}) {
			t.Errorf("retry ordering: %v, %v", seen, err)
		}
		if len(other.ch) != 1 {
			t.Fatal("failed delivery was broadcast again to another subscriber")
		}
	})
}
