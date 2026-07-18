// Package testutil provides Kafka-specific testing utilities for the messaging/kafka module.
//
// Types are re-exported from the generic messaging/testutil package for backward compatibility. New code should prefer importing messaging/testutil directly.
//
// # Quick Start
//
//	kfk := testutil.NewComponent()
//	testutil.T(t).Setup(kfk)
//
//	// Access mock producer to inspect sent messages
//	kfk.MockProducerClient().Messages() // returns all produced messages
//
//	// Access mock consumer that can be fed messages
//	kfk.MockConsumerClient("my-topic").Feed(Message{Value: []byte("hello")})
package testutil
