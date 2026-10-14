package record

import (
	"bytes"
	"context"
	"fmt"

	apperrors "github.com/kbukum/gokit/errors"
)

// ParseLimits bounds an in-memory JSONL import before retaining records.
type ParseLimits struct {
	MaxBytes   int
	MaxRecords int
}

// ParseJSONLinesBounded decodes object records within explicit byte/count limits and caller cancellation.
func ParseJSONLinesBounded(ctx context.Context, data []byte, limits ParseLimits) ([]Record, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if limits.MaxBytes < 1 || limits.MaxRecords < 1 {
		return nil, apperrors.InvalidInput("limits", "Positive byte and record limits are required")
	}
	if len(data) > limits.MaxBytes {
		return nil, apperrors.InvalidInput("json_lines", "Payload exceeds byte limit")
	}
	var rows []Record
	for line := range bytes.SplitSeq(data, []byte{'\n'}) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		if len(rows) == limits.MaxRecords {
			return nil, apperrors.InvalidInput("json_lines", "Payload exceeds record limit")
		}
		row, err := decodeObject(line)
		if err != nil {
			return nil, apperrors.InvalidInput("json_lines", fmt.Sprintf("Record %d is not a valid object", len(rows)+1)).WithCause(err)
		}
		rows = append(rows, row)
	}
	return rows, nil
}
