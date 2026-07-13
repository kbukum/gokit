package qdrant

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/kbukum/gokit/vectorstore"
)

func TestQdrantDistanceMapsMetrics(t *testing.T) {
	t.Parallel()
	cases := map[string]string{vectorstore.MetricCosine: "Cosine", vectorstore.MetricDot: "Dot", vectorstore.MetricL2: "Euclid"}
	for metric, want := range cases {
		got, err := qdrantDistance(metric)
		if err != nil || got != want {
			t.Fatalf("qdrantDistance(%q) = %q, %v; want %q", metric, got, err, want)
		}
	}
}

func TestPointIDConversion(t *testing.T) {
	t.Parallel()
	id, err := pointIDFromString("42")
	if err != nil {
		t.Fatalf("numeric id: %v", err)
	}
	b, err := json.Marshal(id)
	if err != nil || string(b) != "42" {
		t.Fatalf("numeric json = %s, %v", b, err)
	}
	uuid := "550e8400-e29b-41d4-a716-446655440000"
	id, err = pointIDFromString(uuid)
	if err != nil {
		t.Fatalf("uuid id: %v", err)
	}
	b, err = json.Marshal(id)
	if err != nil || string(b) != `"`+uuid+`"` {
		t.Fatalf("uuid json = %s, %v", b, err)
	}
	if _, err := pointIDFromString("00042"); err == nil {
		t.Fatal("expected leading zero id to fail")
	}
	if _, err := pointIDFromString("bad-id"); err == nil {
		t.Fatal("expected non-uuid id to fail")
	}
}

func TestPayloadAndFilterConversions(t *testing.T) {
	t.Parallel()
	payload, err := payloadToJSON(vectorstore.NewPointPayload().WithField("tag", "blue").WithField("active", true).WithField("score", 1.5))
	if err != nil {
		t.Fatalf("payloadToJSON: %v", err)
	}
	if string(payload["tag"]) != `"blue"` {
		t.Fatalf("tag json = %s", payload["tag"])
	}
	filter, err := filterToJSON(vectorstore.NewSearchFilter().MustMatch("tag", "blue"))
	if err != nil {
		t.Fatalf("filterToJSON: %v", err)
	}
	if len(filter["must"]) != 1 {
		t.Fatalf("filter = %#v", filter)
	}
	if _, err := payloadToJSON(vectorstore.NewPointPayload().WithField("nested", map[string]string{"bad": "value"})); err == nil {
		t.Fatal("expected nested payload to fail")
	}
}

func FuzzPointIDFromString(f *testing.F) {
	for _, seed := range []string{"42", "0", "00042", "550e8400-e29b-41d4-a716-446655440000", "bad"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, id string) { _, _ = pointIDFromString(id) })
}

func TestConversionErrorPaths(t *testing.T) {
	t.Parallel()
	if _, err := qdrantDistance("bad"); err == nil {
		t.Fatal("expected bad metric")
	}
	if _, err := supportedValueJSON(math.Inf(1)); err == nil {
		t.Fatal("expected non-finite float error")
	}
	if _, err := filterToJSON(vectorstore.NewSearchFilter().MustMatch("nested", map[string]string{"bad": "value"})); err == nil {
		t.Fatal("expected unsupported filter value")
	}
	if _, err := pointIDToString(json.RawMessage(`{"bad":true}`)); err == nil {
		t.Fatal("expected unsupported returned id")
	}
	if _, err := payloadFromJSON(map[string]json.RawMessage{"nested": json.RawMessage(`{"bad":true}`)}); err == nil {
		t.Fatal("expected unsupported returned payload")
	}
}

func TestSupportedValueJSONFloat32(t *testing.T) {
	t.Parallel()
	b, err := supportedValueJSON(float32(1.25))
	if err != nil || string(b) != "1.25" {
		t.Fatalf("float32 = %s, %v", b, err)
	}
	if _, err := supportedValueJSON(float32(math.Inf(1))); err == nil {
		t.Fatal("expected non-finite float32 error")
	}
}

func TestPointIDFromStringRejectsOverflow(t *testing.T) {
	t.Parallel()
	if _, err := pointIDFromString("99999999999999999999999"); err == nil {
		t.Fatal("expected uint64 overflow error")
	}
}

func TestPointIDToStringDecodesStringIDs(t *testing.T) {
	t.Parallel()
	got, err := pointIDToString(json.RawMessage(`"550e8400-e29b-41d4-a716-446655440000"`))
	if err != nil || got != "550e8400-e29b-41d4-a716-446655440000" {
		t.Fatalf("string id = %q, %v", got, err)
	}
}

func TestPayloadFromJSONRejectsMalformedField(t *testing.T) {
	t.Parallel()
	if _, err := payloadFromJSON(map[string]json.RawMessage{"broken": json.RawMessage(`{`)}); err == nil {
		t.Fatal("expected decode error for malformed payload field")
	}
}
