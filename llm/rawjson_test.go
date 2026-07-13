package llm

import (
	"encoding/json"
	"testing"

	"go.yaml.in/yaml/v3"
)

func TestRawJSON_JSONRoundTrip(t *testing.T) {
	in := RawJSON(`{"think":false,"format":"json"}`)
	data, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if string(data) != `{"think":false,"format":"json"}` {
		t.Fatalf("Marshal = %s, want raw bytes unchanged", data)
	}

	var out RawJSON
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if string(out) != string(in) {
		t.Fatalf("round-trip = %s, want %s", out, in)
	}
}

func TestRawJSON_EmptyMarshalsNull(t *testing.T) {
	data, err := json.Marshal(RawJSON(nil))
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if string(data) != "null" {
		t.Fatalf("empty Marshal = %s, want null", data)
	}
}

func TestRawJSON_YAMLRoundTrip(t *testing.T) {
	const doc = "think: false\nformat: json\n"
	var got RawJSON
	if err := yaml.Unmarshal([]byte(doc), &got); err != nil {
		t.Fatalf("yaml.Unmarshal: %v", err)
	}

	var fields map[string]any
	if err := json.Unmarshal(got, &fields); err != nil {
		t.Fatalf("YAML value is not carried as a JSON object: %v", err)
	}
	if fields["think"] != false || fields["format"] != "json" {
		t.Fatalf("decoded = %v, want think=false format=json", fields)
	}

	out, err := yaml.Marshal(got)
	if err != nil {
		t.Fatalf("yaml.Marshal: %v", err)
	}
	if len(out) == 0 {
		t.Fatal("yaml.Marshal produced empty output")
	}
}

// A CompletionRequest defined in YAML must carry provider extensions the same
// way as a JSON-defined one: the YAML mapping is normalized to opaque JSON that
// each dialect merges at its wire boundary.
func TestCompletionRequest_YAMLExtra(t *testing.T) {
	const doc = "model: gpt-4o\nextra:\n  reasoning_effort: high\n"
	var req CompletionRequest
	if err := yaml.Unmarshal([]byte(doc), &req); err != nil {
		t.Fatalf("yaml.Unmarshal: %v", err)
	}
	if req.Model != "gpt-4o" {
		t.Fatalf("Model = %q, want gpt-4o", req.Model)
	}

	var fields map[string]any
	if err := json.Unmarshal(req.Extra, &fields); err != nil {
		t.Fatalf("Extra is not carried as a JSON object: %v", err)
	}
	if fields["reasoning_effort"] != "high" {
		t.Fatalf("Extra[reasoning_effort] = %v, want high", fields["reasoning_effort"])
	}
}

func TestCompletionRequest_YAMLNoExtra(t *testing.T) {
	var req CompletionRequest
	if err := yaml.Unmarshal([]byte("model: gpt-4o\n"), &req); err != nil {
		t.Fatalf("yaml.Unmarshal: %v", err)
	}
	if len(req.Extra) != 0 {
		t.Fatalf("Extra = %s, want empty", req.Extra)
	}
}

func TestRawJSON_YAMLNull(t *testing.T) {
	got := RawJSON("preexisting")
	if err := yaml.Unmarshal([]byte("null\n"), &got); err != nil {
		t.Fatalf("yaml.Unmarshal: %v", err)
	}
	if got != nil {
		t.Fatalf("YAML null should clear the value, got %s", got)
	}

	out, err := yaml.Marshal(RawJSON(nil))
	if err != nil {
		t.Fatalf("yaml.Marshal: %v", err)
	}
	if string(out) != "null\n" {
		t.Fatalf("empty MarshalYAML = %q, want null", out)
	}
}

func TestRawJSON_MarshalJSONInvalid(t *testing.T) {
	if _, err := RawJSON(`{bad`).MarshalJSON(); err == nil {
		t.Fatal("MarshalJSON should fail closed on invalid JSON")
	}
	if _, err := json.Marshal(CompletionRequest{Extra: RawJSON(`not json`)}); err == nil {
		t.Fatal("marshaling a request with invalid Extra should fail")
	}
}

func TestRawJSON_MarshalYAMLInvalidJSON(t *testing.T) {
	if _, err := RawJSON(`{bad`).MarshalYAML(); err == nil {
		t.Fatal("MarshalYAML should fail on invalid JSON")
	}
}

func TestRawJSON_NilPointerGuards(t *testing.T) {
	var p *RawJSON
	if err := p.UnmarshalJSON([]byte(`{}`)); err == nil {
		t.Fatal("UnmarshalJSON on nil pointer should error")
	}
	var n yaml.Node
	if err := yaml.Unmarshal([]byte("{}\n"), &n); err != nil {
		t.Fatalf("prepare node: %v", err)
	}
	if err := p.UnmarshalYAML(&n); err == nil {
		t.Fatal("UnmarshalYAML on nil pointer should error")
	}
}
