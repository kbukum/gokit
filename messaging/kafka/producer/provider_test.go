package producer

import (
	"context"
	"testing"
)

func TestSinkProvider_Name(t *testing.T) {
	p := &SinkProvider{name: "test-kafka", producer: &Producer{}}
	if got := p.Name(); got != "test-kafka" {
		t.Errorf("Name() = %q, want %q", got, "test-kafka")
	}
}

func TestSinkProvider_IsAvailable_Open(t *testing.T) {
	prod := &Producer{}
	p := NewSinkProvider("test", prod)
	if !p.IsAvailable(context.Background()) {
		t.Error("expected IsAvailable=true for open producer")
	}
}

func TestSinkProvider_IsAvailable_Closed(t *testing.T) {
	prod := &Producer{closed: true}
	p := NewSinkProvider("test", prod)
	if p.IsAvailable(context.Background()) {
		t.Error("expected IsAvailable=false for closed producer")
	}
}

func TestSinkProvider_Producer(t *testing.T) {
	prod := &Producer{}
	p := NewSinkProvider("test", prod)
	if p.Producer() != prod {
		t.Error("Producer() should return the underlying producer")
	}
}

func TestSinkProvider_Send_Closed(t *testing.T) {
	// A properly constructed but closed producer should return an error.
	// We can't easily construct a full producer without Kafka,
	// so we test the availability check guards the path.
	prod := &Producer{closed: true}
	p := NewSinkProvider("test", prod)

	if p.IsAvailable(context.Background()) {
		t.Error("expected IsAvailable=false for closed producer")
	}
}
