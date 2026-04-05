package testutil

import (
	msgtestutil "github.com/kbukum/gokit/messaging/testutil"
)

// Type aliases — re-export generic test types for backward compatibility.
// All types are defined in messaging/testutil and reused here.
type Message = msgtestutil.Message
type MockProducer = msgtestutil.MockProducer
type MockConsumer = msgtestutil.MockConsumer

// NewMockConsumer creates a mock consumer for the given topic.
var NewMockConsumer = msgtestutil.NewMockConsumer

// Component is a test Kafka component with mock producer and consumers.
// It wraps the generic messaging/testutil.Component with the "kafka-test" name.
type Component struct {
	*msgtestutil.Component
}

// NewComponent creates a new mock Kafka test component.
func NewComponent() *Component {
	return &Component{
		Component: msgtestutil.NewComponent("kafka-test"),
	}
}
