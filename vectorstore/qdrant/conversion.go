package qdrant

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"

	"github.com/google/uuid"

	"github.com/kbukum/gokit/vectorstore"
)

type pointID struct {
	num  *uint64
	uuid string
}

func (id pointID) MarshalJSON() ([]byte, error) {
	if id.num != nil {
		return []byte(strconv.FormatUint(*id.num, 10)), nil
	}
	return json.Marshal(id.uuid)
}

func pointIDFromString(id string) (pointID, error) {
	if isCanonicalNumericID(id) {
		value, err := strconv.ParseUint(id, 10, 64)
		if err != nil {
			return pointID{}, fmt.Errorf("qdrant: numeric point id %q is outside uint64 bounds: %w", id, err)
		}
		return pointID{num: &value}, nil
	}
	parsed, err := uuid.Parse(id)
	if err != nil {
		return pointID{}, fmt.Errorf("qdrant: point id %q must be numeric or a valid UUID: %w", id, err)
	}
	return pointID{uuid: parsed.String()}, nil
}

func pointIDToString(value json.RawMessage) (string, error) {
	var n uint64
	if err := json.Unmarshal(value, &n); err == nil {
		return strconv.FormatUint(n, 10), nil
	}
	var s string
	if err := json.Unmarshal(value, &s); err == nil {
		return s, nil
	}
	return "", fmt.Errorf("qdrant: unsupported point id %s", string(value))
}

func isCanonicalNumericID(id string) bool {
	if id == "" || (len(id) > 1 && id[0] == '0') {
		return false
	}
	for _, ch := range id {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return true
}

func qdrantDistance(metric string) (string, error) {
	switch metric {
	case vectorstore.MetricCosine:
		return "Cosine", nil
	case vectorstore.MetricDot:
		return "Dot", nil
	case vectorstore.MetricL2:
		return "Euclid", nil
	default:
		return "", &vectorstore.MetricError{Metric: metric}
	}
}

func payloadToJSON(payload *vectorstore.PointPayload) (map[string]json.RawMessage, error) {
	out := map[string]json.RawMessage{}
	if payload == nil {
		return out, nil
	}
	for key, value := range payload.Fields {
		b, err := supportedValueJSON(value)
		if err != nil {
			return nil, fmt.Errorf("qdrant: payload field %q: %w", key, err)
		}
		out[key] = b
	}
	return out, nil
}

func supportedValueJSON(value any) (json.RawMessage, error) {
	switch v := value.(type) {
	case string, bool, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return json.Marshal(v)
	case float32:
		if math.IsInf(float64(v), 0) || math.IsNaN(float64(v)) {
			return nil, errorsNonFiniteFloat()
		}
		return json.Marshal(v)
	case float64:
		if math.IsInf(v, 0) || math.IsNaN(v) {
			return nil, errorsNonFiniteFloat()
		}
		return json.Marshal(v)
	default:
		return nil, fmt.Errorf("unsupported value type %T", value)
	}
}

func errorsNonFiniteFloat() error { return fmt.Errorf("float values must be finite") }

func filterToJSON(filter *vectorstore.SearchFilter) (map[string][]map[string]json.RawMessage, error) {
	if filter == nil || len(filter.Must) == 0 {
		return nil, nil //nolint:nilnil // (nil, nil) is the documented "no filter" signal consumed by Search.
	}
	must := make([]map[string]json.RawMessage, 0, len(filter.Must))
	for _, condition := range filter.Must {
		value, err := supportedValueJSON(condition.Value)
		if err != nil {
			return nil, fmt.Errorf("qdrant: filter field %q: %w", condition.Field, err)
		}
		match, err := json.Marshal(map[string]json.RawMessage{"value": value})
		if err != nil {
			return nil, err
		}
		key, err := json.Marshal(condition.Field)
		if err != nil {
			return nil, err
		}
		must = append(must, map[string]json.RawMessage{"key": key, "match": match})
	}
	return map[string][]map[string]json.RawMessage{"must": must}, nil
}

func payloadFromJSON(fields map[string]json.RawMessage) (*vectorstore.PointPayload, error) {
	payload := vectorstore.NewPointPayload()
	for field, raw := range fields {
		var v any
		if err := json.Unmarshal(raw, &v); err != nil {
			return nil, fmt.Errorf("qdrant: decode payload field %q: %w", field, err)
		}
		switch value := v.(type) {
		case string, bool, float64:
			payload.WithField(field, value)
		default:
			return nil, fmt.Errorf("qdrant: unsupported returned payload field %q", field)
		}
	}
	return payload, nil
}
