// Package testutil provides testing utilities for the kafka module.
//
// It includes mock producer and consumer implementations and a test component
// that implements both component.Component and testutil.TestComponent interfaces.
//
// # Quick Start
//
//	kfk := testutil.NewComponent()
//	testutil.T(t).Setup(kfk)
//
//	// Access mock producer to inspect sent messages
//	kfk.MockProducer().Messages() // returns all produced messages
//
//	// Access mock consumer that can be fed messages
//	kfk.MockConsumer("my-topic").Feed(kafka.Message{Value: []byte("hello")})
package testutil
