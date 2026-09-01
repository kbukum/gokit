package logging

import (
	"encoding/json"
	"testing"

	"go.yaml.in/yaml/v3"
)

func TestOutputZeroValueMarshalsAsStdout(t *testing.T) {
	t.Parallel()

	jsonData, err := json.Marshal(Output{})
	if err != nil {
		t.Fatalf("json marshal zero: %v", err)
	}
	if string(jsonData) != `"stdout"` {
		t.Fatalf("json zero = %s, want \"stdout\"", jsonData)
	}
	var fromJSON Output
	if err := json.Unmarshal(jsonData, &fromJSON); err != nil {
		t.Fatalf("json unmarshal: %v", err)
	}
	if fromJSON != OutputStdout() {
		t.Fatalf("json round trip = %+v, want stdout", fromJSON)
	}

	yamlData, err := yaml.Marshal(Output{})
	if err != nil {
		t.Fatalf("yaml marshal zero: %v", err)
	}
	var fromYAML Output
	if err := yaml.Unmarshal(yamlData, &fromYAML); err != nil {
		t.Fatalf("yaml unmarshal %s: %v", yamlData, err)
	}
	if fromYAML != OutputStdout() {
		t.Fatalf("yaml round trip = %+v, want stdout", fromYAML)
	}
}

func TestOutputShorthandJSONRoundTrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		output Output
		json   string
	}{
		{"stdout", OutputStdout(), `"stdout"`},
		{"stderr", OutputStderr(), `"stderr"`},
		{"file", OutputFile("/var/log/app.log"), `{"type":"file","path":"/var/log/app.log"}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			data, err := json.Marshal(tc.output)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			if string(data) != tc.json {
				t.Fatalf("marshal = %s, want %s", data, tc.json)
			}
			var got Output
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if got != tc.output {
				t.Fatalf("round trip = %+v, want %+v", got, tc.output)
			}
		})
	}
}

func TestOutputShorthandYAMLRoundTrip(t *testing.T) {
	t.Parallel()

	for _, tc := range []Output{OutputStdout(), OutputStderr(), OutputFile("/p.log")} {
		data, err := yaml.Marshal(tc)
		if err != nil {
			t.Fatalf("marshal %+v: %v", tc, err)
		}
		var got Output
		if err := yaml.Unmarshal(data, &got); err != nil {
			t.Fatalf("unmarshal %s: %v", data, err)
		}
		if got != tc {
			t.Fatalf("round trip = %+v, want %+v", got, tc)
		}
	}
}

func TestOutputTaggedObjectRequiresDiscriminator(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		json string
	}{
		{"empty object", `{}`},
		{"path only", `{"path":"/p.log"}`},
		{"empty type", `{"type":"","path":"/p.log"}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var out Output
			if err := json.Unmarshal([]byte(tc.json), &out); err == nil {
				t.Fatalf("expected error for %s, got %+v", tc.json, out)
			}
			if _, err := ParseOutput(map[string]any{"path": "/p.log"}); err == nil {
				t.Fatal("ParseOutput map without type should be rejected")
			}
		})
	}
}

func TestOutputRejectsPathOnStreamOutput(t *testing.T) {
	t.Parallel()

	for _, out := range []Output{
		{Type: OutputTypeStdout, Path: "/var/log/app.log"},
		{Type: OutputTypeStderr, Path: "/var/log/app.log"},
		{Type: "", Path: "/var/log/app.log"},
	} {
		if err := out.Validate(); err == nil {
			t.Fatalf("expected error validating %+v, path is only valid for file output", out)
		}
	}
}

func TestOutputMarshalFailsClosedOnInvalidValue(t *testing.T) {
	t.Parallel()

	for _, out := range []Output{
		{Type: "socket"},
		{Type: OutputTypeFile},
		{Type: OutputTypeStdout, Path: "/var/log/app.log"},
	} {
		if _, err := json.Marshal(out); err == nil {
			t.Errorf("json.Marshal(%+v) must fail closed on an invalid output", out)
		}
		if _, err := yaml.Marshal(out); err == nil {
			t.Errorf("yaml.Marshal(%+v) must fail closed on an invalid output", out)
		}
	}
}

func TestOutputRejectsNonStringTaggedFields(t *testing.T) {
	t.Parallel()

	if _, err := ParseOutput(map[string]any{"type": 42}); err == nil {
		t.Fatal("non-string type should be rejected")
	}
	if _, err := ParseOutput(map[string]any{"type": "file", "path": 42}); err == nil {
		t.Fatal("non-string path should be rejected")
	}
}

func FuzzParseOutput(f *testing.F) {
	for _, seed := range []string{"stdout", "stderr", "file", "", "bogus", "STDERR", "  stdout  "} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, input string) {
		out, err := ParseOutput(input)
		if err != nil {
			return
		}
		if verr := out.Validate(); verr != nil {
			t.Fatalf("ParseOutput(%q) accepted an invalid output %+v: %v", input, out, verr)
		}
	})
}
