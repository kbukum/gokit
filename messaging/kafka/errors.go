package kafka

import (
	"strings"

	"github.com/kbukum/gokit/messaging"
)

// KafkaErrorClassifier implements messaging.ErrorClassifier with Kafka-specific error patterns.
type KafkaErrorClassifier struct{}

var _ messaging.ErrorClassifier = KafkaErrorClassifier{}

func (KafkaErrorClassifier) IsConnectionError(err error) bool { return IsConnectionError(err) }
func (KafkaErrorClassifier) IsRetryableError(err error) bool  { return IsRetryableError(err) }

// kafkaConnectionPatterns are Kafka-specific connection error patterns that supplement the generic messaging.ConnectionPatterns.
var kafkaConnectionPatterns = []string{
	"broker not available",
	"leader not available",
	"network exception",
}

// kafkaRetryablePatterns are Kafka-specific retryable patterns that supplement the generic messaging.RetryablePatterns.
var kafkaRetryablePatterns = []string{
	"not enough replicas",
	"offset out of range",
}

// IsConnectionError checks if a Kafka error is a connection-level error.
// It checks both generic patterns and Kafka-specific patterns.
func IsConnectionError(err error) bool {
	return messaging.IsConnectionError(err, kafkaConnectionPatterns...)
}

// IsRetryableError determines if a Kafka error should trigger a retry.
// It checks both generic patterns and Kafka-specific patterns.
func IsRetryableError(err error) bool {
	if err == nil {
		return false
	}
	if IsConnectionError(err) {
		return true
	}
	return messaging.IsRetryableError(err, kafkaRetryablePatterns...)
}

// IsNonRetryableError checks if the error should not be retried.
func IsNonRetryableError(err error) bool {
	if err == nil {
		return false
	}
	errStr := strings.ToLower(err.Error())
	nonRetryablePatterns := []string{
		"message too large",
		"invalid topic",
		"invalid partition",
		"unknown topic",
		"authorization failed",
	}
	for _, p := range nonRetryablePatterns {
		if strings.Contains(errStr, p) {
			return true
		}
	}
	return false
}
