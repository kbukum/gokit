package logging

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestLoggerReleaseClosesOwnedSinksOnce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.log")
	l, err := New(&Config{Level: "info", Format: "json", Output: OutputFile(path)}, "svc")
	if err != nil {
		t.Fatal(err)
	}
	l.Info("before release")
	if err := l.Shutdown(t.Context()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if err := l.Close(); err != nil {
		t.Fatalf("repeated release must return the recorded outcome, got %v", err)
	}
	l.Info("after release")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "before release") || strings.Contains(string(data), "after release") {
		t.Fatalf("released sink content = %q", data)
	}
}

func TestDerivedLoggerDoesNotReleaseRootSinks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.log")
	l, err := New(&Config{Level: "info", Format: "json", Output: OutputFile(path)}, "svc")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() })
	if err := l.WithComponent("child").Close(); err != nil {
		t.Fatal(err)
	}
	l.Info("root still writes")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "root still writes") {
		t.Fatalf("derived release closed the root sink: %q", data)
	}
}

func TestLoggerShutdownHonorsCallerDeadline(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	accepted := make(chan net.Conn, 4)
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				close(accepted)
				return
			}
			accepted <- conn // hold the connection open without responding
		}
	}()
	t.Cleanup(func() {
		_ = listener.Close()
		for conn := range accepted {
			_ = conn.Close()
		}
	})
	l, err := New(&Config{
		Level: "info", Format: "json", Output: OutputStderr(),
		OTLP: OTLPConfig{Enabled: true, Protocol: "http", Insecure: true, Endpoint: listener.Addr().String()},
	}, "svc")
	if err != nil {
		t.Fatal(err)
	}
	l.Info("pending export")
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	err = l.Shutdown(ctx)
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("Shutdown exceeded the caller deadline: %v", elapsed)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Shutdown error = %v, want deadline exceeded", err)
	}
	if again := l.Close(); !errors.Is(again, context.DeadlineExceeded) {
		t.Fatalf("repeated release = %v, want recorded deadline error", again)
	}
}

type shutdownSignalExporter struct {
	memExporter
	shutdowns atomic.Int32
	stopped   chan struct{}
}

func (e *shutdownSignalExporter) Shutdown(context.Context) error {
	if e.shutdowns.Add(1) == 1 {
		close(e.stopped)
	}
	return nil
}

func TestLoggerShutdownWithExpiredContextStillStopsExporter(t *testing.T) {
	exp := &shutdownSignalExporter{stopped: make(chan struct{})}
	log := &Logger{release: &release{otlp: newOTLPProvider(exp, nil)}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := log.Shutdown(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Shutdown error = %v, want context.Canceled", err)
	}
	select {
	case <-exp.stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("exporter was not stopped after shutdown with an ended context")
	}
	if err := log.Close(); !errors.Is(err, context.Canceled) {
		t.Fatalf("second release = %v, want recorded outcome", err)
	}
	if got := exp.shutdowns.Load(); got != 1 {
		t.Fatalf("exporter shutdowns = %d, want 1", got)
	}
}
