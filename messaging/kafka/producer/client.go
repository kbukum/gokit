package producer

import (
	"context"

	kafkago "github.com/segmentio/kafka-go"
)

type kafkaWriter interface {
	WriteMessages(context.Context, ...kafkago.Message) error
	Stats() kafkago.WriterStats
	Close() error
}
