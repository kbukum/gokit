package consumer

import (
	"context"
	"testing"
)

func TestConsumer_Name(t *testing.T) {
	c := &Consumer{groupID: "my-group", topic: "my-topic"}
	expected := "my-group:my-topic"
	if got := c.Name(); got != expected {
		t.Errorf("Name() = %q, want %q", got, expected)
	}
}

func TestConsumer_IsAvailable_Nil(t *testing.T) {
	c := &Consumer{}
	if c.IsAvailable(context.Background()) {
		t.Error("expected IsAvailable=false when reader is nil")
	}
}

func TestConsumer_Topic(t *testing.T) {
	c := &Consumer{topic: "events"}
	if c.Topic() != "events" {
		t.Errorf("Topic() = %q, want events", c.Topic())
	}
}

func TestConsumer_GroupID(t *testing.T) {
	c := &Consumer{groupID: "workers"}
	if c.GroupID() != "workers" {
		t.Errorf("GroupID() = %q, want workers", c.GroupID())
	}
}
