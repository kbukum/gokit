package messaging

import (
	"errors"
	"testing"
)

func TestIsConnectionError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		err   error
		extra []string
		want  bool
	}{
		{"nil error", nil, nil, false},
		{"unrelated error", errors.New("something else"), nil, false},
		{"connection refused", errors.New("connection refused"), nil, true},
		{"connection reset", errors.New("Connection Reset by peer"), nil, true},
		{"broken pipe", errors.New("broken pipe"), nil, true},
		{"i/o timeout", errors.New("i/o timeout"), nil, true},
		{"no route to host", errors.New("no route to host"), nil, true},
		{"network unreachable", errors.New("network is unreachable"), nil, true},
		{"connection closed", errors.New("connection closed"), nil, true},
		{"dial tcp", errors.New("dial tcp 127.0.0.1:9092"), nil, true},
		{"extra pattern match", errors.New("broker not available"), []string{"broker not available"}, true},
		{"extra pattern no match", errors.New("something else"), []string{"broker not available"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := IsConnectionError(tt.err, tt.extra...); got != tt.want {
				t.Errorf("IsConnectionError() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsRetryableError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		err   error
		extra []string
		want  bool
	}{
		{"nil error", nil, nil, false},
		{"unrelated error", errors.New("unknown error"), nil, false},
		{"connection error is retryable", errors.New("connection refused"), nil, true},
		{"temporary", errors.New("temporary failure"), nil, true},
		{"request timed out", errors.New("request timed out"), nil, true},
		{"extra pattern match", errors.New("not enough replicas"), []string{"not enough replicas"}, true},
		{"extra pattern no match", errors.New("random error"), []string{"not enough replicas"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := IsRetryableError(tt.err, tt.extra...); got != tt.want {
				t.Errorf("IsRetryableError() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestConnectionPatterns_NotEmpty(t *testing.T) {
	t.Parallel()
	if len(ConnectionPatterns) == 0 {
		t.Error("ConnectionPatterns should not be empty")
	}
}

func TestRetryablePatterns_NotEmpty(t *testing.T) {
	t.Parallel()
	if len(RetryablePatterns) == 0 {
		t.Error("RetryablePatterns should not be empty")
	}
}
