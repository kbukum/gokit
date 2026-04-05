package testutil

import (
	"context"
	"errors"
	"testing"

	"github.com/kbukum/gokit/messaging"
)

func TestMockProducer_SetError(t *testing.T) {
	t.Parallel()

	p := &MockProducer{}
	injected := errors.New("injected error")
	p.SetError(injected)

	ctx := context.Background()
	if err := p.PublishJSON(ctx, "t", "k", "v"); err != injected {
		t.Errorf("PublishJSON error = %v, want %v", err, injected)
	}
	if err := p.PublishBinary(ctx, "t", "k", nil); err != injected {
		t.Errorf("PublishBinary error = %v, want %v", err, injected)
	}
	if err := p.Publish(ctx, "t", messaging.Event{}, "k"); err != injected {
		t.Errorf("Publish error = %v, want %v", err, injected)
	}
	if err := p.Send(ctx, messaging.Message{}); err != injected {
		t.Errorf("Send error = %v, want %v", err, injected)
	}

	// Messages should NOT have been recorded
	if p.HasMessages() {
		t.Error("expected no messages when SetError is active")
	}
}

func TestMockProducer_SetError_Clear(t *testing.T) {
	t.Parallel()

	p := &MockProducer{}
	p.SetError(errors.New("fail"))
	p.SetError(nil)

	if err := p.PublishJSON(context.Background(), "t", "k", "v"); err != nil {
		t.Errorf("unexpected error after clearing: %v", err)
	}
	if !p.HasMessages() {
		t.Error("expected message to be recorded after clearing error")
	}
}

func TestMockProducer_HasMessages(t *testing.T) {
	t.Parallel()

	p := &MockProducer{}
	if p.HasMessages() {
		t.Error("expected no messages on new producer")
	}
	p.WriteMessage("t", nil, []byte("v"))
	if !p.HasMessages() {
		t.Error("expected HasMessages=true after write")
	}
}

func TestMockProducer_LastMessage(t *testing.T) {
	t.Parallel()

	p := &MockProducer{}
	p.WriteMessage("t1", nil, []byte("first"))
	p.WriteMessage("t2", nil, []byte("second"))

	last := p.LastMessage()
	if last.Topic != "t2" || string(last.Value) != "second" {
		t.Errorf("LastMessage() = %+v, want topic=t2 value=second", last)
	}
}

func TestMockProducer_LastMessage_Panic(t *testing.T) {
	t.Parallel()

	p := &MockProducer{}
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on empty producer")
		}
	}()
	p.LastMessage()
}

func TestMockProducer_MessageCount(t *testing.T) {
	t.Parallel()

	p := &MockProducer{}
	if p.MessageCount() != 0 {
		t.Errorf("MessageCount() = %d, want 0", p.MessageCount())
	}
	p.WriteMessage("t", nil, []byte("1"))
	p.WriteMessage("t", nil, []byte("2"))
	if p.MessageCount() != 2 {
		t.Errorf("MessageCount() = %d, want 2", p.MessageCount())
	}
}
