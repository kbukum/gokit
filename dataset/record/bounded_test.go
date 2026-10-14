package record_test

import (
	"context"
	"errors"
	"testing"

	"github.com/kbukum/gokit/dataset/record"
	apperrors "github.com/kbukum/gokit/errors"
)

func TestBoundedJSONLines(t *testing.T) {
	t.Parallel()
	data := []byte("{\"id\":\"one\"}\n{\"id\":\"two\"}\n")
	for name, limits := range map[string]record.ParseLimits{
		"byte overflow": {MaxBytes: len(data) - 1, MaxRecords: 2},
		"case overflow": {MaxBytes: len(data), MaxRecords: 1},
		"invalid bytes": {MaxBytes: 0, MaxRecords: 2},
		"invalid cases": {MaxBytes: len(data), MaxRecords: 0},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			rows, err := record.ParseJSONLinesBounded(t.Context(), data, limits)
			var appErr *apperrors.AppError
			if rows != nil || !errors.As(err, &appErr) {
				t.Fatalf("limit must refuse without partial rows: %v", err)
			}
		})
	}
	rows, err := record.ParseJSONLinesBounded(t.Context(), data, record.ParseLimits{MaxBytes: len(data), MaxRecords: 2})
	if err != nil || len(rows) != 2 {
		t.Fatalf("exact limits: count=%d, error=%v", len(rows), err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if rows, err := record.ParseJSONLinesBounded(ctx, data, record.ParseLimits{MaxBytes: len(data), MaxRecords: 2}); rows != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled import: %v", err)
	}
}

func TestJSONLinesRejectsAmbiguousObjects(t *testing.T) {
	t.Parallel()
	for name, data := range map[string]string{
		"trailing object":   `{"id":"one"} {"id":"two"}`,
		"duplicate field":   `{"id":"one","id":"two"}`,
		"escaped duplicate": `{"id":"one","\u0069d":"two"}`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			rows, err := record.ParseJSONLinesBounded(t.Context(), []byte(data), record.ParseLimits{MaxBytes: 1024, MaxRecords: 2})
			if rows != nil || err == nil {
				t.Fatal("ambiguous object accepted")
			}
		})
	}
}
