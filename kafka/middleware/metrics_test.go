package middleware

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/kbukum/gokit/kafka"
)

// setupTestMeter installs a test MeterProvider with a ManualReader.
func setupTestMeter(t *testing.T) *sdkmetric.ManualReader {
	t.Helper()
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	otel.SetMeterProvider(mp)
	t.Cleanup(func() {
		_ = mp.Shutdown(context.Background())
	})
	return reader
}

func collectMetrics(t *testing.T, reader *sdkmetric.ManualReader) map[string]metricdata.Metrics {
	t.Helper()
	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("collect metrics: %v", err)
	}
	result := make(map[string]metricdata.Metrics)
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			result[m.Name] = m
		}
	}
	return result
}

func TestInstrumentHandler_Success(t *testing.T) {
	reader := setupTestMeter(t)

	handler := func(_ context.Context, _ kafka.Message) error {
		time.Sleep(5 * time.Millisecond)
		return nil
	}

	wrapped := InstrumentHandler("orders", "worker-group", handler)
	msg := kafka.Message{Topic: "orders", Headers: map[string]string{}}

	if err := wrapped(context.Background(), msg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	metrics := collectMetrics(t, reader)

	// messages_total should be 1
	mt, ok := metrics["kafka_consumer_messages_total"]
	if !ok {
		t.Fatal("kafka_consumer_messages_total not found")
	}
	sumData, ok := mt.Data.(metricdata.Sum[int64])
	if !ok {
		t.Fatalf("expected Sum[int64], got %T", mt.Data)
	}
	if len(sumData.DataPoints) == 0 || sumData.DataPoints[0].Value != 1 {
		t.Errorf("messages_total = %v, want 1", sumData.DataPoints)
	}

	// errors_total should be absent or 0
	if et, ok := metrics["kafka_consumer_errors_total"]; ok {
		if sumData, ok := et.Data.(metricdata.Sum[int64]); ok {
			for _, dp := range sumData.DataPoints {
				if dp.Value != 0 {
					t.Errorf("errors_total = %d, want 0 on success", dp.Value)
				}
			}
		}
	}

	// duration should have been recorded
	dur, ok := metrics["kafka_consumer_processing_duration_seconds"]
	if !ok {
		t.Fatal("kafka_consumer_processing_duration_seconds not found")
	}
	histData, ok := dur.Data.(metricdata.Histogram[float64])
	if !ok {
		t.Fatalf("expected Histogram[float64], got %T", dur.Data)
	}
	if len(histData.DataPoints) == 0 || histData.DataPoints[0].Count == 0 {
		t.Error("expected at least one histogram data point")
	}
}

func TestInstrumentHandler_Error(t *testing.T) {
	reader := setupTestMeter(t)

	handler := func(_ context.Context, _ kafka.Message) error {
		return errors.New("process failed")
	}

	wrapped := InstrumentHandler("events", "group-a", handler)
	msg := kafka.Message{Topic: "events", Headers: map[string]string{}}

	err := wrapped(context.Background(), msg)
	if err == nil {
		t.Fatal("expected error from handler")
	}

	metrics := collectMetrics(t, reader)

	// errors_total should be 1
	et, ok := metrics["kafka_consumer_errors_total"]
	if !ok {
		t.Fatal("kafka_consumer_errors_total not found")
	}
	sumData, ok := et.Data.(metricdata.Sum[int64])
	if !ok {
		t.Fatalf("expected Sum[int64], got %T", et.Data)
	}
	if len(sumData.DataPoints) == 0 || sumData.DataPoints[0].Value != 1 {
		t.Errorf("errors_total = %v, want 1", sumData.DataPoints)
	}

	// messages_total should also be 1
	mt, ok := metrics["kafka_consumer_messages_total"]
	if !ok {
		t.Fatal("kafka_consumer_messages_total not found")
	}
	sumData2, ok := mt.Data.(metricdata.Sum[int64])
	if !ok {
		t.Fatalf("expected Sum[int64], got %T", mt.Data)
	}
	if len(sumData2.DataPoints) == 0 || sumData2.DataPoints[0].Value != 1 {
		t.Errorf("messages_total = %v, want 1", sumData2.DataPoints)
	}
}

func TestInstrumentHandler_PassesThrough(t *testing.T) {
	_ = setupTestMeter(t)

	var received kafka.Message
	handler := func(_ context.Context, msg kafka.Message) error {
		received = msg
		return nil
	}

	wrapped := InstrumentHandler("t", "g", handler)
	msg := kafka.Message{Topic: "t", Key: "k1", Value: []byte("v"), Headers: map[string]string{}}
	_ = wrapped(context.Background(), msg)

	if received.Key != "k1" || string(received.Value) != "v" {
		t.Errorf("handler received wrong message: %+v", received)
	}
}
