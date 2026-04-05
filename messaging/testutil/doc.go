// Package testutil provides broker-agnostic testing utilities for the
// messaging module.
//
// It includes mock producer and consumer implementations and a test component
// that implements both component.Component and testutil.TestComponent interfaces.
// These mocks work with any broker — they communicate via in-memory channels
// and slices, requiring no running infrastructure.
//
// # Quick Start
//
//	comp := testutil.NewComponent("my-broker")
//	// start, use, stop...
//	comp.MockProducerClient().Messages()        // inspect sent messages
//	comp.MockConsumerClient("topic").Feed(msg)   // feed messages to consumer
package testutil
