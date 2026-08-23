package messaging

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/kbukum/gokit/logging"
)

type recordingProducer struct {
	mu       sync.Mutex
	sent     []Message
	batches  int
	flushed  int
	closed   bool
	sendErr  error
	closeErr error
}

func (p *recordingProducer) Send(_ context.Context, msg Message) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.sendErr != nil {
		return p.sendErr
	}
	p.sent = append(p.sent, msg)
	return nil
}

func (p *recordingProducer) SendBatch(_ context.Context, msgs []Message) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.batches++
	p.sent = append(p.sent, msgs...)
	return nil
}

func (p *recordingProducer) Publish(context.Context, string, Event, ...string) error { return nil }
func (p *recordingProducer) PublishJSON(context.Context, string, string, any) error  { return nil }
func (p *recordingProducer) PublishBinary(context.Context, string, string, []byte) error {
	return nil
}

func (p *recordingProducer) Flush(context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.flushed++
	return nil
}

func (p *recordingProducer) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closed = true
	return p.closeErr
}

func (p *recordingProducer) isClosed() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.closed
}

func (p *recordingProducer) sentCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.sent)
}

type recordingMetrics struct {
	mu        sync.Mutex
	publishes int
	lastErr   error
}

func (m *recordingMetrics) RecordPublish(_ string, _ time.Duration, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.publishes++
	m.lastErr = err
}

func (m *recordingMetrics) RecordConsume(string, time.Duration, error) {}

func (m *recordingMetrics) count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.publishes
}

func newTestManagedProducer(p Producer, metrics MetricsCollector) *ManagedProducer {
	return NewManagedProducer(ManagedProducerConfig{
		Producer: p,
		Name:     "test",
		Log:      logging.NewDefault("test"),
		Metrics:  metrics,
	})
}

func TestManagedProducer_StartStopIsRunning(t *testing.T) {
	t.Parallel()

	p := &recordingProducer{}
	mp := newTestManagedProducer(p, nil)
	ctx := context.Background()

	if mp.IsRunning() {
		t.Fatal("producer running before Start")
	}
	if err := mp.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if !mp.IsRunning() {
		t.Fatal("producer not running after Start")
	}
	if err := mp.Stop(ctx); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if mp.IsRunning() {
		t.Fatal("producer running after Stop")
	}
	if !p.isClosed() {
		t.Fatal("Stop did not close the underlying producer")
	}
}

func TestManagedProducer_DoubleStartIsNoop(t *testing.T) {
	t.Parallel()

	mp := newTestManagedProducer(&recordingProducer{}, nil)
	ctx := context.Background()
	if err := mp.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := mp.Start(ctx); err != nil {
		t.Fatalf("second Start: %v", err)
	}
	if !mp.IsRunning() {
		t.Fatal("producer not running")
	}
}

func TestManagedProducer_StopWhenNotRunningIsNoop(t *testing.T) {
	t.Parallel()

	mp := newTestManagedProducer(&recordingProducer{}, nil)
	if err := mp.Stop(context.Background()); err != nil {
		t.Fatalf("Stop when not running: %v", err)
	}
}

func TestManagedProducer_SendBeforeStartErrors(t *testing.T) {
	t.Parallel()

	p := &recordingProducer{}
	mp := newTestManagedProducer(p, nil)
	err := mp.Send(context.Background(), Message{Topic: "t", Value: []byte("v")})
	if err == nil {
		t.Fatal("Send before Start did not error")
	}
	if p.sentCount() != 0 {
		t.Fatal("Send delegated to producer before Start")
	}
}

func TestManagedProducer_SendAfterStartDelegatesAndRecordsMetrics(t *testing.T) {
	t.Parallel()

	p := &recordingProducer{}
	metrics := &recordingMetrics{}
	mp := newTestManagedProducer(p, metrics)
	ctx := context.Background()
	if err := mp.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := mp.Send(ctx, Message{Topic: "t", Value: []byte("v")}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if p.sentCount() != 1 {
		t.Fatalf("producer received %d messages, want 1", p.sentCount())
	}
	if metrics.count() != 1 {
		t.Fatalf("metrics recorded %d publishes, want 1", metrics.count())
	}
}

func TestManagedProducer_SendErrorIsRecorded(t *testing.T) {
	t.Parallel()

	p := &recordingProducer{sendErr: errors.New("boom")}
	metrics := &recordingMetrics{}
	mp := newTestManagedProducer(p, metrics)
	ctx := context.Background()
	if err := mp.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := mp.Send(ctx, Message{Topic: "t"}); err == nil {
		t.Fatal("Send did not propagate producer error")
	}
	if metrics.lastErr == nil {
		t.Fatal("metrics did not record the send error")
	}
}
