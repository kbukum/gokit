package record

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/kbukum/gokit/dataset/payload"
	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/fs"
	"github.com/kbukum/gokit/stream"
)

// ReadCSV reads a CSV file (bounded by limits) and returns a pipeline of its records.
// The first row is the header; each later row must have the same number of columns
// or reading fails closed.
func ReadCSV(path string, limits payload.Limits) (*stream.Pipeline[Record], error) {
	return readFile(path, limits, ParseCSV)
}

// ReadJSONArray reads a JSON array file (bounded by limits) whose elements are objects
// and returns a pipeline of its records.
func ReadJSONArray(path string, limits payload.Limits) (*stream.Pipeline[Record], error) {
	return readFile(path, limits, ParseJSONArray)
}

// ReadJSONLines reads a newline-delimited JSON file (bounded by limits) whose non-empty lines are objects
// and returns a pipeline of its records.
func ReadJSONLines(path string, limits payload.Limits) (*stream.Pipeline[Record], error) {
	return readFile(path, limits, ParseJSONLines)
}

// readFile performs the bounded read then applies parse,
// surfacing any parse error to the caller rather than mid-stream.
func readFile(path string, limits payload.Limits, parse func([]byte) ([]Record, error)) (*stream.Pipeline[Record], error) {
	limits = limits.WithDefaults()
	data, err := fs.ReadFileLimit(path, limits.MaxInMemoryBytes)
	if err != nil {
		return nil, err
	}
	records, err := parse(data)
	if err != nil {
		return nil, err
	}
	return stream.FromSlice(records), nil
}

// ParseCSV parses CSV bytes into records, treating the first row as the header.
// It fails closed on malformed rows and never panics on untrusted input.
func ParseCSV(data []byte) ([]Record, error) {
	r := csv.NewReader(bytes.NewReader(data))
	header, err := r.Read()
	if err != nil {
		if err == io.EOF {
			return []Record{}, nil
		}
		return nil, apperrors.InvalidInput("csv", "failed to read header row").WithCause(err)
	}
	var records []Record
	for {
		row, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, apperrors.InvalidInput("csv", "failed to read data row").WithCause(err)
		}
		fields := make(map[string]Value, len(header))
		for i, col := range header {
			fields[col] = row[i]
		}
		records = append(records, Record{fields: fields})
	}
	return records, nil
}

// ParseJSONArray parses a JSON array of objects into records.
// It fails closed when the payload is not an array of objects.
func ParseJSONArray(data []byte) ([]Record, error) {
	var raw []json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, apperrors.InvalidInput("json_array", "payload is not a JSON array").WithCause(err)
	}
	records := make([]Record, 0, len(raw))
	for i, elem := range raw {
		rec, err := decodeObject(elem)
		if err != nil {
			return nil, apperrors.InvalidInput("json_array", fmt.Sprintf("element %d is not an object", i)).WithCause(err)
		}
		records = append(records, rec)
	}
	return records, nil
}

// ParseJSONLines parses newline-delimited JSON objects into records, skipping blank lines.
// It fails closed on any malformed or non-object line.
func ParseJSONLines(data []byte) ([]Record, error) {
	var records []Record
	for i, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		rec, err := decodeObject([]byte(trimmed))
		if err != nil {
			return nil, apperrors.InvalidInput("json_lines", fmt.Sprintf("line %d is not an object", i+1)).WithCause(err)
		}
		records = append(records, rec)
	}
	return records, nil
}

// decodeObject decodes a single JSON object into a Record, rejecting any non-object shape.
func decodeObject(data []byte) (Record, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	token, err := dec.Token()
	if err != nil {
		return Record{}, err
	}
	if token != json.Delim('{') {
		return Record{}, apperrors.InvalidInput("json", "expected a JSON object")
	}
	fields := make(map[string]Value)
	for dec.More() {
		token, err := dec.Token()
		if err != nil {
			return Record{}, err
		}
		name, ok := token.(string)
		if !ok {
			return Record{}, apperrors.InvalidInput("json", "expected an object field")
		}
		if _, exists := fields[name]; exists {
			return Record{}, apperrors.InvalidInput("json", "duplicate object field")
		}
		var value Value
		if err := dec.Decode(&value); err != nil {
			return Record{}, err
		}
		fields[name] = value
	}
	if _, err := dec.Token(); err != nil {
		return Record{}, err
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		if err != nil {
			return Record{}, err
		}
		return Record{}, apperrors.InvalidInput("json", "trailing object data")
	}
	return Record{fields: fields}, nil
}
