package provider_test

import (
	"context"
	"fmt"
	"io"
	"testing"

	"github.com/kbukum/gokit/provider"
)

// --- Test types ---

type echoProvider struct {
	name string
}

func (p *echoProvider) Name() string                       { return p.name }
func (p *echoProvider) IsAvailable(_ context.Context) bool { return true }

func (p *echoProvider) Execute(_ context.Context, in string) (string, error) {
	return "echo:" + in, nil
}

// Verify echoProvider satisfies RequestResponse
var _ provider.RequestResponse[string, string] = (*echoProvider)(nil)

// --- Stream provider ---

type sliceIterator[T any] struct {
	items []T
	pos   int
}

func (it *sliceIterator[T]) Next(_ context.Context) (val T, hasMore bool, err error) {
	if it.pos >= len(it.items) {
		var zero T
		return zero, false, nil
	}
	v := it.items[it.pos]
	it.pos++
	return v, true, nil
}

func (it *sliceIterator[T]) Close() error { return nil }

type splitProvider struct{}

func (p *splitProvider) Name() string                       { return "split" }
func (p *splitProvider) IsAvailable(_ context.Context) bool { return true }
func (p *splitProvider) Execute(_ context.Context, in string) (provider.Iterator[byte], error) {
	items := make([]byte, len(in))
	for i := range in {
		items[i] = in[i]
	}
	return &sliceIterator[byte]{items: items}, nil
}

var _ provider.Stream[string, byte] = (*splitProvider)(nil)

// --- Sink provider ---

type collectSink struct {
	collected []string
}

func (s *collectSink) Name() string                       { return "collect" }
func (s *collectSink) IsAvailable(_ context.Context) bool { return true }
func (s *collectSink) Send(_ context.Context, in string) error {
	s.collected = append(s.collected, in)
	return nil
}

var _ provider.Sink[string] = (*collectSink)(nil)

// --- Duplex provider ---

type echoDuplex struct{}

func (d *echoDuplex) Name() string                       { return "echo-duplex" }
func (d *echoDuplex) IsAvailable(_ context.Context) bool { return true }
func (d *echoDuplex) Open(_ context.Context) (provider.DuplexStream[string, string], error) {
	return &echoDuplexStream{ch: make(chan string, 10)}, nil
}

type echoDuplexStream struct {
	ch     chan string
	closed bool
}

func (s *echoDuplexStream) Send(in string) error {
	if s.closed {
		return fmt.Errorf("stream closed")
	}
	s.ch <- "echo:" + in
	return nil
}

func (s *echoDuplexStream) Recv() (string, error) {
	v, ok := <-s.ch
	if !ok {
		return "", io.EOF
	}
	return v, nil
}

func (s *echoDuplexStream) Close() error {
	s.closed = true
	close(s.ch)
	return nil
}

var _ provider.Duplex[string, string] = (*echoDuplex)(nil)

// --- Lifecycle providers ---

type initCloseProvider struct {
	name        string
	initialized bool
	closed      bool
}

func (p *initCloseProvider) Name() string                       { return p.name }
func (p *initCloseProvider) IsAvailable(_ context.Context) bool { return p.initialized && !p.closed }
func (p *initCloseProvider) Execute(_ context.Context, in string) (string, error) {
	return in, nil
}
func (p *initCloseProvider) Init(_ context.Context) error {
	p.initialized = true
	return nil
}
func (p *initCloseProvider) Close(_ context.Context) error {
	p.closed = true
	return nil
}

var _ provider.Initializable = (*initCloseProvider)(nil)
var _ provider.Closeable = (*initCloseProvider)(nil)

// --- Tests ---

func TestRequestResponse(t *testing.T) {
	p := &echoProvider{name: "test"}
	result, err := p.Execute(context.Background(), "hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "echo:hello" {
		t.Fatalf("expected echo:hello, got %s", result)
	}
}

func TestStream(t *testing.T) {
	p := &splitProvider{}
	iter, err := p.Execute(context.Background(), "abc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer iter.Close()

	var result []byte
	for {
		v, more, err := iter.Next(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !more {
			break
		}
		result = append(result, v)
	}
	if string(result) != "abc" {
		t.Fatalf("expected abc, got %s", string(result))
	}
}

func TestSink(t *testing.T) {
	s := &collectSink{}
	ctx := context.Background()

	if err := s.Send(ctx, "a"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := s.Send(ctx, "b"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(s.collected) != 2 || s.collected[0] != "a" || s.collected[1] != "b" {
		t.Fatalf("expected [a b], got %v", s.collected)
	}
}

func TestDuplex(t *testing.T) {
	d := &echoDuplex{}
	stream, err := d.Open(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := stream.Send("hello"); err != nil {
		t.Fatalf("send error: %v", err)
	}
	v, err := stream.Recv()
	if err != nil {
		t.Fatalf("recv error: %v", err)
	}
	if v != "echo:hello" {
		t.Fatalf("expected echo:hello, got %s", v)
	}

	stream.Close()
}

func TestManagerLifecycle(t *testing.T) {
	registry := provider.NewRegistry[provider.RequestResponse[string, string]]()
	selector := &provider.HealthCheckSelector[provider.RequestResponse[string, string]]{}
	mgr := provider.NewManager(registry, selector)

	p := &initCloseProvider{name: "test-lc"}
	registry.RegisterFactory("test-lc", func(_ map[string]any) (provider.RequestResponse[string, string], error) {
		return p, nil
	})

	// Initialize should call Init()
	if err := mgr.InitializeWithContext(context.Background(), "test-lc", nil); err != nil {
		t.Fatalf("initialize error: %v", err)
	}
	if !p.initialized {
		t.Fatal("expected Init() to be called")
	}

	// CloseAll should call Close()
	if err := mgr.CloseAll(context.Background()); err != nil {
		t.Fatalf("close error: %v", err)
	}
	if !p.closed {
		t.Fatal("expected Close() to be called")
	}
}
