package kafka

import (
	"errors"
	"testing"
)

func TestIsConnectionError(t *testing.T) {
	tests := []struct {
		err  error
		want bool
	}{
		{nil, false},
		{errors.New("something else"), false},
		{errors.New("connection refused"), true},
		{errors.New("Connection Reset by peer"), true},
		{errors.New("broken pipe"), true},
		{errors.New("i/o timeout"), true},
		{errors.New("no route to host"), true},
		{errors.New("network is unreachable"), true},
		{errors.New("broker not available"), true},
		{errors.New("leader not available"), true},
		{errors.New("connection closed"), true},
		{errors.New("dial tcp 127.0.0.1:9092"), true},
		{errors.New("network exception"), true},
	}
	for _, tt := range tests {
		name := "nil"
		if tt.err != nil {
			name = tt.err.Error()
		}
		t.Run(name, func(t *testing.T) {
			if got := IsConnectionError(tt.err); got != tt.want {
				t.Errorf("IsConnectionError(%q) = %v, want %v", name, got, tt.want)
			}
		})
	}
}

func TestIsRetryableError(t *testing.T) {
	tests := []struct {
		err  error
		want bool
	}{
		{nil, false},
		{errors.New("unknown error"), false},
		{errors.New("connection refused"), true}, // connection errors are retryable
		{errors.New("temporary failure"), true},
		{errors.New("request timed out"), true},
		{errors.New("not enough replicas"), true},
		{errors.New("offset out of range"), true},
	}
	for _, tt := range tests {
		name := "nil"
		if tt.err != nil {
			name = tt.err.Error()
		}
		t.Run(name, func(t *testing.T) {
			if got := IsRetryableError(tt.err); got != tt.want {
				t.Errorf("IsRetryableError(%q) = %v, want %v", name, got, tt.want)
			}
		})
	}
}

func TestIsNonRetryableError(t *testing.T) {
	tests := []struct {
		err  error
		want bool
	}{
		{nil, false},
		{errors.New("random error"), false},
		{errors.New("message too large"), true},
		{errors.New("invalid topic"), true},
		{errors.New("invalid partition"), true},
		{errors.New("unknown topic or partition"), true},
		{errors.New("authorization failed"), true},
	}
	for _, tt := range tests {
		name := "nil"
		if tt.err != nil {
			name = tt.err.Error()
		}
		t.Run(name, func(t *testing.T) {
			if got := IsNonRetryableError(tt.err); got != tt.want {
				t.Errorf("IsNonRetryableError(%q) = %v, want %v", name, got, tt.want)
			}
		})
	}
}
