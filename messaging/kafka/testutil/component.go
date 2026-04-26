package testutil

import (
	msgtestutil "github.com/kbukum/gokit/messaging/testutil"
)

// Kafka-specific convenience re-exports. The underlying mock types live in
// [github.com/kbukum/gokit/messaging/testutil] (transport-agnostic); these
// aliases let Kafka tests import from a single kafka-flavored package without
// pulling the generic testutil import in every test file.
type (
	Message      = msgtestutil.Message
	MockProducer = msgtestutil.MockProducer
	MockConsumer = msgtestutil.MockConsumer
)

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
