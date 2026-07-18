package record

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"slices"

	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/stream"
)

// WriteCSV drains p and writes its records as CSV to w, returning the number of records written. The header is taken from the first record's sorted keys; every later record must have exactly the same keys or writing fails closed.
func WriteCSV(ctx context.Context, w io.Writer, p *stream.Pipeline[Record]) (int, error) {
	cw := csv.NewWriter(w)
	var header []string
	count := 0
	err := stream.ForEach(ctx, p, func(_ context.Context, rec Record) error {
		keys := rec.Keys()
		if header == nil {
			header = keys
			if err := cw.Write(header); err != nil {
				return apperrors.Internal(err)
			}
		} else if !slices.Equal(header, keys) {
			return apperrors.InvalidInput("csv", fmt.Sprintf("record columns %v do not match header %v", keys, header))
		}
		row := make([]string, len(header))
		for i, k := range header {
			v, _ := rec.Get(k)
			cell, err := valueToCell(v)
			if err != nil {
				return err
			}
			row[i] = cell
		}
		if err := cw.Write(row); err != nil {
			return apperrors.Internal(err)
		}
		count++
		return nil
	})
	if err != nil {
		return count, err
	}
	cw.Flush()
	if err := cw.Error(); err != nil {
		return count, apperrors.Internal(err)
	}
	return count, nil
}

// WriteJSONArray drains p and writes its records as a single JSON array to w, returning the number of records written.
func WriteJSONArray(ctx context.Context, w io.Writer, p *stream.Pipeline[Record]) (int, error) {
	records, err := stream.Collect(ctx, p)
	if err != nil {
		return 0, err
	}
	objects := make([]map[string]Value, len(records))
	for i, rec := range records {
		objects[i] = rec.ToJSON()
	}
	data, err := json.Marshal(objects)
	if err != nil {
		return 0, apperrors.Internal(err)
	}
	if _, err := w.Write(data); err != nil {
		return 0, apperrors.Internal(err)
	}
	return len(records), nil
}

// WriteJSONLines drains p and writes each record as one JSON object per line to w, returning the number of records written.
func WriteJSONLines(ctx context.Context, w io.Writer, p *stream.Pipeline[Record]) (int, error) {
	count := 0
	err := stream.ForEach(ctx, p, func(_ context.Context, rec Record) error {
		data, err := json.Marshal(rec.ToJSON())
		if err != nil {
			return apperrors.Internal(err)
		}
		data = append(data, '\n')
		if _, err := w.Write(data); err != nil {
			return apperrors.Internal(err)
		}
		count++
		return nil
	})
	return count, err
}

// valueToCell renders a field value as a CSV cell: scalars use their natural text form; composite values are JSON-encoded.
func valueToCell(v Value) (string, error) {
	switch t := v.(type) {
	case nil:
		return "", nil
	case string:
		return t, nil
	case bool:
		return fmt.Sprintf("%t", t), nil
	case float64:
		return fmt.Sprintf("%v", t), nil
	case json.Number:
		return t.String(), nil
	default:
		data, err := json.Marshal(v)
		if err != nil {
			return "", apperrors.Internal(err)
		}
		return string(data), nil
	}
}
