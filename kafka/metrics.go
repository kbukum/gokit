package kafka

import (
	"fmt"

	kafkago "github.com/segmentio/kafka-go"
)

// WriterMetrics contains structured publisher metrics.
type WriterMetrics struct {
	Writes       int64   `json:"writes"`
	Messages     int64   `json:"messages"`
	Bytes        int64   `json:"bytes"`
	Errors       int64   `json:"errors"`
	Retries      int64   `json:"retries"`
	AvgWriteTime float64 `json:"avg_write_time_ms"`
	MaxWriteTime float64 `json:"max_write_time_ms"`
	Topic        string  `json:"topic,omitempty"`
}

// ReaderMetrics contains structured consumer metrics.
type ReaderMetrics struct {
	Dials      int64  `json:"dials"`
	Fetches    int64  `json:"fetches"`
	Messages   int64  `json:"messages"`
	Bytes      int64  `json:"bytes"`
	Errors     int64  `json:"errors"`
	Rebalances int64  `json:"rebalances"`
	Offset     int64  `json:"offset"`
	Lag        int64  `json:"lag"`
	Topic      string `json:"topic"`
	Partition  string `json:"partition"`
}

// CollectWriterMetrics extracts structured metrics from kafka.WriterStats.
func CollectWriterMetrics(stats kafkago.WriterStats) WriterMetrics {
	return WriterMetrics{
		Writes:       stats.Writes,
		Messages:     stats.Messages,
		Bytes:        stats.Bytes,
		Errors:       stats.Errors,
		Retries:      stats.Retries,
		AvgWriteTime: float64(stats.WriteTime.Avg) / 1e6,
		MaxWriteTime: float64(stats.WriteTime.Max) / 1e6,
		Topic:        stats.Topic,
	}
}

// CollectReaderMetrics extracts structured metrics from kafka.ReaderStats.
func CollectReaderMetrics(stats kafkago.ReaderStats) ReaderMetrics {
	return ReaderMetrics{
		Dials:      stats.Dials,
		Fetches:    stats.Fetches,
		Messages:   stats.Messages,
		Bytes:      stats.Bytes,
		Errors:     stats.Errors,
		Rebalances: stats.Rebalances,
		Offset:     stats.Offset,
		Lag:        stats.Lag,
		Topic:      stats.Topic,
		Partition:  fmt.Sprintf("%s", stats.Partition),
	}
}
