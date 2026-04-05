package consumer

import (
	"github.com/kbukum/gokit/messaging"
)

// AsRunner wraps a Consumer with a MessageHandler to create a messaging.ConsumerRunner
// suitable for use with kafka.Component.AddConsumer().
//
// This delegates to messaging.AsRunner, which provides the generic runner pattern.
func AsRunner(c *Consumer, h messaging.MessageHandler) messaging.ConsumerRunner {
	return messaging.AsRunner(c, h)
}
