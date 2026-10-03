package memory

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"testing/synctest"

	"go.uber.org/goleak"

	"github.com/kbukum/gokit/messaging"
)

func TestMain(m *testing.M) { goleak.VerifyTestMain(m) }

func TestPublishingWithoutSubscribersRetainsNothing(t *testing.T) {
	t.Parallel()
	broker := NewBroker()
	defer broker.Close()
	for _, count := range []int{1000, 10000} {
		for i := range count {
			if err := broker.Producer().Send(t.Context(), messaging.Message{
				Topic: fmt.Sprintf("topic-%d", i), Payload: make([]byte, 1024),
			}); err != nil {
				t.Fatal(err)
			}
		}
		if got := len(broker.Topics()); got != 0 {
			t.Fatalf("retained %d published topics after %d messages, want zero", got, count)
		}
	}
}

func TestConsumerCloseUnsubscribesAndUnblocks(t *testing.T) {
	for _, closeBroker := range []bool{false, true} {
		synctest.Test(t, func(t *testing.T) {
			broker := NewBrokerWithBuffer(1)
			defer broker.Close()
			consumer := broker.Consumer("topic")
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			done := make(chan error, 1)
			go func() { done <- consumer.Consume(ctx, func(context.Context, messaging.Message) error { return nil }) }()
			synctest.Wait()
			if closeBroker {
				broker.Close()
			} else if err := consumer.Close(); err != nil {
				t.Fatal(err)
			}
			synctest.Wait()
			select {
			case err := <-done:
				if !errors.Is(err, messaging.ErrClosed) {
					t.Errorf("close result: %v", err)
				}
			default:
				t.Error("close did not release consumer")
				cancel()
				<-done
			}
			if len(broker.Topics()) != 0 {
				t.Error("closed subscription retained topic")
			}
		})
	}
}
