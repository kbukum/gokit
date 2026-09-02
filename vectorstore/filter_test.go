package vectorstore

import (
	"encoding/json"
	"testing"
)

func TestFilterConditionJSONShape(t *testing.T) {
	t.Parallel()

	cond := FilterCondition{Field: "status", Value: "active"}
	data, err := json.Marshal(cond)
	if err != nil {
		t.Fatalf("marshal FilterCondition: %v", err)
	}

	const want = `{"field":"status","equals":"active"}`
	if got := string(data); got != want {
		t.Fatalf("FilterCondition JSON = %s, want %s", got, want)
	}
}

func TestSearchFilterJSONShape(t *testing.T) {
	t.Parallel()

	f := NewSearchFilter().MustMatch("status", "active")
	data, err := json.Marshal(f)
	if err != nil {
		t.Fatalf("marshal SearchFilter: %v", err)
	}

	const want = `{"must":[{"field":"status","equals":"active"}]}`
	if got := string(data); got != want {
		t.Fatalf("SearchFilter JSON = %s, want %s", got, want)
	}
}

func TestFilterConditionJSONRoundTrip(t *testing.T) {
	t.Parallel()

	const wire = `{"field":"score","equals":42}`
	var cond FilterCondition
	if err := json.Unmarshal([]byte(wire), &cond); err != nil {
		t.Fatalf("unmarshal FilterCondition: %v", err)
	}
	if cond.Field != "score" {
		t.Errorf("Field = %q, want %q", cond.Field, "score")
	}
	if got, ok := cond.Value.(float64); !ok || got != 42 {
		t.Errorf("Value = %v (%T), want 42", cond.Value, cond.Value)
	}
}
