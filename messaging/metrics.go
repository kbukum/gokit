package messaging

import "time"

// MetricsCollector records messaging operational metrics. Each broker implementation provides its own metrics collection.
type MetricsCollector interface {
	// RecordPublish records a publish operation's outcome.
	RecordPublish(topic string, duration time.Duration, err error)
	// RecordConsume records a consume operation's outcome.
	RecordConsume(topic string, duration time.Duration, err error)
}
