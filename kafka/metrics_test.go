package kafka

import (
	"testing"
	"time"

	kafkago "github.com/segmentio/kafka-go"
)

func TestCollectWriterMetrics(t *testing.T) {
	stats := kafkago.WriterStats{
		Writes:   100,
		Messages: 500,
		Bytes:    1024,
		Errors:   2,
		Retries:  5,
		WriteTime: kafkago.DurationStats{
			Avg: 10 * time.Millisecond,
			Max: 50 * time.Millisecond,
		},
		Topic: "test-topic",
	}

	m := CollectWriterMetrics(stats)
	if m.Writes != 100 {
		t.Errorf("Writes = %d, want 100", m.Writes)
	}
	if m.Messages != 500 {
		t.Errorf("Messages = %d, want 500", m.Messages)
	}
	if m.Bytes != 1024 {
		t.Errorf("Bytes = %d, want 1024", m.Bytes)
	}
	if m.Errors != 2 {
		t.Errorf("Errors = %d, want 2", m.Errors)
	}
	if m.Retries != 5 {
		t.Errorf("Retries = %d, want 5", m.Retries)
	}
	if m.AvgWriteTime != 10.0 {
		t.Errorf("AvgWriteTime = %f, want 10.0", m.AvgWriteTime)
	}
	if m.MaxWriteTime != 50.0 {
		t.Errorf("MaxWriteTime = %f, want 50.0", m.MaxWriteTime)
	}
	if m.Topic != "test-topic" {
		t.Errorf("Topic = %q, want test-topic", m.Topic)
	}
}

func TestCollectReaderMetrics(t *testing.T) {
	stats := kafkago.ReaderStats{
		Dials:      10,
		Fetches:    200,
		Messages:   1000,
		Bytes:      2048,
		Errors:     3,
		Rebalances: 1,
		Offset:     999,
		Lag:        5,
		Topic:      "reader-topic",
		Partition:  "0",
	}

	m := CollectReaderMetrics(stats)
	if m.Dials != 10 {
		t.Errorf("Dials = %d, want 10", m.Dials)
	}
	if m.Fetches != 200 {
		t.Errorf("Fetches = %d, want 200", m.Fetches)
	}
	if m.Messages != 1000 {
		t.Errorf("Messages = %d, want 1000", m.Messages)
	}
	if m.Bytes != 2048 {
		t.Errorf("Bytes = %d, want 2048", m.Bytes)
	}
	if m.Errors != 3 {
		t.Errorf("Errors = %d, want 3", m.Errors)
	}
	if m.Rebalances != 1 {
		t.Errorf("Rebalances = %d, want 1", m.Rebalances)
	}
	if m.Offset != 999 {
		t.Errorf("Offset = %d, want 999", m.Offset)
	}
	if m.Lag != 5 {
		t.Errorf("Lag = %d, want 5", m.Lag)
	}
	if m.Topic != "reader-topic" {
		t.Errorf("Topic = %q, want reader-topic", m.Topic)
	}
	if m.Partition != "0" {
		t.Errorf("Partition = %q, want 0", m.Partition)
	}
}

func TestCollectWriterMetrics_Zero(t *testing.T) {
	m := CollectWriterMetrics(kafkago.WriterStats{})
	if m.Writes != 0 || m.Messages != 0 || m.Errors != 0 {
		t.Errorf("expected zero metrics, got %+v", m)
	}
}
