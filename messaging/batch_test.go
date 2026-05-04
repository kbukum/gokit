package messaging

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// stubProducer is a minimal Producer for batch tests.
type stubProducer struct {
	mu       sync.Mutex
	messages []Message
	err      error
}

func (p *stubProducer) Publish(_ context.Context, _ string, _ Event, _ ...string) error {
	return p.err
}

func (p *stubProducer) PublishJSON(_ context.Context, _, _ string, _ any) error {
	return p.err
}

func (p *stubProducer) PublishBinary(_ context.Context, topic, key string, data []byte) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.err != nil {
		return p.err
	}
	p.messages = append(p.messages, Message{Topic: topic, Key: key, Value: data})
	return nil
}
func (p *stubProducer) Send(ctx context.Context, msg Message) error {
	return p.PublishBinary(ctx, msg.Topic, msg.Key, msg.Value)
}
func (p *stubProducer) SendBatch(ctx context.Context, messages []Message) error {
	for _, msg := range messages {
		if err := p.Send(ctx, msg); err != nil {
			return err
		}
	}
	return nil
}
func (p *stubProducer) Flush(context.Context) error { return nil }
func (p *stubProducer) Close() error                { return nil }
func (p *stubProducer) count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.messages)
}

func TestBatchProducer_SizeTriggeredFlush(t *testing.T) {
	t.Parallel()

	sp := &stubProducer{}
	bp := NewBatchProducer(sp, "topic", BatchConfig{
		MaxSize: 3,
		MaxWait: time.Hour, // disable time-based flush
	})
	defer bp.Close(context.Background())

	ctx := context.Background()
	for i := 0; i < 3; i++ {
		if err := bp.Send(ctx, Message{Key: "k", Value: []byte("v")}); err != nil {
			t.Fatal(err)
		}
	}

	if got := sp.count(); got != 3 {
		t.Errorf("count = %d, want 3", got)
	}
}

func TestBatchProducer_TimeTriggeredFlush(t *testing.T) {
	t.Parallel()

	sp := &stubProducer{}
	bp := NewBatchProducer(sp, "topic", BatchConfig{
		MaxSize: 100,
		MaxWait: 50 * time.Millisecond,
	})
	defer bp.Close(context.Background())

	ctx := context.Background()
	if err := bp.Send(ctx, Message{Key: "k", Value: []byte("v")}); err != nil {
		t.Fatal(err)
	}

	// Wait for the timer to fire.
	time.Sleep(200 * time.Millisecond)

	if got := sp.count(); got != 1 {
		t.Errorf("count = %d, want 1 (time-triggered flush)", got)
	}
}

func TestBatchProducer_ByteTriggeredFlush(t *testing.T) {
	t.Parallel()

	sp := &stubProducer{}
	bp := NewBatchProducer(sp, "topic", BatchConfig{
		MaxSize:  100,
		MaxWait:  time.Hour,
		MaxBytes: 10,
	})
	defer bp.Close(context.Background())

	ctx := context.Background()
	// 6 bytes each → second message should trigger flush at 12 bytes ≥ 10.
	for i := 0; i < 2; i++ {
		if err := bp.Send(ctx, Message{Key: "k", Value: []byte("123456")}); err != nil {
			t.Fatal(err)
		}
	}

	if got := sp.count(); got != 2 {
		t.Errorf("count = %d, want 2", got)
	}
}

func TestBatchProducer_CloseFlushesRemaining(t *testing.T) {
	t.Parallel()

	sp := &stubProducer{}
	bp := NewBatchProducer(sp, "topic", BatchConfig{
		MaxSize: 100,
		MaxWait: time.Hour,
	})

	ctx := context.Background()
	for i := 0; i < 5; i++ {
		if err := bp.Send(ctx, Message{Key: "k", Value: []byte("v")}); err != nil {
			t.Fatal(err)
		}
	}

	if got := sp.count(); got != 0 {
		t.Errorf("count before close = %d, want 0", got)
	}

	if err := bp.Close(ctx); err != nil {
		t.Fatal(err)
	}

	if got := sp.count(); got != 5 {
		t.Errorf("count after close = %d, want 5", got)
	}
}

func TestBatchProducer_FlushExplicit(t *testing.T) {
	t.Parallel()

	sp := &stubProducer{}
	bp := NewBatchProducer(sp, "topic", BatchConfig{
		MaxSize: 100,
		MaxWait: time.Hour,
	})
	defer bp.Close(context.Background())

	ctx := context.Background()
	_ = bp.Send(ctx, Message{Key: "k", Value: []byte("v")})

	if err := bp.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	if got := sp.count(); got != 1 {
		t.Errorf("count = %d, want 1", got)
	}
}

func TestBatchProducer_ConcurrentSend(t *testing.T) {
	t.Parallel()

	sp := &stubProducer{}
	bp := NewBatchProducer(sp, "topic", BatchConfig{
		MaxSize: 10,
		MaxWait: time.Hour,
	})

	const n = 100
	var wg sync.WaitGroup
	var sendErr atomic.Value
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			if err := bp.Send(context.Background(), Message{Key: "k", Value: []byte("v")}); err != nil {
				sendErr.Store(err)
			}
		}()
	}
	wg.Wait()

	if v := sendErr.Load(); v != nil {
		t.Fatalf("concurrent send error: %v", v)
	}

	if err := bp.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := sp.count(); got != n {
		t.Errorf("count = %d, want %d", got, n)
	}
}

func TestBatchProducer_SendAfterCloseReturnsError(t *testing.T) {
	t.Parallel()

	sp := &stubProducer{}
	bp := NewBatchProducer(sp, "topic", BatchConfig{MaxSize: 10, MaxWait: time.Hour})
	_ = bp.Close(context.Background())

	err := bp.Send(context.Background(), Message{Key: "k", Value: []byte("v")})
	if err == nil {
		t.Error("expected error after Close, got nil")
	}
}

func TestBatchProducer_ProducerErrorPropagated(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("publish failed")
	sp := &stubProducer{err: sentinel}
	bp := NewBatchProducer(sp, "topic", BatchConfig{MaxSize: 1, MaxWait: time.Hour})
	defer bp.Close(context.Background())

	err := bp.Send(context.Background(), Message{Key: "k", Value: []byte("v")})
	if !errors.Is(err, sentinel) {
		t.Errorf("error = %v, want %v", err, sentinel)
	}
}
