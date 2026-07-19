package consumer

import (
	"context"

	kafkago "github.com/segmentio/kafka-go"
)

type kafkaReader interface {
	ReadMessage(context.Context) (kafkago.Message, error)
	FetchMessage(context.Context) (kafkago.Message, error)
	CommitMessages(context.Context, ...kafkago.Message) error
	Stats() kafkago.ReaderStats
	Close() error
}
